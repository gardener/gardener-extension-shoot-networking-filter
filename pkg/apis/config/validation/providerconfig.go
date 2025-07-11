// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"net"
	"regexp"
	"slices"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
)

func ValidateProviderConfig(config *config.Configuration, fldPath *field.Path) field.ErrorList {
	if config == nil {
		return nil
	}

	allErrs := field.ErrorList{}

	if errList := validateEgressFilterConfig(config.EgressFilter, fldPath.Child("egressFilter")); errList != nil {
		allErrs = append(allErrs, errList...)
	}

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
		if errList := validateStaticFilterList(egressFilter.StaticFilterList, fldPath.Child("staticFilterList")); errList != nil {
			allErrs = append(allErrs, errList...)
		}
	}

	if egressFilter.Workers != nil {
		if errList := validateWorkersConfig(egressFilter.Workers, fldPath.Child("workers")); errList != nil {
			allErrs = append(allErrs, errList...)
		}
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

	for _, filter := range staticFilterList {
		if _, _, err := net.ParseCIDR(filter.Network); err != nil {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Child("network"),
				filter.Network,
				"filter network must be a valid CIDR",
			))
		}

		if !slices.Contains(allowedPolicies, filter.Policy) {
			allErrs = append(allErrs, field.Invalid(
				fldPath.Child("policy"),
				filter.Policy,
				fmt.Sprintf("filter policy must be one of: %v", allowedPolicies),
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

	if workers.BlackholingEnabled && len(workers.Names) == 0 {
		allErrs = append(allErrs, field.Invalid(
			fldPath.Child("blackholingEnabled"),
			workers.BlackholingEnabled,
			"if blackholing is enabled, at least one worker name must be specified",
		))
	}

	nameRegexp := regexp.MustCompile(`^[a-z0-9-]+$`)
	const maxNameLength = 15

	if len(workers.Names) > 0 {
		for _, name := range workers.Names {
			if len(name) == 0 {
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("names"),
					name,
					"worker name cannot be empty",
				))
				continue
			}
			if len(name) > maxNameLength {
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("names"),
					name,
					fmt.Sprintf("worker name must not exceed %d characters", maxNameLength),
				))
			}
			if !nameRegexp.MatchString(name) {
				allErrs = append(allErrs, field.Invalid(
					fldPath.Child("names"),
					name,
					"worker name must contain only lowercase alphanumeric characters or hyphen",
				))
			}
		}
	}

	return allErrs
}
