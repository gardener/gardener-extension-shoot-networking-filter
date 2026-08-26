// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

//go:generate extension-generator --name=extension-shoot-networking-filter --provider-type=shoot-networking-filter --component-category=extension --extension-oci-repository=local-skaffold/gardener-extension-shoot-networking-filter/charts/extension:v0.0.0 --destination=$PWD/extension.yaml

package operator
