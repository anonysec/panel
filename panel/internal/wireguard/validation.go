package wireguard

import (
	"encoding/base64"
	"fmt"
	"net"
)

// ValidatePort checks that a port number is within the acceptable range
// for WireGuard (1024–65535 inclusive).
func ValidatePort(port int) error {
	if port < 1024 || port > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1024 and 65535", port)
	}
	return nil
}

// ValidateNetworkCIDR checks that a CIDR string is a valid IPv4 or IPv6 subnet.
func ValidateNetworkCIDR(cidr string) error {
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid network CIDR %q: %w", cidr, err)
	}
	return nil
}

// ValidateWireGuardKey checks that a key is a valid WireGuard key:
// 44-character base64 string that decodes to exactly 32 bytes.
func ValidateWireGuardKey(key string) error {
	if len(key) != 44 {
		return fmt.Errorf("invalid key: expected 44 characters, got %d", len(key))
	}
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return fmt.Errorf("invalid key: base64 decode failed: %w", err)
	}
	if len(decoded) != 32 {
		return fmt.Errorf("invalid key: expected 32 bytes after decode, got %d", len(decoded))
	}
	return nil
}
