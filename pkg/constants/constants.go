// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package constants

const (
	// ExtensionType is the name of the extension type.
	ExtensionType = "shoot-networking-filter"
	// ServiceName is the name of the service.
	ServiceName = ExtensionType

	extensionServiceName = "extension-" + ServiceName
	// EgressFilterSecretName name of the secret containing the egress filter lists
	EgressFilterSecretName = extensionServiceName
	// NamespaceKubeSystem kube-system namespace
	NamespaceKubeSystem = "kube-system"
	// ManagedResourceNamesShoot is the name used to describe the managed shoot resources.
	ManagedResourceNamesShoot = extensionServiceName + "-shoot"

	// ApplicationName is the name for resource describing the components deployed by the extension controller.
	ApplicationName = "egress-filter-applier"

	ImageEgressFilterBlackholer = "egress-filter-blackholer"
	ImageEgressFilterFirwaller  = "egress-filter-firewaller"

	// FilterListSecretName name of the secret containing the egress filter list
	FilterListSecretName = "egress-filter-list"
	// ExtensionNamespaceEnvName is the namespace of the extension deployment
	ExtensionNamespaceEnvName = "EXTENSION_NAMESPACE"

	// KeyIPV4List is the key in the filter list secret for the ipv4 policy list
	KeyIPV4List = "ipv4-list"
	// KeyIPV6List is the key in the filter list secret for the ipv6 policy list
	KeyIPV6List = "ipv6-list"

	// KeyClientID is the key in the OAuth2 secret for the client ID.
	KeyClientID = "clientID"
	// KeyClientSecret is the key in the OAuth2 secret for the optional client secret.
	KeyClientSecret = "clientSecret"
	// KeyClientCert is the key in the OAuth2 secret for the optional client certificate.
	KeyClientCert = "client.crt.pem"
	// KeyClientCertKey is the key in the OAuth2 secret for the optional private key of the client certificate.
	KeyClientCertKey = "client.key.pem"
)
