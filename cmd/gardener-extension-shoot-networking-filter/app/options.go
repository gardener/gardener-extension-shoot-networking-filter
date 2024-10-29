// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"os"

	extensionscmdcontroller "github.com/gardener/gardener/extensions/pkg/controller/cmd"
	extensionsheartbeatcmd "github.com/gardener/gardener/extensions/pkg/controller/heartbeat/cmd"

	pfcmd "github.com/gardener/gardener-extension-shoot-networking-filter/pkg/cmd"
)

// ExtensionName is the name of the extension.
const ExtensionName = "shoot-networking-filter"

// Options holds configuration passed to the Networking Policy Filter controller.
type Options struct {
	generalOptions     *extensionscmdcontroller.GeneralOptions
	pfOptions          *pfcmd.PolicyFilterOptions
	restOptions        *extensionscmdcontroller.RESTOptions
	managerOptions     *extensionscmdcontroller.ManagerOptions
	controllerOptions  *extensionscmdcontroller.ControllerOptions
	lifecycleOptions   *extensionscmdcontroller.ControllerOptions
	healthOptions      *extensionscmdcontroller.ControllerOptions
	heartbeatOptions   *extensionsheartbeatcmd.Options
	controllerSwitches *extensionscmdcontroller.SwitchOptions
	reconcileOptions   *extensionscmdcontroller.ReconcilerOptions
	optionAggregator   extensionscmdcontroller.OptionAggregator
}

// NewOptions creates a new Options instance.
func NewOptions() *Options {
	options := &Options{
		generalOptions: &extensionscmdcontroller.GeneralOptions{},
		pfOptions:      &pfcmd.PolicyFilterOptions{},
		restOptions:    &extensionscmdcontroller.RESTOptions{},
		managerOptions: &extensionscmdcontroller.ManagerOptions{
			// These are default values.
			LeaderElection:          true,
			LeaderElectionID:        extensionscmdcontroller.LeaderElectionNameID(ExtensionName),
			LeaderElectionNamespace: os.Getenv("LEADER_ELECTION_NAMESPACE"),
		},
		controllerOptions: &extensionscmdcontroller.ControllerOptions{
			// This is a default value.
			MaxConcurrentReconciles: 5,
		},
		lifecycleOptions: &extensionscmdcontroller.ControllerOptions{
			// This is a default value.
			MaxConcurrentReconciles: 5,
		},
		healthOptions: &extensionscmdcontroller.ControllerOptions{
			// This is a default value.
			MaxConcurrentReconciles: 5,
		},
		heartbeatOptions: &extensionsheartbeatcmd.Options{
			ExtensionName: ExtensionName,
			// This is a default value.
			RenewIntervalSeconds: 30,
			Namespace:            os.Getenv("LEADER_ELECTION_NAMESPACE"),
		},
		reconcileOptions:   &extensionscmdcontroller.ReconcilerOptions{},
		controllerSwitches: pfcmd.ControllerSwitches(),
	}

	options.optionAggregator = extensionscmdcontroller.NewOptionAggregator(
		options.generalOptions,
		options.pfOptions,
		options.restOptions,
		options.managerOptions,
		options.controllerOptions,
		extensionscmdcontroller.PrefixOption("lifecycle-", options.lifecycleOptions),
		extensionscmdcontroller.PrefixOption("healthcheck-", options.healthOptions),
		extensionscmdcontroller.PrefixOption("heartbeat-", options.heartbeatOptions),
		options.controllerSwitches,
		options.reconcileOptions,
	)

	return options
}
