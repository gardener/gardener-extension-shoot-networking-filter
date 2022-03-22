// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"net"
	"strings"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"

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

func ipNetListfromPlainYamlList(yaml string) []net.IPNet {
	if strings.TrimSpace(yaml) == "[]" {
		return nil
	}
	var result []net.IPNet
	for _, s := range strings.Split(yaml, "\n") {
		s = strings.Trim(s, "- ")
		_, ipnet, err := net.ParseCIDR(s)
		if err != nil || ipnet == nil {
			continue
		}
		result = append(result, *ipnet)
	}
	return result
}

func filterSecretDataForIPs(logger logr.Logger, secretData map[string][]byte, lbIPs []net.IP) (map[string][]byte, error) {
	filteredSecretData := map[string][]byte{}
	for key, value := range secretData {
		switch key {
		case constants.KeyIPV4List:
			ipV4List := ipNetListfromPlainYamlList(string(value))
			ipV4List = filterIPNetListForIPs(logger, ipV4List, lbIPs)
			strList := ipNetListToStringList(ipV4List)
			value = []byte(convertToPlainYamlList(strList))
		case constants.KeyIPV6List:
			ipV6List := ipNetListfromPlainYamlList(string(value))
			ipV6List = filterIPNetListForIPs(logger, ipV6List, lbIPs)
			strList := ipNetListToStringList(ipV6List)
			value = []byte(convertToPlainYamlList(strList))
		}
		filteredSecretData[key] = value
	}
	return filteredSecretData, nil
}

func filterIPNetListForIPs(logger logr.Logger, filterList []net.IPNet, lbIPs []net.IP) []net.IPNet {
	for _, lbIP := range lbIPs {
		var toBeSplitted []net.IPNet
		for i := len(filterList) - 1; i >= 0; i-- {
			if filterList[i].Contains(lbIP) {
				logger.Info("Identified load balancer IP in filtered CIDR. Splitting CIDR to remove IP.", "loadBalancerIP", lbIP.String(), "cidr", filterList[i].String())
				toBeSplitted = append(toBeSplitted, filterList[i])
				filterList = append(filterList[:i], filterList[i+1:]...)
			}
		}
		if len(toBeSplitted) > 0 {
			for _, cidr := range toBeSplitted {
				splitted := removeFromCIDR(cidr, lbIP)
				filterList = append(filterList, splitted...)
			}
		}
	}
	return filterList
}

func ipNetListToStringList(filterList []net.IPNet) []string {
	result := make([]string, len(filterList))
	for i, v := range filterList {
		result[i] = v.String()
	}
	return result
}

// removeFromCIDR removes a single IP from a CIDR and returns the remaining CIDRs
func removeFromCIDR(cidr net.IPNet, ip net.IP) []net.IPNet {
	if !cidr.Contains(ip) {
		return []net.IPNet{cidr}
	}

	var result []net.IPNet
	if x := ip.To4(); x != nil {
		ip = x
	}
	n := len(ip)
	ones, bits := cidr.Mask.Size()

	for j := ones; j < bits; j++ {
		m := net.CIDRMask(j, bits)
		m1 := net.CIDRMask(j+1, bits)
		out := make(net.IP, n)
		for i := 0; i < n; i++ {
			out[i] = (ip[i] ^ (m[i] ^ m1[i])) & m1[i]
		}
		subCIDR := net.IPNet{
			IP:   out,
			Mask: net.CIDRMask(j+1, bits),
		}
		result = append(result, subCIDR)
	}
	return result
}
