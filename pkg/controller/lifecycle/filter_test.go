// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	. "github.com/onsi/ginkgo"
	"sigs.k8s.io/controller-runtime/pkg/log"

	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Filter methods", func() {
	var (
		logger    = log.NullLogger{}
		emptyList = []config.Filter{}
		goodList  = []config.Filter{
			{Network: "1.2.3.4/31", Policy: "BLOCK_ACCESS"},
			{Network: "5.6.7.8/32", Policy: "ALLOW_ACCESS"},
			{Network: "1.2.3.5/24", Policy: "BLOCK_ACCESS"},
			{Network: "::2/128", Policy: "BLOCK_ACCESS"},
		}
		invalidCIDRList = []config.Filter{
			{Network: "1.2.3.4/33", Policy: "BLOCK_ACCESS"},
			{Network: "1.2.3.4.5/32", Policy: "BLOCK_ACCESS"},
			{Network: "0.0.0.0/0", Policy: "BLOCK_ACCESS"},
			{Network: "::2/129", Policy: "BLOCK_ACCESS"},
		}
		overlappingCIDRList = []config.Filter{
			{Network: "127.0.0.1/24", Policy: "BLOCK_ACCESS"},
			{Network: "::1/127", Policy: "BLOCK_ACCESS"},
			{Network: "10.0.1.0/16", Policy: "BLOCK_ACCESS"},
			{Network: "172.16.0.0/10", Policy: "BLOCK_ACCESS"},
			{Network: "192.168.0.0/17", Policy: "BLOCK_ACCESS"},
			{Network: "100.64.2.0/16", Policy: "BLOCK_ACCESS"},
			{Network: "169.254.1.2/32", Policy: "BLOCK_ACCESS"},
			{Network: "fe80::/9", Policy: "BLOCK_ACCESS"},
			{Network: "fe80::/11", Policy: "BLOCK_ACCESS"},
			{Network: "fc00::/7", Policy: "BLOCK_ACCESS"},
		}
	)

	DescribeTable("#generateEgressFilterValues", func(filterList []config.Filter, expected_ipv4List, expected_ipv6List []string) {
		ipv4List, ipv6List, err := generateEgressFilterValues(filterList, logger)
		Expect(err).To(BeNil())
		Expect(ipv4List).To(Equal(expected_ipv4List))
		Expect(ipv6List).To(Equal(expected_ipv6List))
	},
		Entry("nil", nil, []string{}, []string{}),
		Entry("empty list", emptyList, []string{}, []string{}),
		Entry("good list", goodList, []string{"1.2.3.4/31", "1.2.3.0/24"}, []string{"::2/128"}),
		Entry("ignore invalid CIDRs", invalidCIDRList, []string{}, []string{}),
		Entry("ignore overlapping CIDRs", overlappingCIDRList, []string{}, []string{}),
	)

	DescribeTable("#convertToPlainYamlList", func(list []string, expectedYaml string) {
		yaml := convertToPlainYamlList(list)
		Expect(yaml).To(Equal(expectedYaml))
	},
		Entry("nil", nil, "[]"),
		Entry("empty list", []string{}, "[]"),
		Entry("good list", []string{"1.2.3.4/31", "1.2.3.0/24"}, "- 1.2.3.4/31\n- 1.2.3.0/24\n"),
	)
})
