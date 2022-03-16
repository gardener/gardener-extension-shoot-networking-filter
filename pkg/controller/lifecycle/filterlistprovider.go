// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/metrics"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	minRefreshPeriod = 30 * time.Minute
)

type filterListProvider interface {
	Setup() error
	ReadSecretData(ctx context.Context) (map[string][]byte, error)
}

type basicFilterListProvider struct {
	ctx    context.Context
	client client.Client
	logger logr.Logger
}

// ReadSecretData reads the secret data of a secret in the extension deployment namespace.
func (p *basicFilterListProvider) ReadSecretData(ctx context.Context) (map[string][]byte, error) {
	namespace, err := getExtensionDeploymentNamespace()
	if err != nil {
		return nil, err
	}
	secret := &corev1.Secret{}
	key := client.ObjectKey{Name: constants.FilterListSecretName, Namespace: namespace}
	err = p.client.Get(ctx, key, secret)
	if err != nil {
		return nil, err
	}
	return secret.Data, nil
}

func (p *basicFilterListProvider) createOrUpdateFilterListSecret(ctx context.Context, filterList []config.Filter) error {
	namespace, err := getExtensionDeploymentNamespace()
	if err != nil {
		return err
	}
	ipv4List, ipv6List, err := generateEgressFilterValues(filterList, p.logger)
	if err != nil {
		return err
	}
	ipv4Data := convertToPlainYamlList(ipv4List)
	ipv6Data := convertToPlainYamlList(ipv6List)
	p.logger.Info("filter lists loaded", constants.KeyIPV4List, len(ipv4List), constants.KeyIPV6List, len(ipv6List))
	metrics.ReportFilterListSize(constants.KeyIPV4List, len(ipv4List))
	metrics.ReportFilterListSize(constants.KeyIPV6List, len(ipv6List))

	secret := &corev1.Secret{}
	secret.Name = constants.FilterListSecretName
	secret.Namespace = namespace
	_, err = controllerutils.CreateOrGetAndMergePatch(ctx, p.client, secret, func() error {
		secret.Data = map[string][]byte{
			constants.KeyIPV4List: []byte(ipv4Data),
			constants.KeyIPV6List: []byte(ipv6Data),
		}
		return nil
	})
	return err
}

type staticFilterListProvider struct {
	basicFilterListProvider
	filterList []config.Filter
}

var _ filterListProvider = &staticFilterListProvider{}

func newStaticFilterListProvider(ctx context.Context, client client.Client, logger logr.Logger,
	filterList []config.Filter) *staticFilterListProvider {
	return &staticFilterListProvider{
		basicFilterListProvider: basicFilterListProvider{
			ctx:    ctx,
			client: client,
			logger: logger.WithName("flp-static"),
		},
		filterList: filterList,
	}
}

func (p *staticFilterListProvider) Setup() error {
	return p.createOrUpdateFilterListSecret(p.ctx, p.filterList)
}

type downloaderFilterListProvider struct {
	basicFilterListProvider
	downloaderConfig *config.DownloaderConfig
	oauth2Secret     *config.OAuth2Secret
	ticker           *time.Ticker
	tickerDone       chan bool
}

var _ filterListProvider = &downloaderFilterListProvider{}

func newDownloaderFilterListProvider(ctx context.Context, client client.Client, logger logr.Logger,
	downloaderConfig *config.DownloaderConfig, oauth2Secret *config.OAuth2Secret) *downloaderFilterListProvider {

	return &downloaderFilterListProvider{
		basicFilterListProvider: basicFilterListProvider{
			ctx:    ctx,
			client: client,
			logger: logger.WithName("flp-download"),
		},
		downloaderConfig: downloaderConfig,
		oauth2Secret:     oauth2Secret,
	}
}

