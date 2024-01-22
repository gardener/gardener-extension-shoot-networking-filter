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
	namespace := os.Getenv(constants.ExtensionNamespaceEnvName)
	if namespace == "" {
		return "", fmt.Errorf("missing env variable %q", constants.ExtensionNamespaceEnvName)
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

func main() {

	log.SetLogger(logger.MustNewZapLogger(logger.InfoLevel, logger.FormatJSON))
	logger := log.Log.WithName("Networking-Filter")
	logger.Info("Starting Network filter")

	var provider lifecycle.FilterListProvider

	blackholingEnabled := false
	sleepDuration := "1h"
	refreshPeriod := 2 * time.Hour
	pspEnabled := true

	configLocation := flag.String("config", "/etc/runtime-networking-filter/config.yaml", "Config location")
	oAuth2ConfigDir := flag.String("oauth2-config-dir", "/etc/runtime-networking-filter/oauth2", "OAuth2 config directory")
	resourceClass := flag.String("resource-class", "seed", "resource-class of gardener resource manager")
	flag.Parse()

	networkFilterNamespace, err := getNameSpace()
	if err != nil {
		logger.Error(err, "Error getting namespace")
		panic(err.Error())
	}
	logger.Info("Get Client.")
	client, err := getClient(logger)
	if err != nil {
		logger.Error(err, "Error getting Client")
		panic(err.Error())
	}
	logger.Info("Get Config.")
	pfconfig, err := getPFConfig(logger, *configLocation, *oAuth2ConfigDir)
	if err != nil {
		logger.Error(err, "Error getting config.")
		panic(err.Error())
	}

	serviceConfig := pfconfig.Config()
	oauth2secret := pfconfig.Oauth2Config()

	if serviceConfig.EgressFilter != nil {
		blackholingEnabled = serviceConfig.EgressFilter.BlackholingEnabled
		if serviceConfig.EgressFilter.PSPDisabled != nil {
			pspEnabled = !*serviceConfig.EgressFilter.PSPDisabled
		}
		if serviceConfig.EgressFilter.SleepDuration != nil {
			sleepDuration = serviceConfig.EgressFilter.SleepDuration.Duration.String()
		}
	}

	switch serviceConfig.EgressFilter.FilterListProviderType {
	case config.FilterListProviderTypeStatic:
		provider = lifecycle.NewStaticFilterListProvider(context.Background(), client, logger, serviceConfig.EgressFilter.StaticFilterList)
	case config.FilterListProviderTypeDownload:
		if serviceConfig.EgressFilter.DownloaderConfig.RefreshPeriod != nil && serviceConfig.EgressFilter.DownloaderConfig.RefreshPeriod.Duration > refreshPeriod {
			refreshPeriod = serviceConfig.EgressFilter.DownloaderConfig.RefreshPeriod.Duration
		}
		provider = lifecycle.NewDownloaderFilterListProvider(context.Background(), client, logger,
			serviceConfig.EgressFilter.DownloaderConfig, oauth2secret)
	default:
		panic(fmt.Errorf("unexpected FilterListProviderType: %s", serviceConfig.EgressFilter.FilterListProviderType))
	}

	err = provider.Setup()
	if err != nil {
		logger.Error(err, "Error error setting up provider.")
		panic(err.Error())
	}

	for {
		secretData, err := provider.ReadSecretData(context.Background())
		if err != nil {
			logger.Error(err, "Error creating filter secret.")
			panic(err.Error())
		}
		shootResources, err := lifecycle.GetShootResources(blackholingEnabled, pspEnabled, sleepDuration, constants.NamespaceKubeSystem, secretData)
		if err != nil {
			logger.Error(err, "Error creating shoot resources.")
			panic(err.Error())
		}
		logger.Info("Update managedresource.")
		err = managedresources.Create(context.Background(), client, networkFilterNamespace, "networking-filter", nil, true, *resourceClass, shootResources, func() *bool { v := false; return &v }(), nil, nil)
		if err != nil {
			logger.Error(err, "Error creating managedresource")
			panic(err.Error())
		}
		logger.Info("Update Succeeded.")
		logger.Info("Sleep for ", "refresh-period", refreshPeriod)
		time.Sleep(refreshPeriod)
	}
}
