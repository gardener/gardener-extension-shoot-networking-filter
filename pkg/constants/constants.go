// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package constants

const (
	// ExtensionType is the name of the extension type.
	ExtensionType = "networking-policy-filter"
	// ServiceName is the name of the service.
	ServiceName = ExtensionType

	extensionServiceName = "extension-" + ServiceName
	// ManagedResourceNamesSeed is the name used to describe the managed seed resources.
	ManagedResourceNamesSeed = extensionServiceName + "-seed"
	// ManagedResourceNamesShoot is the name used to describe the managed shoot resources.
	ManagedResourceNamesShoot = extensionServiceName + "-shoot"

	// ApplicationName is the name for resource describing the components deployed by the extension controller.
	ApplicationName = "oidc-webhook-authenticator"
	// WebhookTLSecretName is the name of the TLS secret resource used by the OIDC webhook in the seed cluster.
	WebhookTLSecretName = ApplicationName + "-tls"
	// TokenValidator is used to name the resources used to allow the kube-apiserver to validate tokens against the oidc authenticator.
	TokenValidator = ApplicationName + "-token-validator"
)
