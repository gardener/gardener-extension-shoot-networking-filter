// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/go-logr/logr"

	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/apis/config"
	"github.com/gardener/gardener-extension-shoot-networking-filter/pkg/constants"
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

// parseFilterList parses JSON data and detects whether it's v1 or v2 format.
// Returns the parsed filter list in v1 format.
func parseFilterList(data []byte) ([]config.Filter, error) {
	// Detect format by checking structure
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse JSON structure: %w", err)
	}

	// Detect format: v2 has "entries" field, v1 has "network" field
	isV2Format := false
	if len(raw) > 0 {
		_, hasEntries := raw[0]["entries"]
		_, hasNetwork := raw[0]["network"]
		isV2Format = hasEntries && !hasNetwork
	}

	var filters []config.Filter
	var err error
	if isV2Format {
		// Parse as v2 array format
		var filtersV2 []config.FilterListV2
		if err := json.Unmarshal(data, &filtersV2); err != nil {
			return nil, fmt.Errorf("failed to parse as v2 format: %w", err)
		}
		filters, err = convertV2ToV1(filtersV2)
		if err != nil {
			return nil, fmt.Errorf("failed to convert v2 to v1 format: %w", err)
		}
	} else {
		// Parse as v1 format
		if err := json.Unmarshal(data, &filters); err != nil {
			return nil, fmt.Errorf("failed to parse as v1 format: %w", err)
		}
	}

	return filters, nil
}

func generateEgressFilterValues(entries []config.Filter, logger logr.Logger) ([]string, []string, error) {
	if len(entries) == 0 {
		return []string{}, []string{}, nil
	}

	ipv4Nets := []net.IPNet{}
	ipv6Nets := []net.IPNet{}

	// First pass: collect all BLOCK_ACCESS entries
OUTER:
	for _, entry := range entries {
		if entry.Policy == config.PolicyBlockAccess {
			ip, ipnet, err := net.ParseCIDR(entry.Network)
			if err != nil {
				logger.Error(err, "Error parsing CIDR from filter list, ignoring it", "offending CIDR", entry.Network)
				continue
			}
			if ip.To4() != nil {
				for _, privateNet := range privateIPv4Ranges {
					if privateNet.Contains(ip) || ipnet.Contains(privateNet.IP) {
						logger.Info("Identified overlapping CIDR in filter list, ignoring it", "offending CIDR", ipnet.String(), "reserved range", privateNet.String())
						continue OUTER
					}
				}
				ipv4Nets = append(ipv4Nets, *ipnet)
			} else {
				for _, privateNet := range privateIPv6Ranges {
					if privateNet.Contains(ip) || ipnet.Contains(privateNet.IP) {
						logger.Info("Identified overlapping CIDR in filter list, ignoring it", "offending CIDR", ipnet.String(), "reserved range", privateNet.String())
						continue OUTER
					}
				}
				ipv6Nets = append(ipv6Nets, *ipnet)
			}
		}
	}

	// Second pass: process ALLOW_ACCESS entries by carving them out from blocked ranges
	for _, entry := range entries {
		if entry.Policy == config.PolicyAllowAccess {
			ip, allowNet, err := net.ParseCIDR(entry.Network)
			if err != nil {
				logger.Error(err, "Error parsing CIDR from allow list, ignoring it", "offending CIDR", entry.Network)
				continue
			}

			if ip.To4() != nil {
				ipv4Nets = removeNetFromNetList(logger, ipv4Nets, *allowNet)
			} else {
				ipv6Nets = removeNetFromNetList(logger, ipv6Nets, *allowNet)
			}
		}
	}

	return ipNetListToStringList(ipv4Nets), ipNetListToStringList(ipv6Nets), nil
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
	for s := range strings.SplitSeq(yaml, "\n") {
		s = strings.Trim(s, "- ")
		_, ipnet, err := net.ParseCIDR(s)
		if err != nil || ipnet == nil {
			continue
		}
		result = append(result, *ipnet)
	}
	return result
}

