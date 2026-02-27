// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/metrics"
)

const (
	minRefreshPeriod = 30 * time.Minute
)

type FilterListProvider interface {
	Setup() error
	ReadSecretData(ctx context.Context) (map[string][]byte, error)
	GetFilterList() []config.Filter
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

type StaticFilterListProvider struct {
	basicFilterListProvider
	filterList []config.Filter
}

var _ FilterListProvider = &StaticFilterListProvider{}

func NewStaticFilterListProvider(ctx context.Context, client client.Client, logger logr.Logger,
	filterList []config.Filter) *StaticFilterListProvider {
	return newStaticFilterListProvider(ctx, client, logger, filterList)
}

func newStaticFilterListProvider(ctx context.Context, client client.Client, logger logr.Logger,
	filterList []config.Filter) *StaticFilterListProvider {
	return &StaticFilterListProvider{
		basicFilterListProvider: basicFilterListProvider{
			ctx:    ctx,
			client: client,
			logger: logger.WithName("flp-static"),
		},
		filterList: filterList,
	}
}

func (p *StaticFilterListProvider) Setup() error {
	return nil
}

func (p *StaticFilterListProvider) GetFilterList() []config.Filter {
	return p.filterList
}

type DownloaderFilterListProvider struct {
	basicFilterListProvider
	downloaderConfig *config.DownloaderConfig
	oauth2Secret     *config.OAuth2Secret
	ticker           *time.Ticker
	tickerDone       chan bool
	filterList       []config.Filter // Store the raw filter list in memory
}

var _ FilterListProvider = &DownloaderFilterListProvider{}

func NewDownloaderFilterListProvider(ctx context.Context, client client.Client, logger logr.Logger,
	downloaderConfig *config.DownloaderConfig, oauth2Secret *config.OAuth2Secret) *DownloaderFilterListProvider {
	return newDownloaderFilterListProvider(ctx, client, logger, downloaderConfig, oauth2Secret)
}

func newDownloaderFilterListProvider(ctx context.Context, client client.Client, logger logr.Logger,
	downloaderConfig *config.DownloaderConfig, oauth2Secret *config.OAuth2Secret) *DownloaderFilterListProvider {

	return &DownloaderFilterListProvider{
		basicFilterListProvider: basicFilterListProvider{
			ctx:    ctx,
			client: client,
			logger: logger.WithName("flp-download"),
		},
		downloaderConfig: downloaderConfig,
		oauth2Secret:     oauth2Secret,
	}
}

func (p *DownloaderFilterListProvider) Setup() error {
	if p.downloaderConfig == nil {
		return fmt.Errorf("missing egressFilter.downloaderConfig")
	}
	err := p.downloadAndStore()
	if err != nil {
		return err
	}
	if p.downloaderConfig.RefreshPeriod != nil {
		if p.downloaderConfig.RefreshPeriod.Duration < minRefreshPeriod {
			return fmt.Errorf("egressFilter.downloaderConfig.RefreshPeriod is too small: %.0f s < %.0f s", p.downloaderConfig.RefreshPeriod.Seconds(), minRefreshPeriod.Seconds())
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

func (p *DownloaderFilterListProvider) stopTicker() {
	if p.ticker != nil {
		p.ticker.Stop()
		p.tickerDone <- true
		p.ticker = nil
	}
}

func (p *DownloaderFilterListProvider) downloadAndStore() error {
	filterList, err := p.download()
	metrics.ReportDownload(err == nil)
	if err != nil {
		p.logger.Info("download failed", "error", err)
		return err
	}
	p.logger.Info("download ok")
	p.filterList = filterList

	return nil
}

func (p *DownloaderFilterListProvider) GetFilterList() []config.Filter {
	return p.filterList
}

func (p *DownloaderFilterListProvider) download() ([]config.Filter, error) {
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
	cl := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := cl.Do(req) // #nosec G704 -- downloaderConfig is only supported in seed configuration
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse filter list using common parser
	filterList, err := parseFilterList(b)
	if err != nil {
		wrappedErr := fmt.Errorf("could not unmarshal body: '%s'", string(b))
		return nil, fmt.Errorf("unmarshalling body failed: %w: %w", err, wrappedErr)
	}

	p.logger.Info("downloaded filter list", "entries", len(filterList))

	if len(filterList) > constants.FilterListMaxEntries {
		return nil, fmt.Errorf("filterList too large: %d entries (max %d)", len(filterList), constants.FilterListMaxEntries)
	}

	for i, filter := range filterList {
		if _, _, err := net.ParseCIDR(filter.Network); err != nil {
			return nil, fmt.Errorf("filterList[%d].network: %q  %w", i, filter.Network, err)
		}
	}
	return filterList, nil
}

// convertV2ToV1 converts a v2 format filter list to v1 format
func convertV2ToV1(filterListV2 []config.FilterListV2) ([]config.Filter, error) {
	var result []config.Filter
	for _, list := range filterListV2 {
		for _, entry := range list.Entries {
			policy, err := convertPolicyV2ToV1(entry.Policy)
			if err != nil {
				return nil, fmt.Errorf("invalid policy for network %s: %w", entry.Target, err)
			}
			filter := config.Filter{
				Network: entry.Target,
				Policy:  policy,
				Tags:    entry.Tags, // Preserve tags from v2 format
			}
			result = append(result, filter)
		}
	}
	return result, nil
}

// convertPolicyV2ToV1 converts v2 policy format to v1 format
func convertPolicyV2ToV1(policyV2 config.Policy) (config.Policy, error) {
	switch policyV2 {
	case config.PolicyBlock:
		return config.PolicyBlockAccess, nil
	case config.PolicyAllow:
		return config.PolicyAllowAccess, nil
	default:
		return "", fmt.Errorf("unknown policy value: %s", policyV2)
	}
}

func (p *DownloaderFilterListProvider) getAccessToken(endpoint string, oauth2secret *config.OAuth2Secret) (string, error) {
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

	clientCredConfig := clientcredentials.Config{
		ClientID:     oauth2secret.ClientID,
		ClientSecret: oauth2secret.ClientSecret,
		TokenURL:     endpoint,
	}
	ctx := p.ctx
	if len(oauth2secret.ClientCert) != 0 {
		cert, err := tls.X509KeyPair(oauth2secret.ClientCert, oauth2secret.ClientCertKey)
		if err != nil {
			return "", fmt.Errorf("building httpClient certificate failed: %w", err)
		}
		httpClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					Certificates: []tls.Certificate{cert},
					MinVersion:   tls.VersionTLS12,
				},
			},
		}
		ctx = context.WithValue(p.ctx, oauth2.HTTPClient, httpClient)
		clientCredConfig.AuthStyle = oauth2.AuthStyleInParams
	} else {
		clientCredConfig.AuthStyle = oauth2.AuthStyleInHeader
	}

	token, err := clientCredConfig.Token(ctx)
	if err != nil {
		return "", err
	}
	if token.AccessToken == "" {
		return "", fmt.Errorf("missing access token")
	}
	return token.AccessToken, nil
}

func getExtensionDeploymentNamespace() (string, error) {
	namespace := os.Getenv(constants.FilterNamespaceEnvName)
	if namespace == "" {
		return "", fmt.Errorf("missing env variable %q", constants.FilterNamespaceEnvName)
	}
	return namespace, nil
}
