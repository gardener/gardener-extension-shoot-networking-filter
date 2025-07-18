// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config/v1alpha1"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config/validation"
)

// ValidateProviderConfig validates the given provider configuration.
func ValidateProviderConfig(cfg *config.Configuration) error {
	externalConfig := v1alpha1.Configuration{}
	if err := v1alpha1.Convert_config_Configuration_To_v1alpha1_Configuration(cfg, &externalConfig, nil); err != nil {
		return fmt.Errorf("failed to convert configuration: %w", err)
	}
	allErrs := validation.ValidateProviderConfig(cfg, field.NewPath("providerConfig"))
	if len(allErrs) > 0 {
		return fmt.Errorf("invalid network filter configuration: %s", field.ErrorList(allErrs).ToAggregate().Error())
	}
	// Additional validation logic can be added here if needed.

	return nil
}
