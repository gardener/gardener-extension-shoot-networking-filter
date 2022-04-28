// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"net"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Filter methods", func() {
	var (
		logger    = logr.Discard()
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

	DescribeTable("#removeFromCIDR", func(ipstr, cidr string, expectedCIDRs []string) {
		ip := net.ParseIP(ipstr)
		Expect(ip).NotTo(BeNil())
		_, pipnet, err := net.ParseCIDR(cidr)
		Expect(err).To(BeNil())
		ipnet := *pipnet
		actual := removeFromCIDR(ipnet, ip)
		var actualCIDRs []string
		for _, a := range actual {
			actualCIDRs = append(actualCIDRs, a.String())
		}
		Expect(actualCIDRs).To(Equal(expectedCIDRs))
	},
		Entry("1.2.3.4 in 1.2.4.0/24", "1.2.3.4", "1.2.4.0/24", []string{"1.2.4.0/24"}),
		Entry("1.2.3.4 in 1.2.3.4/32", "1.2.3.4", "1.2.3.4/32", nil),
		Entry("1.2.3.4 in 1.2.3.4/31", "1.2.3.4", "1.2.3.4/31", []string{"1.2.3.5/32"}),
		Entry("1.2.3.5 in 1.2.3.4/31", "1.2.3.5", "1.2.3.4/31", []string{"1.2.3.4/32"}),
		Entry("1.2.3.4 in 1.2.3.4/30", "1.2.3.4", "1.2.3.4/30", []string{"1.2.3.6/31", "1.2.3.5/32"}),
		Entry("1.2.3.5 in 1.2.3.4/30", "1.2.3.5", "1.2.3.4/30", []string{"1.2.3.6/31", "1.2.3.4/32"}),
		Entry("1.2.3.6 in 1.2.3.4/30", "1.2.3.6", "1.2.3.4/30", []string{"1.2.3.4/31", "1.2.3.7/32"}),
		Entry("1.2.3.7 in 1.2.3.4/30", "1.2.3.7", "1.2.3.4/30", []string{"1.2.3.4/31", "1.2.3.6/32"}),
		Entry("45.67.89.101 in 45.67.88.0/22", "45.67.89.101", "45.67.88.0/22",
			[]string{"45.67.90.0/23", "45.67.88.0/24", "45.67.89.128/25", "45.67.89.0/26", "45.67.89.64/27", "45.67.89.112/28", "45.67.89.104/29", "45.67.89.96/30", "45.67.89.102/31", "45.67.89.100/32"}),
		Entry("2001:db8::ff00:42:8329 in 2001:db8::ff00:42:8300/120", "2001:db8::ff00:42:8329", "2001:db8::ff00:42:8300/120",
			[]string{"2001:db8::ff00:42:8380/121", "2001:db8::ff00:42:8340/122", "2001:db8::ff00:42:8300/123", "2001:db8::ff00:42:8330/124", "2001:db8::ff00:42:8320/125", "2001:db8::ff00:42:832c/126", "2001:db8::ff00:42:832a/127", "2001:db8::ff00:42:8328/128"}),
	)

	var (
		empty = map[string]string{
			constants.KeyIPV4List: "[]",
			constants.KeyIPV6List: "[]",
		}
		input1 = map[string]string{
			constants.KeyIPV4List: `- 1.2.3.4/30
- 1.2.3.8/30
`,
			constants.KeyIPV6List: `- 2001::2/127
`,
		}
		expected1 = map[string]string{
			constants.KeyIPV4List: `- 1.2.3.8/30
- 1.2.3.6/32
`,
			constants.KeyIPV6List: `- 2001::2/127
`,
		}
		input2 = map[string]string{
			constants.KeyIPV4List: `- 1.0.0.64/29
- 1.2.3.4/30
- 1.2.3.8/30
- 1.2.3.4/31
- 1.2.3.4/32
- 1.2.3.0/29
- 1.0.0.64/28
`,
			constants.KeyIPV6List: `- 2001::2/127
- 2001:db8::ff00:42:8328/127
`,
		}
		expected2 = map[string]string{
			constants.KeyIPV4List: `- 1.0.0.64/29
- 1.2.3.8/30
- 1.0.0.64/28
- 1.2.3.0/30
- 1.2.3.6/32
- 1.2.3.6/32
`,
			constants.KeyIPV6List: `- 2001::2/127
- 2001:db8::ff00:42:8328/128
`,
		}
		lbIPs1 = []string{"1.2.3.4", "1.2.3.5", "1.2.3.7", "5.6.7.8", "2001:db8::ff00:42:8329"}
	)
	DescribeTable("#filterSecretDataForIPs", func(input map[string]string, lbIPs []string, expected map[string]string) {
		var ips []net.IP
		for _, s := range lbIPs {
			ip := net.ParseIP(s)
			Expect(ip).NotTo(BeNil())
			ips = append(ips, ip)
		}
		in := map[string][]byte{}
		for k, v := range input {
			in[k] = []byte(v)
		}
		actual, err := filterSecretDataForIPs(logger, in, ips)
		Expect(err).To(BeNil())
		out := map[string]string{}
		for k, v := range actual {
			out[k] = string(v)
		}
		Expect(out).To(Equal(expected))
	},
		Entry("empty", empty, nil, empty),
		Entry("empty2", empty, lbIPs1, empty),
		Entry("input1", input1, lbIPs1, expected1),
		Entry("input2", input2, lbIPs1, expected2),
	)
})
