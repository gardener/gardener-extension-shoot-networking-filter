// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	healthcheckconfigv1alpha1 "github.com/gardener/gardener/extensions/pkg/controller/healthcheck/config/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Configuration contains information about the policy filter configuration.
type Configuration struct {
	metav1.TypeMeta `json:",inline"`

	// EgressFilter contains the configuration for the egress filter
	// +optional
	EgressFilter *EgressFilter `json:"egressFilter,omitempty"`

	// HealthCheckConfig is the config for the health check controller.
	// +optional
	HealthCheckConfig *healthcheckconfigv1alpha1.HealthCheckConfig `json:"healthCheckConfig,omitempty"`
}

// EgressFilter contains the configuration for the egress filter.
type EgressFilter struct {
	// BlackholingEnabled is a flag to set blackholing or firewall approach.
	BlackholingEnabled bool `json:"blackholingEnabled"`

	// FilterSetProviderType specifies how the filter set is retrieved.
	// Supported types are `static` and `download`.
	FilterSetProviderType FilterSetProviderType `json:"filterSetProviderType,omitempty"`

	// StaticFilterSet contains the static filter set.
	// Only used for provider type `static`.
	// +optional
	StaticFilterSet []Filter `json:"staticFilterSet,omitempty"`

	// DownloaderConfig contains the configuration for the filter set downloader.
	// Only used for provider type `download`.
	// +optional
	DownloaderConfig *DownloaderConfig `json:"downloaderConfig,omitempty"`
}

// FilterSetProviderType
type FilterSetProviderType string

const (
	// FilterSetProviderTypeStatic is the provider type for static filter set
	FilterSetProviderTypeStatic FilterSetProviderType = "static"
	// FilterSetProviderTypeDownload is the provider type for downloading the filter set from an URL
	FilterSetProviderTypeDownload FilterSetProviderType = "download"
)

// Policy is the access policy
type Policy string

const (
	// PolicyAllowAccess is the `ALLOW_ACCESS` policy
	PolicyAllowAccess Policy = "ALLOW_ACCESS"
	// PolicyBlockAccess is the `BLOCK_ACCESS` policy
	PolicyBlockAccess Policy = "BLOCK_ACCESS"
)

// Filter specifies a network-CIDR policy pair.
type Filter struct {
	// Network is the network CIDR of the filter.
	Network string `json:"network"`
	// Policy is the access policy (`BLOCK_ACCESS` or `ALLOW_ACCESS`).
	Policy Policy `json:"policy"`
}

// DownloaderConfig contains the configuration for the filter set downloader.
type DownloaderConfig struct {
	// Endpoint is the endpoint URL for downloading the filter set.
	Endpoint string `json:"endpoint"`
	// RefreshPeriod is interval for refreshing the filter set.
	// If unset, the filter set is only fetched on startup.
	// +optional
	RefreshPeriod *metav1.Duration `json:"refreshPeriod,omitempty"`
}
