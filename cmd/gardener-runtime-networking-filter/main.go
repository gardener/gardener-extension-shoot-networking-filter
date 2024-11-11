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
	"github.com/gardener/gardener/pkg/utils/managedresources"
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

func getPFConfig(configLocation, oAuth2ConfigDir string) (*pfcmd.PolicyFilterConfig, error) {
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

func getClient() (client.Client, error) {
	clientScheme := scheme.Scheme

	err := resourcesv1alpha1.AddToScheme(clientScheme)
	if err != nil {
		return nil, err
	}
	clusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	c, err := client.New(clusterConfig, client.Options{Scheme: clientScheme})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func newNetworkFilter() networkFilter {
	n := networkFilter{
		blackholingEnabled: false,
		sleepDuration:      "1h",
		refreshPeriod:      time.Hour,
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
	cl, err := getClient()
	if err != nil {
		return fmt.Errorf("getting cl failed: %w", err)
	}
	n.logger.Info("Get Config.")
	pfconfig, err := getPFConfig(*n.configLocation, *n.oAuth2ConfigDir)
	if err != nil {
		return fmt.Errorf("getting config failed: %w", err)
	}

	serviceConfig := pfconfig.Config()
	oauth2secret := pfconfig.Oauth2Config()

	if serviceConfig.EgressFilter != nil {
		n.blackholingEnabled = serviceConfig.EgressFilter.BlackholingEnabled
		if serviceConfig.EgressFilter.SleepDuration != nil {
			n.sleepDuration = serviceConfig.EgressFilter.SleepDuration.Duration.String()
		}
	}

	switch serviceConfig.EgressFilter.FilterListProviderType {
	case config.FilterListProviderTypeStatic:
		provider = lifecycle.NewStaticFilterListProvider(ctx, cl, n.logger, serviceConfig.EgressFilter.StaticFilterList)
	case config.FilterListProviderTypeDownload:
		if serviceConfig.EgressFilter.DownloaderConfig.RefreshPeriod != nil && serviceConfig.EgressFilter.DownloaderConfig.RefreshPeriod.Duration > n.refreshPeriod {
			n.refreshPeriod = serviceConfig.EgressFilter.DownloaderConfig.RefreshPeriod.Duration
		}
		provider = lifecycle.NewDownloaderFilterListProvider(ctx, cl, n.logger,
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
		shootResources, err := lifecycle.GetShootResources(n.blackholingEnabled, n.sleepDuration, constants.NamespaceKubeSystem, secretData)
		if err != nil {
			return fmt.Errorf("failed creating shoot resources: %w", err)
		}
		n.logger.Info("Update managedresource.")
		err = managedresources.Create(ctx, cl, networkFilterNamespace, "networking-filter", nil, true, *n.resourceClass, shootResources, func() *bool { v := false; return &v }(), nil, nil)
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
	loggr := log.Log.WithName("Networking-Filter")
	loggr.Info("Starting Network filter")

	n := newNetworkFilter()
	err := n.startNetworkFilter()
	if err != nil {
		panic(err.Error())
	}
}
