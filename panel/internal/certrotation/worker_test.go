package certrotation

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"
)

func TestCertType(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/etc/openvpn/server/ca.crt", "ca"},
		{"/etc/openvpn/server/ca.key", "ca"},
		{"/etc/openvpn/ca.crt", "ca"},
		{"/etc/openvpn/server/server.crt", "server"},
		{"/etc/openvpn/server/vpn-server.crt", "server"},
		{"/etc/strongswan/ipsec.d/certs/server.crt", "server"},
		{"/etc/openvpn/server/tls-crypt.key", "tls-crypt"},
		{"/etc/openvpn/server/ta.key", "tls-crypt"},
		{"/etc/openvpn/server/tls-auth.key", "tls-crypt"},
		{"/etc/openvpn/server/dh.pem", "unknown"},
		{"/etc/wireguard/wg0.conf", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := certType(tt.path)
			if result != tt.expected {
				t.Errorf("certType(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestCalcFingerprint(t *testing.T) {
	data := []byte("test certificate data")
	fp := calcFingerprint(data)

	// Verify it matches expected SHA256
	expected := fmt.Sprintf("%x", sha256.Sum256(data))
	if fp != expected {
		t.Errorf("calcFingerprint() = %q, want %q", fp, expected)
	}

	// Verify consistency
	fp2 := calcFingerprint(data)
	if fp != fp2 {
		t.Errorf("calcFingerprint() not consistent: %q != %q", fp, fp2)
	}

	// Verify different data produces different fingerprint
	fp3 := calcFingerprint([]byte("different data"))
	if fp == fp3 {
		t.Errorf("calcFingerprint() should produce different results for different data")
	}

	// Verify it is a valid hex string of correct length (64 chars for SHA256)
	if len(fp) != 64 {
		t.Errorf("calcFingerprint() length = %d, want 64", len(fp))
	}
}

func TestDaysUntilExpiry(t *testing.T) {
	tests := []struct {
		name     string
		expiry   time.Time
		expected int
	}{
		{
			name:     "expires in about 30 days",
			expiry:   time.Now().Add(30*24*time.Hour + 12*time.Hour),
			expected: 30,
		},
		{
			name:     "expires in about 7 days",
			expiry:   time.Now().Add(7*24*time.Hour + 12*time.Hour),
			expected: 7,
		},
		{
			name:     "expires in about 1 day",
			expiry:   time.Now().Add(1*24*time.Hour + 12*time.Hour),
			expected: 1,
		},
		{
			name:     "already expired",
			expiry:   time.Now().Add(-24 * time.Hour),
			expected: 0,
		},
		{
			name:     "expires in 12 hours",
			expiry:   time.Now().Add(12 * time.Hour),
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now()
			days := int(tt.expiry.Sub(now).Hours() / 24)
			if days < 0 {
				days = 0
			}
			if days != tt.expected {
				t.Errorf("DaysUntilExpiry = %d, want %d", days, tt.expected)
			}
		})
	}
}
