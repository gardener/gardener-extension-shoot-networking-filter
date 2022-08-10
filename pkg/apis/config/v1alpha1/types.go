// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	healthcheckconfigv1alpha1 "github.com/gardener/gardener/extensions/pkg/apis/config/v1alpha1"

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

	// FilterListProviderType specifies how the filter list is retrieved.
	// Supported types are `static` and `download`.
	FilterListProviderType FilterListProviderType `json:"filterListProviderType,omitempty"`

	// StaticFilterList contains the static filter list.
	// Only used for provider type `static`.
	// +optional
	StaticFilterList []Filter `json:"staticFilterList,omitempty"`

	// DownloaderConfig contains the configuration for the filter list downloader.
	// Only used for provider type `download`.
	// +optional
	DownloaderConfig *DownloaderConfig `json:"downloaderConfig,omitempty"`

	// EnsureConnectivity configures the removal of seed and/or shoot load balancers IPs from the filter list.
	// +optional
	EnsureConnectivity *EnsureConnectivity `json:"ensureConnectivity,omitempty"`

	// PSPDisabled is a flag to disable pod security policy.
	PSPDisabled *bool `json:"pspDisabled,omitempty"`
}

// FilterListProviderType
type FilterListProviderType string

const (
	// FilterListProviderTypeStatic is the provider type for static filter list
	FilterListProviderTypeStatic FilterListProviderType = "static"
	// FilterListProviderTypeDownload is the provider type for downloading the filter list from an URL
	FilterListProviderTypeDownload FilterListProviderType = "download"
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

// DownloaderConfig contains the configuration for the filter list downloader.
type DownloaderConfig struct {
	// Endpoint is the endpoint URL for downloading the filter list.
	Endpoint string `json:"endpoint"`
	// OAuth2Endpoint contains the optional OAuth endpoint for fetching the access token.
	// If specified, the OAuth2Secret must be provided, too.
	// +optional
	OAuth2Endpoint *string `json:"oauth2Endpoint,omitempty"`
	// RefreshPeriod is interval for refreshing the filter list.
	// If unset, the filter list is only fetched on startup.
	// +optional
	RefreshPeriod *metav1.Duration `json:"refreshPeriod,omitempty"`
}

// EnsureConnectivity configures the removal of seed and/or shoot load balancers IPs from the filter list.
type EnsureConnectivity struct {
	// SeedNamespaces contains the seed namespaces to check for load balancers.
	// +optional
	SeedNamespaces []string `json:"seedNamespaces,omitempty"`
}
