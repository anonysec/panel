package wireguard

import (
	"encoding/base64"
	"testing"
)

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"below range", 0, true},
		{"just below min", 1023, true},
		{"at min", 1024, false},
		{"common wireguard port", 51820, false},
		{"at max", 65535, false},
		{"above max", 65536, true},
		{"negative", -1, true},
		{"mid range", 8080, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort(%d) error = %v, wantErr %v", tt.port, err, tt.wantErr)
			}
		})
	}
}

func TestValidateNetworkCIDR(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		wantErr bool
	}{
		{"valid IPv4 /24", "10.66.66.0/24", false},
		{"valid IPv4 /16", "192.168.0.0/16", false},
		{"valid IPv4 /32", "10.0.0.1/32", false},
		{"valid IPv6", "fd00::/64", false},
		{"valid IPv6 full", "2001:db8::/32", false},
		{"empty string", "", true},
		{"no prefix", "10.0.0.0", true},
		{"invalid prefix length", "10.0.0.0/33", true},
		{"garbage", "not-a-cidr", true},
		{"just a slash", "/24", true},
		{"missing address", "/0", true},
		{"IPv6 invalid prefix", "fd00::/129", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNetworkCIDR(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNetworkCIDR(%q) error = %v, wantErr %v", tt.cidr, err, tt.wantErr)
			}
		})
	}
}

func TestValidateWireGuardKey(t *testing.T) {
	// Generate a valid 32-byte key encoded as base64
	validKey := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid key (all zeros)", validKey, false},
		{"empty string", "", true},
		{"too short", "AAAA", true},
		{"too long", validKey + "A", true},
		{"invalid base64 chars", "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!=", true},
		{"wrong decoded length (24 bytes)", base64.StdEncoding.EncodeToString(make([]byte, 24)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWireGuardKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWireGuardKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestValidateWireGuardKey_WithGeneratedKeys(t *testing.T) {
	// Validate that keys from our own GenerateKeyPair pass validation
	for i := 0; i < 10; i++ {
		privKey, pubKey, err := GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair() error = %v", err)
		}
		if err := ValidateWireGuardKey(privKey); err != nil {
			t.Errorf("ValidateWireGuardKey(privKey) error = %v", err)
		}
		if err := ValidateWireGuardKey(pubKey); err != nil {
			t.Errorf("ValidateWireGuardKey(pubKey) error = %v", err)
		}
	}

	// Also test preshared keys
	for i := 0; i < 10; i++ {
		psk, err := GeneratePresharedKey()
		if err != nil {
			t.Fatalf("GeneratePresharedKey() error = %v", err)
		}
		if err := ValidateWireGuardKey(psk); err != nil {
			t.Errorf("ValidateWireGuardKey(psk) error = %v", err)
		}
	}
}
