// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"net"
	"strings"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"

	"github.com/go-logr/logr"
)

var privateIPv4Ranges []net.IPNet
var privateIPv6Ranges []net.IPNet

func init() {
	// localhost (RFC1122)
	_, ipv4Range127, _ := net.ParseCIDR("127.0.0.0/8")
	// localhost ipv6 (RFC4291)
	_, ipv6Range1, _ := net.ParseCIDR("::1/128")
	// Private IP ranges (RFC1918)
	_, ipv4Range10, _ := net.ParseCIDR("10.0.0.0/8")
	_, ipv4Range172, _ := net.ParseCIDR("172.16.0.0/12")
	_, ipv4Range192, _ := net.ParseCIDR("192.168.0.0/16")
	// Carrier grade NAT (RFC6598)
	_, ipv4Range100, _ := net.ParseCIDR("100.64.0.0/10")
	// Link local (RFC3927)
	_, ipv4Range169, _ := net.ParseCIDR("169.254.0.0/16")
	// IPv6 link local (RFC4291)
	_, ipv6RangeFE80, _ := net.ParseCIDR("fe80::/10")
	// IPv6 unique local unicast (RFC4193)
	_, ipv6RangeFC00, _ := net.ParseCIDR("fc00::/7")
	privateIPv4Ranges = []net.IPNet{*ipv4Range127, *ipv4Range10, *ipv4Range172, *ipv4Range192, *ipv4Range100, *ipv4Range169}
	privateIPv6Ranges = []net.IPNet{*ipv6Range1, *ipv6RangeFE80, *ipv6RangeFC00}
}

func generateEgressFilterValues(entries []config.Filter, logger logr.Logger) ([]string, []string, error) {
	if len(entries) == 0 {
		return []string{}, []string{}, nil
	}

	ipv4 := []string{}
	ipv6 := []string{}
OUTER:
	for _, entry := range entries {
		if entry.Policy == config.PolicyBlockAccess {
			ip, net, err := net.ParseCIDR(entry.Network)
			if err != nil {
				logger.Error(err, "Error parsing CIDR from filter list, ignoring it", "offending CIDR", entry.Network)
				continue
			}
			if ip.To4() != nil {
				for _, privateNet := range privateIPv4Ranges {
					if privateNet.Contains(ip) || net.Contains(privateNet.IP) {
						logger.Info("Identified overlapping CIDR in filter list, ignoring it", "offending CIDR", net.String(), "reserved range", privateNet.String())
						continue OUTER
					}
				}
				ipv4 = append(ipv4, net.String())
			} else {
				for _, privateNet := range privateIPv6Ranges {
					if privateNet.Contains(ip) || net.Contains(privateNet.IP) {
						logger.Info("Identified overlapping CIDR in filter list, ignoring it", "offending CIDR", net.String(), "reserved range", privateNet.String())
						continue OUTER
					}
				}
				ipv6 = append(ipv6, net.String())
			}
		}
	}
	return ipv4, ipv6, nil
}

func convertToPlainYamlList(list []string) string {
	if len(list) == 0 {
		return "[]"
	}

	sb := strings.Builder{}
	for _, entry := range list {
		sb.WriteString("- ")
		sb.WriteString(entry)
		sb.WriteString("\n")
	}
	return sb.String()
}
