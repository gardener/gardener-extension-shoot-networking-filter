// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net"
	"slices"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
)

func ValidateProviderConfig(config *config.Configuration, fldPath *field.Path) field.ErrorList {
	if config == nil {
		return nil
	}

	allErrs := field.ErrorList{}

	if config.HealthCheckConfig != nil {
		allErrs = append(allErrs, field.Invalid(
			fldPath.Child("healthCheckConfig"),
			config.HealthCheckConfig,
			"healthCheckConfig is not supported in shoot configuration",
		))
	}

	allErrs = append(allErrs, validateEgressFilterConfig(config.EgressFilter, fldPath.Child("egressFilter"))...)

	return allErrs
}

func validateEgressFilterConfig(egressFilter *config.EgressFilter, fldPath *field.Path) field.ErrorList {
	if egressFilter == nil {
		return nil
	}

	var allErrs field.ErrorList

	if egressFilter.SleepDuration != nil {
		allErrs = append(allErrs, field.Invalid(
			fldPath.Child("sleepDuration"),
			egressFilter.SleepDuration,
			"sleepDuration is not supported in shoot configuration",
		))
	}

	if egressFilter.FilterListProviderType != "" {
		allErrs = append(allErrs, field.Invalid(
			fldPath.Child("filterListProviderType"),
			egressFilter.FilterListProviderType,
			"filterListProviderType is not supported in shoot configuration",
		))
	}

	if egressFilter.StaticFilterList != nil {
		allErrs = append(allErrs, validateStaticFilterList(egressFilter.StaticFilterList, fldPath.Child("staticFilterList"))...)
	}

	if egressFilter.Workers != nil {
		allErrs = append(allErrs, validateWorkersConfig(egressFilter.Workers, fldPath.Child("workers"))...)
	}

	if egressFilter.DownloaderConfig != nil {
		allErrs = append(allErrs, field.Invalid(
			fldPath.Child("downloaderConfig"),
			egressFilter.DownloaderConfig,
			"downloaderConfig is not supported in shoot configuration",
		))
	}

	if egressFilter.EnsureConnectivity != nil {
		allErrs = append(allErrs, field.Invalid(
			fldPath.Child("ensureConnectivity"),
			egressFilter.EnsureConnectivity,
			"ensureConnectivity is not supported in shoot configuration",
		))
	}

	return allErrs
}

func validateStaticFilterList(staticFilterList []config.Filter, fldPath *field.Path) field.ErrorList {
	if len(staticFilterList) == 0 {
		return nil
	}

	// Check max entries
	if len(staticFilterList) > constants.FilterListMaxEntries {
		return field.ErrorList{
			field.Invalid(
				fldPath,
				len(staticFilterList),
				fmt.Sprintf("filter list must not have more than %d entries", constants.FilterListMaxEntries),
			),
		}
	}

	allowedPolicies := []config.Policy{config.PolicyBlockAccess, config.PolicyAllowAccess}

	var allErrs field.ErrorList

	for index, filter := range staticFilterList {
		if _, _, err := net.ParseCIDR(filter.Network); err != nil {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Child("network"),
				filter.Network,
				fmt.Sprintf("filter network at index %d must be a valid CIDR", index),
			))
		}

		if !slices.Contains(allowedPolicies, filter.Policy) {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Child("policy"),
				filter.Policy,
				fmt.Sprintf("filter policy at index %d must be one of: %v", index, allowedPolicies),
			))
		}
	}
	return allErrs
}

func validateWorkersConfig(workers *config.Workers, fldPath *field.Path) field.ErrorList {
	if workers == nil {
		return nil
	}

	var allErrs field.ErrorList

	if len(workers.Names) == 0 {
		allErrs = append(allErrs, field.Invalid(
			fldPath.Child("blackholingEnabled"),
			workers.BlackholingEnabled,
			"at least one worker name must be specified",
		))
	}

	for index, name := range workers.Names {
		if len(name) > constants.MaxWorkerNameLength {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Child("names"),
				name,
				fmt.Sprintf("worker name at index %d must not exceed %d characters", index, constants.MaxWorkerNameLength),
			))
		}
		for _, msg := range validation.IsDNS1123Label(name) {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("names"), name, fmt.Sprintf("worker name at index %d is not valid: %s", index, msg)))
		}
	}
	return allErrs
}
