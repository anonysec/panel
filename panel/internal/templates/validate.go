package templates

import (
	"fmt"
	"net"
	"strings"
)

// ValidatePrivateNetwork checks that a CIDR is a valid RFC1918 (IPv4) or ULA (IPv6) private network.
func ValidatePrivateNetwork(cidr string, allowIPv6 bool) error {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return fmt.Errorf("empty CIDR")
	}
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR syntax: %w", err)
	}

	if ip.To4() != nil {
		// IPv4: must be RFC1918
		privateRanges := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
		for _, r := range privateRanges {
			_, rNet, _ := net.ParseCIDR(r)
			if rNet.Contains(ip) {
				ones, _ := ipNet.Mask.Size()
				if ones < 16 || ones > 28 {
					return fmt.Errorf("prefix /%d out of range [16,28] for VPN network", ones)
				}
				return nil
			}
		}
		return fmt.Errorf("not a private IPv4 network: %s (must be RFC1918)", cidr)
	}

	// IPv6
	if !allowIPv6 {
		return fmt.Errorf("IPv6 not allowed for this protocol")
	}
	_, ulaNet, _ := net.ParseCIDR("fc00::/7")
	if ulaNet.Contains(ip) {
		ones, _ := ipNet.Mask.Size()
		if ones < 48 || ones > 112 {
			return fmt.Errorf("IPv6 prefix /%d out of range [48,112]", ones)
		}
		return nil
	}
	return fmt.Errorf("not a ULA IPv6 network: %s (must be fc00::/7)", cidr)
}

// ValidatePort checks that a port number is valid (1-65535).
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d out of range [1,65535]", port)
	}
	return nil
}

// ValidateProtocol checks that the protocol is "udp" or "tcp".
func ValidateProtocol(proto string) error {
	proto = strings.ToLower(strings.TrimSpace(proto))
	if proto != "udp" && proto != "tcp" {
		return fmt.Errorf("invalid protocol %q (must be udp or tcp)", proto)
	}
	return nil
}

// ValidateDNS checks that a string is a valid IPv4 or IPv6 address.
func ValidateDNS(dns string) error {
	dns = strings.TrimSpace(dns)
	if dns == "" {
		return fmt.Errorf("empty DNS address")
	}
	if net.ParseIP(dns) == nil {
		return fmt.Errorf("invalid DNS address: %q", dns)
	}
	return nil
}
