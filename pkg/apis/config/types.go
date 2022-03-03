// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	healthcheckconfig "github.com/gardener/gardener/extensions/pkg/controller/healthcheck/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Configuration contains information about the policy filter configuration.
type Configuration struct {
	metav1.TypeMeta

	// EgressFilter contains the configuration for the egress filter
	EgressFilter *EgressFilter

	// HealthCheckConfig is the config for the health check controller.
	HealthCheckConfig *healthcheckconfig.HealthCheckConfig
}

// EgressFilter contains the configuration for the egress filter.
type EgressFilter struct {
	// BlackholingEnabled is a flag to set blackholing or firewall approach.
	BlackholingEnabled bool
}
