package wireguard

import (
	"net"
	"testing"
)

func TestParseSubnetRange_IPv4_Slash24(t *testing.T) {
	first, last, bits, err := ParseSubnetRange("10.66.66.0/24")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bits != 24 {
		t.Errorf("expected bits=24, got %d", bits)
	}
	if first.String() != "10.66.66.2" {
		t.Errorf("expected first=10.66.66.2, got %s", first.String())
	}
	if last.String() != "10.66.66.254" {
		t.Errorf("expected last=10.66.66.254, got %s", last.String())
	}
}

func TestParseSubnetRange_IPv4_Slash16(t *testing.T) {
	first, last, bits, err := ParseSubnetRange("172.16.0.0/16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bits != 16 {
		t.Errorf("expected bits=16, got %d", bits)
	}
	if first.String() != "172.16.0.2" {
		t.Errorf("expected first=172.16.0.2, got %s", first.String())
	}
	if last.String() != "172.16.255.254" {
		t.Errorf("expected last=172.16.255.254, got %s", last.String())
	}
}

func TestParseSubnetRange_IPv4_Slash30(t *testing.T) {
	first, last, bits, err := ParseSubnetRange("192.168.1.0/30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bits != 30 {
		t.Errorf("expected bits=30, got %d", bits)
	}
	// /30 has network=.0, broadcast=.3, gateway=.1, only .2 allocatable
	if first.String() != "192.168.1.2" {
		t.Errorf("expected first=192.168.1.2, got %s", first.String())
	}
	if last.String() != "192.168.1.2" {
		t.Errorf("expected last=192.168.1.2, got %s", last.String())
	}
}

func TestParseSubnetRange_IPv4_Slash31_TooSmall(t *testing.T) {
	_, _, _, err := ParseSubnetRange("10.0.0.0/31")
	if err != ErrSubnetTooSmall {
		t.Errorf("expected ErrSubnetTooSmall, got %v", err)
	}
}

func TestParseSubnetRange_IPv6_Slash64(t *testing.T) {
	first, last, bits, err := ParseSubnetRange("fd00:1::/64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bits != 64 {
		t.Errorf("expected bits=64, got %d", bits)
	}
	expectedFirst := net.ParseIP("fd00:1::2")
	if !first.Equal(expectedFirst) {
		t.Errorf("expected first=%s, got %s", expectedFirst, first)
	}
	expectedLast := net.ParseIP("fd00:1::ffff:ffff:ffff:ffff")
	if !last.Equal(expectedLast) {
		t.Errorf("expected last=%s, got %s", expectedLast, last)
	}
}

func TestParseSubnetRange_InvalidCIDR(t *testing.T) {
	_, _, _, err := ParseSubnetRange("not-a-cidr")
	if err != ErrInvalidCIDR {
		t.Errorf("expected ErrInvalidCIDR, got %v", err)
	}
}

func TestAllocateNextIP_IPv4_EmptyPool(t *testing.T) {
	ip, err := AllocateNextIP("10.66.66.0/24", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First allocatable after network (.0) and gateway (.1) is .2
	if ip != "10.66.66.2" {
		t.Errorf("expected 10.66.66.2, got %s", ip)
	}
}

func TestAllocateNextIP_IPv4_SkipsUsed(t *testing.T) {
	usedIPs := []string{"10.66.66.2", "10.66.66.3"}
	ip, err := AllocateNextIP("10.66.66.0/24", usedIPs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.66.66.4" {
		t.Errorf("expected 10.66.66.4, got %s", ip)
	}
}

func TestAllocateNextIP_IPv4_SkipsGateway(t *testing.T) {
	// Even if .1 is not in usedIPs, it should be skipped (gateway)
	ip, err := AllocateNextIP("10.66.66.0/24", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip == "10.66.66.0" || ip == "10.66.66.1" {
		t.Errorf("should not allocate network or gateway, got %s", ip)
	}
}

func TestAllocateNextIP_IPv4_PoolExhausted(t *testing.T) {
	// /30 has only one allocatable IP: .2
	usedIPs := []string{"192.168.1.2"}
	_, err := AllocateNextIP("192.168.1.0/30", usedIPs)
	if err != ErrPoolExhausted {
		t.Errorf("expected ErrPoolExhausted, got %v", err)
	}
}

func TestAllocateNextIP_IPv6_EmptyPool(t *testing.T) {
	ip, err := AllocateNextIP("fd00:1::/112", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := net.ParseIP("fd00:1::2")
	got := net.ParseIP(ip)
	if !got.Equal(expected) {
		t.Errorf("expected %s, got %s", expected, ip)
	}
}

func TestAllocateNextIP_IPv6_SkipsUsed(t *testing.T) {
	usedIPs := []string{"fd00:1::2", "fd00:1::3"}
	ip, err := AllocateNextIP("fd00:1::/112", usedIPs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := net.ParseIP("fd00:1::4")
	got := net.ParseIP(ip)
	if !got.Equal(expected) {
		t.Errorf("expected %s, got %s", expected, ip)
	}
}

func TestAllocateNextIP_InvalidCIDR(t *testing.T) {
	_, err := AllocateNextIP("invalid", nil)
	if err != ErrInvalidCIDR {
		t.Errorf("expected ErrInvalidCIDR, got %v", err)
	}
}

func TestFormatWithPrefix(t *testing.T) {
	result, err := FormatWithPrefix("10.66.66.2", "10.66.66.0/24")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "10.66.66.2/24" {
		t.Errorf("expected 10.66.66.2/24, got %s", result)
	}
}

func TestFormatWithPrefix_IPv6(t *testing.T) {
	result, err := FormatWithPrefix("fd00:1::2", "fd00:1::/64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "fd00:1::2/64" {
		t.Errorf("expected fd00:1::2/64, got %s", result)
	}
}

func TestFormatWithPrefix_InvalidCIDR(t *testing.T) {
	_, err := FormatWithPrefix("10.0.0.1", "invalid")
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}
}

func TestDualStackAllocation(t *testing.T) {
	// Simulate the dual-stack allocation path from createWireguardPeer:
	// Allocate from IPv4 pool, allocate from IPv6 pool, combine.
	networkCIDR := "10.66.66.0/24"
	networkIPv6 := "fd00:1::/64"
	usedIPs := []string{"10.66.66.2", "fd00:1::2"}

	// Allocate IPv4
	ipv4, err := AllocateNextIP(networkCIDR, usedIPs)
	if err != nil {
		t.Fatalf("IPv4 allocation failed: %v", err)
	}
	formattedIPv4, err := FormatWithPrefix(ipv4, networkCIDR)
	if err != nil {
		t.Fatalf("IPv4 format failed: %v", err)
	}

	// Allocate IPv6
	ipv6, err := AllocateNextIP(networkIPv6, usedIPs)
	if err != nil {
		t.Fatalf("IPv6 allocation failed: %v", err)
	}
	formattedIPv6, err := FormatWithPrefix(ipv6, networkIPv6)
	if err != nil {
		t.Fatalf("IPv6 format failed: %v", err)
	}

	// Combine as dual-stack
	combined := formattedIPv4 + ", " + formattedIPv6

	if combined != "10.66.66.3/24, fd00:1::3/64" {
		t.Errorf("expected '10.66.66.3/24, fd00:1::3/64', got '%s'", combined)
	}
}