func (p *downloaderFilterListProvider) Setup() error {
	if p.downloaderConfig == nil {
		return fmt.Errorf("missing egressFilter.downloaderConfig")
	}
	err := p.downloadAndStore()
	if err != nil {
		return err
	}
	if p.downloaderConfig.RefreshPeriod != nil {
		if p.downloaderConfig.RefreshPeriod.Duration < minRefreshPeriod {
			return fmt.Errorf("egressFilter.downloaderConfig.RefreshPeriod is too small: %.0f s < %.0f s", p.downloaderConfig.RefreshPeriod.Duration.Seconds(), minRefreshPeriod.Seconds())
		}
		p.ticker = time.NewTicker(p.downloaderConfig.RefreshPeriod.Duration)
		p.tickerDone = make(chan bool)
		go func() {
			for {
				select {
				case <-p.tickerDone:
					return
				case <-p.ticker.C:
					_ = p.downloadAndStore()
				}
			}
		}()
	}
	return nil
}

func (p *downloaderFilterListProvider) stopTicker() {
	if p.ticker != nil {
		p.ticker.Stop()
		p.tickerDone <- true
		p.ticker = nil
	}
}

func (p *downloaderFilterListProvider) downloadAndStore() error {
	filterList, err := p.download()
	metrics.ReportDownload(err == nil)
	if err != nil {
		p.logger.Info("download failed", "error", err)
		return err
	}
	p.logger.Info("download ok")
	err = p.createOrUpdateFilterListSecret(p.ctx, filterList)
	if err != nil {
		p.logger.Info("secret update failed", "error", err)
		return err
	}
	p.logger.Info("secret update ok")
	return nil
}

func (p *downloaderFilterListProvider) download() ([]config.Filter, error) {
	req, err := http.NewRequest(http.MethodGet, p.downloaderConfig.Endpoint, nil)
	if err != nil {
		return nil, err
	}
	if p.downloaderConfig.OAuth2Endpoint != nil {
		token, err := p.getAccessToken(*p.downloaderConfig.OAuth2Endpoint, p.oauth2Secret)
		if err != nil {
			return nil, fmt.Errorf("retrieving access token failed: %w", err)
		}
		req.Header.Add("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var filterList []config.Filter
	err = json.Unmarshal(b, &filterList)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling body failed with %w", err)
	}
	return filterList, nil
}

func (p *downloaderFilterListProvider) getAccessToken(endpoint string, oauth2secret *config.OAuth2Secret) (string, error) {
	if oauth2secret == nil {
		return "", fmt.Errorf("OAuth2 secret data is missing")
	}

	if len(oauth2secret.ClientID) == 0 {
		return "", fmt.Errorf("missing key %s in OAuth2 secret", constants.KeyClientID)
	}
	if len(oauth2secret.ClientSecret) == 0 && (len(oauth2secret.ClientCert) == 0 || len(oauth2secret.ClientCertKey) == 0) {
		return "", fmt.Errorf("missing key(s): either %s or %s and %s in OAuth2 secret", constants.KeyClientSecret,
			constants.KeyClientCert, constants.KeyClientCertKey)
	}

	clientcredConfig := clientcredentials.Config{
		ClientID:     oauth2secret.ClientID,
		ClientSecret: oauth2secret.ClientSecret,
		TokenURL:     endpoint,
	}
	ctx := p.ctx
	if len(oauth2secret.ClientCert) != 0 {
		cert, err := tls.X509KeyPair(oauth2secret.ClientCert, oauth2secret.ClientCertKey)
		if err != nil {
			return "", fmt.Errorf("building client certificate failed: %w", err)
		}
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{cert},
				},
			},
		}
		ctx = context.WithValue(p.ctx, oauth2.HTTPClient, client)
		clientcredConfig.AuthStyle = oauth2.AuthStyleInParams
	} else {
		clientcredConfig.AuthStyle = oauth2.AuthStyleInHeader
	}

	token, err := clientcredConfig.Token(ctx)
	if err != nil {
		return "", err
	}
	if token.AccessToken == "" {
		return "", fmt.Errorf("missing access token")
	}
	return token.AccessToken, nil
}

func getExtensionDeploymentNamespace() (string, error) {
	namespace := os.Getenv(constants.ExtensionNamespaceEnvName)
	if namespace == "" {
		return "", fmt.Errorf("missing env variable %q", constants.ExtensionNamespaceEnvName)
	}
	return namespace, nil
}
