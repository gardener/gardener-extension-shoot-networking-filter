// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"time"

	extensionsconfigv1alpha1 "github.com/gardener/gardener/extensions/pkg/apis/config/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
)

var _ = Describe("Provider Config Validation", func() {
	DescribeTable("#ValidateProviderConfig",
		func(providerConfig *config.Configuration, fldPath *field.Path, matcher gomegatypes.GomegaMatcher) {
			Expect(ValidateProviderConfig(providerConfig, fldPath)).To(matcher)
		},

		Entry("should succeed with empty config (shoot)", &config.Configuration{}, field.NewPath("config"),
			BeEmpty()),
		Entry("should return error for sleepDuration in shoot config",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					SleepDuration: func() *metav1.Duration { d := metav1.Duration{Duration: 15 * time.Minute}; return &d }(),
				},
			},
			field.NewPath("config"),
			ContainElement(
				PointTo(MatchFields(IgnoreExtras, Fields{"Field": Equal("config.egressFilter.sleepDuration")})),
			),
		),
		Entry("should return error for filterListProviderType in shoot config",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					FilterListProviderType: config.FilterListProviderTypeStatic,
				},
			},
			field.NewPath("config"),
			ContainElement(
				PointTo(MatchFields(IgnoreExtras, Fields{"Field": Equal("config.egressFilter.filterListProviderType")})),
			),
		),
		Entry("should return error for downloaderConfig in shoot config",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					DownloaderConfig: &config.DownloaderConfig{Endpoint: "https://example.com"},
				},
			},
			field.NewPath("config"),
			ContainElement(
				PointTo(MatchFields(IgnoreExtras, Fields{"Field": Equal("config.egressFilter.downloaderConfig")})),
			),
		),
		Entry("should return error for ensureConnectivity in shoot config",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					EnsureConnectivity: &config.EnsureConnectivity{SeedNamespaces: []string{"foo"}},
				},
			},
			field.NewPath("config"),
			ContainElement(
				PointTo(MatchFields(IgnoreExtras, Fields{"Field": Equal("config.egressFilter.ensureConnectivity")})),
			),
		),
		Entry("should return error if staticFilterList exceeds max entries",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					StaticFilterList: func() []config.Filter {
						list := make([]config.Filter, 50001)
						for i := range list {
							list[i] = config.Filter{Network: "10.0.0.0/24", Policy: config.PolicyBlockAccess}
						}
						return list
					}(),
				},
			},
			field.NewPath("config"),
			ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Field": Equal("config.egressFilter.staticFilterList")})),
			),
		),
		Entry("should return error for invalid policy in staticFilterList",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					StaticFilterList: []config.Filter{
						{Network: "10.0.0.0/24", Policy: "invalid-policy"},
					},
				},
			},
			field.NewPath("config"),
			ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Field": Equal("config.egressFilter.staticFilterList[0].policy")})),
			),
		),
		Entry("should succeed with valid StaticFilterList",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					StaticFilterList: []config.Filter{
						{Network: "10.0.0.0/24", Policy: config.PolicyBlockAccess},
						{Network: "2001:db8::/32", Policy: config.PolicyAllowAccess},
					},
				},
			},
			field.NewPath("config"),
			BeEmpty(),
		),
		Entry("should return error for StaticFilterList with invalid CIDR",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					StaticFilterList: []config.Filter{
						{Network: "not-a-cidr", Policy: config.PolicyBlockAccess},
						{Network: "10.0.0.0/24", Policy: config.PolicyAllowAccess},
					},
				},
			},
			field.NewPath("config"),
			ContainElement(
				PointTo(MatchFields(IgnoreExtras, Fields{"Field": Equal("config.egressFilter.staticFilterList[0].network")})),
			),
		),
		Entry("should succeed with empty StaticFilterList",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					StaticFilterList: []config.Filter{},
				},
			},
			field.NewPath("config"),
			BeEmpty(),
		),
		Entry("should return error if workers configuration is present but no worker names",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					Workers: &config.Workers{
						Names: []string{},
					},
				},
			},
			field.NewPath("config"),
			ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Field": Equal("config.egressFilter.workers")})),
			),
		),
		Entry("should return error for worker name exceeding max length",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					Workers: &config.Workers{
						Names: []string{"this-name-is-way-too-long"},
					},
				},
			},
			field.NewPath("config"),
			ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Field": Equal("config.egressFilter.workers[0].names")})),
			),
		),
		Entry("should return error for worker name with invalid characters",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					Workers: &config.Workers{
						Names: []string{"invalid_name!"},
					},
				},
			},
			field.NewPath("config"),
			ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Field": Equal("config.egressFilter.workers[0].names")})),
			),
		),
		Entry("should return error for empty worker name",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					Workers: &config.Workers{
						Names: []string{""},
					},
				},
			},
			field.NewPath("config"),
			ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Field": Equal("config.egressFilter.workers[0].names")})),
			),
		),
		Entry("should succeed with valid worker names and blackholing enabled",
			&config.Configuration{
				EgressFilter: &config.EgressFilter{
					Workers: &config.Workers{
						BlackholingEnabled: true,
						Names:              []string{"worker-1", "worker2"},
					},
				},
			},
			field.NewPath("config"),
			BeEmpty(),
		),
		Entry("should return error for healthCheckConfig in shoot config",
			&config.Configuration{
				HealthCheckConfig: &extensionsconfigv1alpha1.HealthCheckConfig{},
			},
			field.NewPath("config"),
			ContainElement(
				PointTo(MatchFields(IgnoreExtras, Fields{"Field": Equal("config.healthCheckConfig")})),
			),
		),
	)
})
