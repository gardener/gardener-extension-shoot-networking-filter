// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"

	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/extensions/pkg/controller/heartbeat"
	"github.com/gardener/gardener/extensions/pkg/util"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/component-base/version/verflag"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	filterinstall "github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config/install"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/controller/healthcheck"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/controller/lifecycle"
)

// NewServiceControllerCommand creates a new command that is used to start the networking policy filter controller.
func NewServiceControllerCommand() *cobra.Command {
	options := NewOptions()

	cmd := &cobra.Command{
		Use:           "gardener-extension-shoot-networking-filter",
		Short:         "Networking policy filter controller manages components which provide policy filter like egress filter.",
		SilenceErrors: true,

		RunE: func(cmd *cobra.Command, args []string) error {
			verflag.PrintAndExitIfRequested()

			if err := options.optionAggregator.Complete(); err != nil {
				return fmt.Errorf("error completing options: %s", err)
			}

			if err := options.heartbeatOptions.Validate(); err != nil {
				return err
			}
			cmd.SilenceUsage = true
			return options.run(cmd.Context())
		},
	}

	verflag.AddFlags(cmd.Flags())
	options.optionAggregator.AddFlags(cmd.Flags())

	return cmd
}

func (o *Options) run(ctx context.Context) error {
	// TODO: Make these flags configurable via command line parameters or component config file.
	util.ApplyClientConnectionConfigurationToRESTConfig(&componentbaseconfigv1alpha1.ClientConnectionConfiguration{
		QPS:   100.0,
		Burst: 130,
	}, o.restOptions.Completed().Config)

	mgrOpts := o.managerOptions.Completed().Options()

	mgrOpts.Client = client.Options{
		Cache: &client.CacheOptions{
			DisableFor: []client.Object{
				&corev1.Secret{},    // applied for ManagedResources
				&corev1.ConfigMap{}, // applied for monitoring config
			},
		},
	}

	mgr, err := manager.New(o.restOptions.Completed().Config, mgrOpts)
	if err != nil {
		return fmt.Errorf("could not instantiate controller-manager: %s", err)
	}

	if err := extensionscontroller.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("could not update manager scheme: %s", err)
	}

	if err := filterinstall.AddToScheme(mgr.GetScheme()); err != nil {
		return fmt.Errorf("could not update manager scheme: %s", err)
	}

	ctrlConfig := o.pfOptions.Completed()
	ctrlConfig.ApplyHealthCheckConfig(&healthcheck.DefaultAddOptions.HealthCheckConfig)
	ctrlConfig.Apply(&lifecycle.DefaultAddOptions.ServiceConfig)
	o.controllerOptions.Completed().Apply(&lifecycle.DefaultAddOptions.ControllerOptions)
	o.lifecycleOptions.Completed().Apply(&lifecycle.DefaultAddOptions.ControllerOptions)
	o.reconcileOptions.Completed().Apply(&lifecycle.DefaultAddOptions.IgnoreOperationAnnotation, &lifecycle.DefaultAddOptions.ExtensionClass)
	o.healthOptions.Completed().Apply(&healthcheck.DefaultAddOptions.Controller)
	o.heartbeatOptions.Completed().Apply(&heartbeat.DefaultAddOptions)

	if err := o.controllerSwitches.Completed().AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("could not add controllers to manager: %s", err)
	}

	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("error running manager: %s", err)
	}

	return nil
}