func appendStaticIPs(logger logr.Logger, secretData map[string][]byte, filterList []config.Filter) (map[string][]byte, error) {
	newSecretData := map[string][]byte{}

	staticIPv4List, staticIPv6List, err := generateEgressFilterValues(filterList, logger)
	if err != nil {
		return nil, err
	}

	staticIPv4Data := ipNetListfromPlainYamlList(convertToPlainYamlList(staticIPv4List))
	staticIPv6Data := ipNetListfromPlainYamlList(convertToPlainYamlList(staticIPv6List))

	ipV4List := ipNetListfromPlainYamlList(string(secretData[constants.KeyIPV4List]))
	ipV6List := ipNetListfromPlainYamlList(string(secretData[constants.KeyIPV6List]))

	ipV4List = append(ipV4List, staticIPv4Data...)
	ipV6List = append(ipV6List, staticIPv6Data...)

	newSecretData[constants.KeyIPV4List] = []byte(convertToPlainYamlList(ipNetListToStringList(ipV4List)))
	newSecretData[constants.KeyIPV6List] = []byte(convertToPlainYamlList(ipNetListToStringList(ipV6List)))

	return newSecretData, nil
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

// ipToCIDR converts an IP address to a CIDR network (/32 for IPv4, /128 for IPv6)
func ipToCIDR(ip net.IP) net.IPNet {
	ones := 32
	if ip.To4() == nil {
		ones = 128
	}
	return net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(ones, ones),
	}
}

func filterIPNetListForIPs(logger logr.Logger, filterList []net.IPNet, lbIPs []net.IP) []net.IPNet {
	for _, lbIP := range lbIPs {
		ipAsCIDR := ipToCIDR(lbIP)

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
				splitted := removeNetFromCIDR(logger, cidr, ipAsCIDR)
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

// removeNetFromNetList removes an allowed network from a list of blocked networks by splitting overlapping ranges
func removeNetFromNetList(logger logr.Logger, blockList []net.IPNet, allowNet net.IPNet) []net.IPNet {
	var result []net.IPNet

	for _, blockNet := range blockList {
		if overlaps(blockNet, allowNet) {
			logger.Info("Carving out allowed network from blocked range", "allowedNetwork", allowNet.String(), "blockedRange", blockNet.String())
			split := removeNetFromCIDR(logger, blockNet, allowNet)
			result = append(result, split...)
		} else {
			result = append(result, blockNet)
		}
	}

	return result
}

// overlaps checks if two CIDRs overlap
func overlaps(cidr1, cidr2 net.IPNet) bool {
	return cidr1.Contains(cidr2.IP) || cidr2.Contains(cidr1.IP)
}

// removeNetFromCIDR removes an allowed network from a blocked CIDR and returns the remaining CIDRs
func removeNetFromCIDR(logger logr.Logger, blockNet net.IPNet, allowNet net.IPNet) []net.IPNet {
	// If no overlap, return the original blocked network
	if !overlaps(blockNet, allowNet) {
		return []net.IPNet{blockNet}
	}

	// If the allowed network completely contains the blocked network, remove it entirely
	if allowNet.Contains(blockNet.IP) {
		blockMaskSize, _ := blockNet.Mask.Size()
		allowMaskSize, _ := allowNet.Mask.Size()
		if allowMaskSize <= blockMaskSize {
			return []net.IPNet{}
		}
	}

	// The blocked network must contain the allowed network, so split it
	return splitCIDRAroundSubnet(blockNet, allowNet)
}

// splitCIDRAroundSubnet splits a CIDR to exclude a subnet
func splitCIDRAroundSubnet(cidr net.IPNet, subnet net.IPNet) []net.IPNet {
	var result []net.IPNet

	cidrOnes, _ := cidr.Mask.Size()
	subnetOnes, _ := subnet.Mask.Size()

	// If subnet is equal to or larger than cidr, nothing remains
	if subnetOnes <= cidrOnes {
		return result
	}

	currentNet := cidr
	for prefixLen := cidrOnes; prefixLen < subnetOnes; prefixLen++ {
		// Split current network into two halves
		half1, half2 := splitCIDRInHalf(currentNet)

		// Check which half contains the subnet
		if half1.Contains(subnet.IP) {
			// Subnet is in first half, keep second half
			result = append(result, half2)
			currentNet = half1
		} else {
			// Subnet is in second half, keep first half
			result = append(result, half1)
			currentNet = half2
		}
	}

	return result
}

// splitCIDRInHalf splits a CIDR into two equal halves
func splitCIDRInHalf(cidr net.IPNet) (net.IPNet, net.IPNet) {
	ones, bits := cidr.Mask.Size()
	newMask := net.CIDRMask(ones+1, bits)

	// First half uses the current IP
	half1 := net.IPNet{
		IP:   cidr.IP,
		Mask: newMask,
	}

	// Second half needs IP with the next bit flipped
	ip := make(net.IP, len(cidr.IP))
	copy(ip, cidr.IP)

	// Flip the bit at position 'ones'
	bytePos := ones / 8
	bitPos := 7 - (ones % 8)
	ip[bytePos] |= 1 << bitPos

	half2 := net.IPNet{
		IP:   ip,
		Mask: newMask,
	}

	return half1, half2
}
