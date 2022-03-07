// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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

type basicFilterListProvider struct {
	ctx    context.Context
	client client.Client
	logger logr.Logger
}

func (p *basicFilterListProvider) createOrUpdateFilterListSecret(ctx context.Context, filterList []config.Filter) error {
	namespace, err := getExtensionDeploymentNamespace()
	if err != nil {
		return err
	}
	ipv4List, ipv6List, err := generateEgressFilterValues(filterList)
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

func (p *staticFilterListProvider) setup() error {
	return p.createOrUpdateFilterListSecret(p.ctx, p.filterList)
}

type downloaderFilterListProvider struct {
	basicFilterListProvider
	downloaderConfig *config.DownloaderConfig
	ticker           *time.Ticker
	tickerDone       chan bool
}

func newDownloaderFilterListProvider(ctx context.Context, client client.Client, logger logr.Logger,
	downloaderConfig *config.DownloaderConfig) *downloaderFilterListProvider {

	return &downloaderFilterListProvider{
		basicFilterListProvider: basicFilterListProvider{
			ctx:    ctx,
			client: client,
			logger: logger.WithName("flp-download"),
		},
		downloaderConfig: downloaderConfig,
	}
}

func (p *downloaderFilterListProvider) setup() error {
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
	if p.downloaderConfig.Authorization != nil {
		req.Header.Add("Authorization", *p.downloaderConfig.Authorization)
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
		return nil, fmt.Errorf("Unmarshalling body failed with %w", err)
	}
	return filterList, nil
}
