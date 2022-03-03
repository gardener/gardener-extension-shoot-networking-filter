// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"io/ioutil"

	apisconfig "github.com/gardener/gardener-extension-networking-policy-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-networking-policy-filter/pkg/apis/config/v1alpha1"
	controllerconfig "github.com/gardener/gardener-extension-networking-policy-filter/pkg/controller/config"
	healthcheckcontroller "github.com/gardener/gardener-extension-networking-policy-filter/pkg/controller/healthcheck"
	"github.com/gardener/gardener-extension-networking-policy-filter/pkg/controller/lifecycle"
	"github.com/gardener/gardener/extensions/pkg/controller/cmd"
	extensionshealthcheckcontroller "github.com/gardener/gardener/extensions/pkg/controller/healthcheck"
	healthcheckconfig "github.com/gardener/gardener/extensions/pkg/controller/healthcheck/config"
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
	ConfigLocation string
	config         *PolicyFilterConfig
}

// AddFlags implements Flagger.AddFlags.
func (o *PolicyFilterOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.ConfigLocation, "config", "", "Path to policy filter configuration")
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

	o.config = &PolicyFilterConfig{
		config: config,
	}

	return nil
}

// Completed returns the decoded PolicyFilterConfig instance. Only call this if `Complete` was successful.
func (o *PolicyFilterOptions) Completed() *PolicyFilterConfig {
	return o.config
}

// PolicyFilterConfig contains configuration information about the OIDC service.
type PolicyFilterConfig struct {
	config apisconfig.Configuration
}

// Apply applies the PolicyFilterOptions to the passed ControllerOptions instance.
func (c *PolicyFilterConfig) Apply(config *controllerconfig.Config) {
	config.Configuration = c.config
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
