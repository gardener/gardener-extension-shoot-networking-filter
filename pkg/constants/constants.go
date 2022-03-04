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
	// ManagedResourceNamesSeed is the name used to describe the managed seed resources.
	ManagedResourceNamesSeed = extensionServiceName + "-seed"
	// ManagedResourceNamesShoot is the name used to describe the managed shoot resources.
	ManagedResourceNamesShoot = extensionServiceName + "-shoot"

	// ApplicationName is the name for resource describing the components deployed by the extension controller.
	ApplicationName = "egress-filter-applier"
	// WebhookTLSecretName is the name of the TLS secret resource used by the OIDC webhook in the seed cluster.
	WebhookTLSecretName = ApplicationName + "-tls"
	// TokenValidator is used to name the resources used to allow the kube-apiserver to validate tokens against the oidc authenticator.
	TokenValidator = ApplicationName + "-token-validator"

	ImageEgressFilterBlackholer = "egress-filter-blackholer"
	ImageEgressFilterFirwaller  = "egress-filter-firewaller"
)
