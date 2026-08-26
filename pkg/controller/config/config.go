// SPDX-FileCopyrightText: Contributors to the Gardener project
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
