/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/logger"
	managedresources "github.com/gardener/gardener/pkg/utils/managedresources"
	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	pfcmd "github.com/gardener/gardener-extension-shoot-networking-filter/pkg/cmd"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/controller/lifecycle"
)

func getPFConfig(logger logr.Logger, configLocation, oAuth2ConfigDir string) (*pfcmd.PolicyFilterConfig, error) {
	options := pfcmd.PolicyFilterOptions{ConfigLocation: configLocation, OAuth2ConfigDir: oAuth2ConfigDir}
	err := options.Complete()
	if err != nil {
		return nil, err
	}
	pfconfig := options.Completed()
	return pfconfig, err
}

func getNameSpace() (string, error) {
	namespace := os.Getenv(constants.FilterNamespaceEnvName)
	if namespace == "" {
		return "", fmt.Errorf("missing env variable %q", constants.FilterNamespaceEnvName)
	}
	return namespace, nil
}

func getClient(logger logr.Logger) (client.Client, error) {
	clientScheme := scheme.Scheme

	err := resourcesv1alpha1.AddToScheme(clientScheme)
	if err != nil {
		return nil, err
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	client, err := client.New(config, client.Options{Scheme: clientScheme})
	if err != nil {
		return nil, err
	}
	return client, nil
}

func newNetworkFilter() networkFilter {
	n := networkFilter{
		blackholingEnabled: false,
		sleepDuration:      "1h",
		refreshPeriod:      time.Hour,
		pspEnabled:         false,
		configLocation:     flag.String("config", "/etc/runtime-networking-filter/config.yaml", "Config location"),
		oAuth2ConfigDir:    flag.String("oauth2-config-dir", "/etc/runtime-networking-filter/oauth2", "OAuth2 config directory"),
		resourceClass:      flag.String("resource-class", "seed", "resource-class of gardener resource manager"),
	}
	flag.Parse()
	return n
}

type networkFilter struct {
	logger             logr.Logger
	blackholingEnabled bool
	sleepDuration      string
	refreshPeriod      time.Duration
	pspEnabled         bool
	configLocation     *string
	oAuth2ConfigDir    *string
	resourceClass      *string
}

func (n networkFilter) startNetworkFilter() error {
	var provider lifecycle.FilterListProvider

	ctx := context.Background()
	networkFilterNamespace, err := getNameSpace()
	if err != nil {
		return fmt.Errorf("getting namespace failed: %w", err)
	}
	n.logger.Info("Get Client.")
	client, err := getClient(n.logger)
	if err != nil {
		return fmt.Errorf("getting client failed: %w", err)
	}
	n.logger.Info("Get Config.")
	pfconfig, err := getPFConfig(n.logger, *n.configLocation, *n.oAuth2ConfigDir)
	if err != nil {
		return fmt.Errorf("getting config failed: %w", err)
	}

	serviceConfig := pfconfig.Config()
	oauth2secret := pfconfig.Oauth2Config()

	if serviceConfig.EgressFilter != nil {
		n.blackholingEnabled = serviceConfig.EgressFilter.BlackholingEnabled
		if serviceConfig.EgressFilter.PSPDisabled != nil {
			n.pspEnabled = !*serviceConfig.EgressFilter.PSPDisabled
		}
		if serviceConfig.EgressFilter.SleepDuration != nil {
			n.sleepDuration = serviceConfig.EgressFilter.SleepDuration.Duration.String()
		}
	}

	switch serviceConfig.EgressFilter.FilterListProviderType {
	case config.FilterListProviderTypeStatic:
		provider = lifecycle.NewStaticFilterListProvider(ctx, client, n.logger, serviceConfig.EgressFilter.StaticFilterList)
	case config.FilterListProviderTypeDownload:
		if serviceConfig.EgressFilter.DownloaderConfig.RefreshPeriod != nil && serviceConfig.EgressFilter.DownloaderConfig.RefreshPeriod.Duration > n.refreshPeriod {
			n.refreshPeriod = serviceConfig.EgressFilter.DownloaderConfig.RefreshPeriod.Duration
		}
		provider = lifecycle.NewDownloaderFilterListProvider(ctx, client, n.logger,
			serviceConfig.EgressFilter.DownloaderConfig, oauth2secret)
	default:
		return fmt.Errorf("unexpected FilterListProviderType: %s", serviceConfig.EgressFilter.FilterListProviderType)
	}

	err = provider.Setup()
	if err != nil {
		return fmt.Errorf("setting up provider failed: %w", err)
	}

	for {
		secretData, err := provider.ReadSecretData(ctx)
		if err != nil {
			return fmt.Errorf("failed creating filter secret: %w", err)
		}
		shootResources, err := lifecycle.GetShootResources(n.blackholingEnabled, n.pspEnabled, n.sleepDuration, constants.NamespaceKubeSystem, secretData)
		if err != nil {
			return fmt.Errorf("failed creating shoot resources: %w", err)
		}
		n.logger.Info("Update managedresource.")
		err = managedresources.Create(ctx, client, networkFilterNamespace, "networking-filter", nil, true, *n.resourceClass, shootResources, func() *bool { v := false; return &v }(), nil, nil)
		if err != nil {
			return fmt.Errorf("failed creating managedresource: %w", err)
		}
		n.logger.Info("Update Succeeded.")
		n.logger.Info("Sleep for ", "refresh-period", n.refreshPeriod)
		time.Sleep(n.refreshPeriod)
	}
}

func main() {
	log.SetLogger(logger.MustNewZapLogger(logger.InfoLevel, logger.FormatJSON))
	logger := log.Log.WithName("Networking-Filter")
	logger.Info("Starting Network filter")

	n := newNetworkFilter()
	err := n.startNetworkFilter()
	if err != nil {
		panic(err.Error())
	}
}
