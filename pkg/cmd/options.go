// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path"

	apisconfig "github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config/v1alpha1"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
	controllerconfig "github.com/gardener/gardener-extension-shoot-networking-filter/pkg/controller/config"
	healthcheckcontroller "github.com/gardener/gardener-extension-shoot-networking-filter/pkg/controller/healthcheck"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/controller/lifecycle"
	healthcheckconfig "github.com/gardener/gardener/extensions/pkg/apis/config"
	"github.com/gardener/gardener/extensions/pkg/controller/cmd"
	extensionshealthcheckcontroller "github.com/gardener/gardener/extensions/pkg/controller/healthcheck"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var (
	scheme  *runtime.Scheme
	decoder runtime.Decoder
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(apisconfig.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))

	decoder = serializer.NewCodecFactory(scheme).UniversalDecoder()
}

// PolicyFilterOptions holds options related to the policy filter controller.
type PolicyFilterOptions struct {
	ConfigLocation  string
	OAuth2ConfigDir string
	config          *PolicyFilterConfig
}

// AddFlags implements Flagger.AddFlags.
func (o *PolicyFilterOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.ConfigLocation, "config", "", "Path to policy filter configuration")
	fs.StringVar(&o.OAuth2ConfigDir, "oauth2-config-dir", "", "Directory with oauth2 configuration")
}

// Complete implements Completer.Complete.
func (o *PolicyFilterOptions) Complete() error {
	if o.ConfigLocation == "" {
		return errors.New("config location is not set")
	}
	data, err := ioutil.ReadFile(o.ConfigLocation)
	if err != nil {
		return err
	}

	config := apisconfig.Configuration{}
	_, _, err = decoder.Decode(data, nil, &config)
	if err != nil {
		return err
	}

	var oauth2Secret *apisconfig.OAuth2Secret
	if config.EgressFilter != nil && config.EgressFilter.DownloaderConfig != nil && o.OAuth2ConfigDir != "" {
		secretData := &apisconfig.OAuth2Secret{}
		filename := path.Join(o.OAuth2ConfigDir, constants.KeyClientID)
		clientID, err := ioutil.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("cannot read clientID from %s: %w", filename, err)
		}
		secretData.ClientID = string(clientID)
		clientSecret, err := ioutil.ReadFile(path.Join(o.OAuth2ConfigDir, constants.KeyClientSecret))
		if err == nil {
			secretData.ClientSecret = string(clientSecret)
		}
		secretData.ClientCert, _ = ioutil.ReadFile(path.Join(o.OAuth2ConfigDir, constants.KeyClientCert))
		secretData.ClientCertKey, _ = ioutil.ReadFile(path.Join(o.OAuth2ConfigDir, constants.KeyClientCertKey))
		oauth2Secret = secretData
	}

	o.config = &PolicyFilterConfig{
		config:       config,
		oAuth2Secret: oauth2Secret,
	}

	return nil
}

// Completed returns the decoded PolicyFilterConfig instance. Only call this if `Complete` was successful.
func (o *PolicyFilterOptions) Completed() *PolicyFilterConfig {
	return o.config
}

// PolicyFilterConfig contains configuration information about the networking policy filter.
type PolicyFilterConfig struct {
	config       apisconfig.Configuration
	oAuth2Secret *apisconfig.OAuth2Secret
}

// Apply applies the PolicyFilterOptions to the passed ControllerOptions instance.
func (c *PolicyFilterConfig) Apply(config *controllerconfig.Config) {
	config.Configuration = c.config
	config.OAuth2Secret = c.oAuth2Secret
}

// ApplyHealthCheckConfig applies the HealthCheckConfig to the config.
func (c *PolicyFilterConfig) ApplyHealthCheckConfig(config *healthcheckconfig.HealthCheckConfig) {
	if c.config.HealthCheckConfig != nil {
		*config = *c.config.HealthCheckConfig
	}
}

// ControllerSwitches are the cmd.ControllerSwitches for the extension controllers.
func ControllerSwitches() *cmd.SwitchOptions {
	return cmd.NewSwitchOptions(
		cmd.Switch(lifecycle.Name, lifecycle.AddToManager),
		cmd.Switch(extensionshealthcheckcontroller.ControllerName, healthcheckcontroller.AddToManager),
	)
}
