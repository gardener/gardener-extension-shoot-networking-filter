// SPDX-FileCopyrightText: 2021 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
)

// Config contains configuration for the policy filter.
type Config struct {
	config.Configuration
	OAuth2Secret *config.OAuth2Secret
}
