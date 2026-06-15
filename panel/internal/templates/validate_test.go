package templates

import (
	"testing"
)

func TestValidatePrivateNetwork_ValidRFC1918(t *testing.T) {
	tests := []struct {
		cidr string
	}{
		{"10.8.0.0/24"},
		{"10.0.0.0/16"},
		{"172.16.0.0/24"},
		{"172.31.255.0/24"},
		{"192.168.1.0/24"},
		{"192.168.0.0/16"},
	}
	for _, tt := range tests {
		if err := ValidatePrivateNetwork(tt.cidr, false); err != nil {
			t.Errorf("ValidatePrivateNetwork(%q, false) = %v, want nil", tt.cidr, err)
		}
	}
}

func TestValidatePrivateNetwork_ValidULA(t *testing.T) {
	tests := []struct {
		cidr string
	}{
		{"fd00::/64"},
		{"fd12:3456:789a::/48"},
		{"fdab:cdef:0123:4567::/64"},
	}
	for _, tt := range tests {
		if err := ValidatePrivateNetwork(tt.cidr, true); err != nil {
			t.Errorf("ValidatePrivateNetwork(%q, true) = %v, want nil", tt.cidr, err)
		}
	}
}

func TestValidatePrivateNetwork_InvalidPublicIP(t *testing.T) {
	tests := []struct {
		cidr string
	}{
		{"8.8.8.0/24"},
		{"1.1.1.0/24"},
		{"203.0.113.0/24"},
	}
	for _, tt := range tests {
		if err := ValidatePrivateNetwork(tt.cidr, false); err == nil {
			t.Errorf("ValidatePrivateNetwork(%q, false) = nil, want error", tt.cidr)
		}
	}
}

func TestValidatePrivateNetwork_InvalidSyntax(t *testing.T) {
	tests := []struct {
		cidr string
	}{
		{"not-a-cidr"},
		{"10.8.0.0"},
		{"10.8.0.0/abc"},
		{""},
	}
	for _, tt := range tests {
		if err := ValidatePrivateNetwork(tt.cidr, false); err == nil {
			t.Errorf("ValidatePrivateNetwork(%q, false) = nil, want error", tt.cidr)
		}
	}
}

func TestValidatePrivateNetwork_PrefixTooBoard(t *testing.T) {
	// /8 is too broad for a VPN network
	if err := ValidatePrivateNetwork("10.0.0.0/8", false); err == nil {
		t.Error("ValidatePrivateNetwork(10.0.0.0/8) = nil, want prefix error")
	}
}

func TestValidatePrivateNetwork_PrefixTooNarrow(t *testing.T) {
	// /30 is too narrow for a VPN network
	if err := ValidatePrivateNetwork("10.8.0.0/30", false); err == nil {
		t.Error("ValidatePrivateNetwork(10.8.0.0/30) = nil, want prefix error")
	}
}

func TestValidatePrivateNetwork_IPv6RejectedWhenNotAllowed(t *testing.T) {
	if err := ValidatePrivateNetwork("fd00::/64", false); err == nil {
		t.Error("ValidatePrivateNetwork(fd00::/64, false) = nil, want error about IPv6 not allowed")
	}
}

func TestValidatePrivateNetwork_IPv6PrefixOutOfRange(t *testing.T) {
	// /32 is too broad for IPv6 VPN
	if err := ValidatePrivateNetwork("fd00::/32", true); err == nil {
		t.Error("ValidatePrivateNetwork(fd00::/32, true) = nil, want prefix range error")
	}
	// /120 is too narrow
	if err := ValidatePrivateNetwork("fd00::/120", true); err == nil {
		t.Error("ValidatePrivateNetwork(fd00::/120, true) = nil, want prefix range error")
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		port    int
		wantErr bool
	}{
		{0, true},
		{1, false},
		{80, false},
		{443, false},
		{65535, false},
		{65536, true},
		{-1, true},
	}
	for _, tt := range tests {
		err := ValidatePort(tt.port)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidatePort(%d) error = %v, wantErr %v", tt.port, err, tt.wantErr)
		}
	}
}

func TestValidateProtocol(t *testing.T) {
	tests := []struct {
		proto   string
		wantErr bool
	}{
		{"udp", false},
		{"tcp", false},
		{"UDP", false},
		{"TCP", false},
		{"wireguard", true},
		{"", true},
		{"icmp", true},
	}
	for _, tt := range tests {
		err := ValidateProtocol(tt.proto)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateProtocol(%q) error = %v, wantErr %v", tt.proto, err, tt.wantErr)
		}
	}
}

func TestValidateDNS(t *testing.T) {
	tests := []struct {
		dns     string
		wantErr bool
	}{
		{"1.1.1.1", false},
		{"8.8.8.8", false},
		{"2606:4700::1", false},
		{"not-ip", true},
		{"", true},
		{"256.256.256.256", true},
	}
	for _, tt := range tests {
		err := ValidateDNS(tt.dns)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateDNS(%q) error = %v, wantErr %v", tt.dns, err, tt.wantErr)
		}
	}
}
