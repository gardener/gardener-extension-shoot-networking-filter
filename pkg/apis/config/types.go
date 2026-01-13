// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	extensionsconfigv1alpha1 "github.com/gardener/gardener/extensions/pkg/apis/config/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Configuration contains information about the policy filter configuration.
type Configuration struct {
	metav1.TypeMeta

	// EgressFilter contains the configuration for the egress filter
	EgressFilter *EgressFilter

	// HealthCheckConfig is the config for the health check controller.
	HealthCheckConfig *extensionsconfigv1alpha1.HealthCheckConfig
}

// EgressFilter contains the configuration for the egress filter.
type EgressFilter struct {
	// BlackholingEnabled is a flag to set blackholing or firewall approach.
	BlackholingEnabled bool

	// Workers contains worker-specific block modes
	Workers *Workers

	// SleepDuration is the time interval between policy updates.
	SleepDuration *metav1.Duration

	// FilterListProviderType specifies how the filter list is retrieved.
	// Supported types are `static` and `download`.
	FilterListProviderType FilterListProviderType

	// StaticFilterList contains the static filter list.
	// Only used for provider type `static`.
	StaticFilterList []Filter

	// DownloaderConfig contains the configuration for the filter list downloader.
	// Only used for provider type `download`.
	DownloaderConfig *DownloaderConfig

	// EnsureConnectivity configures the removal of seed and/or shoot load balancers IPs from the filter list.
	EnsureConnectivity *EnsureConnectivity

	// TagFilters contains filters to select entries based on tags.
	// Only used with v2 format filter lists.
	TagFilters []TagFilter
}

// TagFilter specifies a tag-based filter criterion.
type TagFilter struct {
	// Name is the tag name to filter on.
	Name string
	// Values is the list of allowed tag values.
	// An entry matches if it has this tag with any of these values.
	Values []string
}

type FilterListProviderType string

const (
	// FilterListProviderTypeStatic is the provider type for static filter list
	FilterListProviderTypeStatic FilterListProviderType = "static"
	// FilterListProviderTypeDownload is the provider type for downloading the filter list from a URL
	FilterListProviderTypeDownload FilterListProviderType = "download"
)

// Policy is the access policy
type Policy string

const (
	// PolicyAllowAccess is the `ALLOW_ACCESS` policy
	PolicyAllowAccess Policy = "ALLOW_ACCESS"
	// PolicyBlockAccess is the `BLOCK_ACCESS` policy
	PolicyBlockAccess Policy = "BLOCK_ACCESS"
	// PolicyAllow is the `ALLOW` policy (v2 format)
	PolicyAllow Policy = "ALLOW"
	// PolicyBlock is the `BLOCK` policy (v2 format)
	PolicyBlock Policy = "BLOCK"
)

// Filter specifies a network-CIDR policy pair.
type Filter struct {
	// Network is the network CIDR of the filter.
	Network string
	// Policy is the access policy (`BLOCK_ACCESS` or `ALLOW_ACCESS`).
	Policy Policy
	// Tags contains metadata tags for the entry (preserved from v2 format).
	Tags []Tag
}

// FilterListV2 represents the v2 policy list format.
// Only the Entries field is used; other fields in the JSON are ignored.
type FilterListV2 struct {
	// Entries contains the list of filter entries.
	Entries []FilterEntryV2 `json:"entries"`
}

// FilterEntryV2 represents a single filter entry in the v2 format.
type FilterEntryV2 struct {
	// Target is the network CIDR of the filter.
	Target string `json:"target"`
	// Tags contains metadata tags for the entry.
	Tags []Tag `json:"tags,omitempty"`
	// Policy is the access policy (`BLOCK` or `ALLOW`).
	Policy Policy `json:"policy"`
}

// Tag represents a metadata tag with a name and values.
type Tag struct {
	// Name is the tag name.
	Name string `json:"name"`
	// Values is the list of tag values.
	Values []string `json:"values"`
}

// DownloaderConfig contains the configuration for the filter list downloader.
type DownloaderConfig struct {
	// Endpoint is the endpoint URL for downloading the filter list.
	Endpoint string
	// OAuth2Endpoint contains the optional OAuth endpoint for fetching the access token.
	// If specified, the OAuth2Secret must be provided, too.
	OAuth2Endpoint *string
	// RefreshPeriod is interval for refreshing the filter list.
	// If unset, the filter list is only fetched on startup.
	RefreshPeriod *metav1.Duration
}

// OAuth2Secret contains the secret data for the optional oauth2 authorisation.
type OAuth2Secret struct {
	// ClientID is the OAuth2 client id.
	ClientID string
	// ClientSecret is the optional OAuth2 client secret.
	ClientSecret string
	// ClientCert is the optional client certificate.
	ClientCert []byte
	// ClientCertKey is the optional private key of the client certificate.
	ClientCertKey []byte
}

// EnsureConnectivity configures the removal of seed and/or shoot load balancers IPs from the filter list.
type EnsureConnectivity struct {
	// SeedNamespaces contains the seed namespaces to check for load balancers.
	SeedNamespaces []string
}

// Workers allows to specify block modes per worker group.
type Workers struct {
	// BlackholingEnabled is a flag to set blackholing or firewall approach.
	BlackholingEnabled bool

	// Names is a list of worker groups to use the specified blocking mode.
	Names []string
}
