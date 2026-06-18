package wireguard

import (
	"strings"
	"testing"
)

func TestGenerateClientConfig(t *testing.T) {
	cfg := ClientConfig{
		PrivateKey:      "cGVlcl9wcml2YXRlX2tleV9iYXNlNjRfZW5jb2RlZA==",
		Address:         "10.0.0.2/32",
		DNS:             "1.1.1.1, 8.8.8.8",
		ServerPublicKey: "c2VydmVyX3B1YmxpY19rZXlfYmFzZTY0X2VuY29kZWQ=",
		PresharedKey:    "cHJlc2hhcmVkX2tleV9iYXNlNjRfZW5jb2RlZF9oZXJl",
		Endpoint:        "vpn.example.com:51820",
	}

	result := GenerateClientConfig(cfg)

	// Verify [Interface] section
	if !strings.Contains(result, "[Interface]") {
		t.Error("missing [Interface] section")
	}
	if !strings.Contains(result, "PrivateKey = "+cfg.PrivateKey) {
		t.Error("missing or incorrect PrivateKey")
	}
	if !strings.Contains(result, "Address = "+cfg.Address) {
		t.Error("missing or incorrect Address")
	}
	if !strings.Contains(result, "DNS = "+cfg.DNS) {
		t.Error("missing or incorrect DNS")
	}

	// Verify no MTU line when GamingOptimize=false and MTU=0
	if strings.Contains(result, "MTU") {
		t.Error("MTU should not be present when GamingOptimize=false and MTU=0")
	}

	// Verify [Peer] section
	if !strings.Contains(result, "[Peer]") {
		t.Error("missing [Peer] section")
	}
	if !strings.Contains(result, "PublicKey = "+cfg.ServerPublicKey) {
		t.Error("missing or incorrect server PublicKey")
	}
	if !strings.Contains(result, "PresharedKey = "+cfg.PresharedKey) {
		t.Error("missing or incorrect PresharedKey")
	}
	if !strings.Contains(result, "AllowedIPs = 0.0.0.0/0, ::/0") {
		t.Error("missing or incorrect AllowedIPs")
	}
	if !strings.Contains(result, "Endpoint = "+cfg.Endpoint) {
		t.Error("missing or incorrect Endpoint")
	}
	if !strings.Contains(result, "PersistentKeepalive = 25") {
		t.Error("missing PersistentKeepalive = 25 for default mode")
	}
}

func TestGenerateClientConfigGamingOptimize(t *testing.T) {
	cfg := ClientConfig{
		PrivateKey:      "cGVlcl9wcml2YXRlX2tleV9iYXNlNjRfZW5jb2RlZA==",
		Address:         "10.0.0.2/32",
		DNS:             "1.1.1.1, 8.8.8.8",
		ServerPublicKey: "c2VydmVyX3B1YmxpY19rZXlfYmFzZTY0X2VuY29kZWQ=",
		PresharedKey:    "cHJlc2hhcmVkX2tleV9iYXNlNjRfZW5jb2RlZF9oZXJl",
		Endpoint:        "vpn.example.com:51820",
		GamingOptimize:  true,
	}

	result := GenerateClientConfig(cfg)

	// Verify MTU = 1280 is present
	if !strings.Contains(result, "MTU = 1280") {
		t.Error("expected MTU = 1280 when GamingOptimize=true")
	}

	// Verify PersistentKeepalive = 15
	if !strings.Contains(result, "PersistentKeepalive = 15") {
		t.Error("expected PersistentKeepalive = 15 when GamingOptimize=true")
	}

	// Verify keepalive is NOT 25
	if strings.Contains(result, "PersistentKeepalive = 25") {
		t.Error("PersistentKeepalive should be 15, not 25, when GamingOptimize=true")
	}
}

func TestGenerateClientConfigExplicitMTU(t *testing.T) {
	cfg := ClientConfig{
		PrivateKey:      "cGVlcl9wcml2YXRlX2tleV9iYXNlNjRfZW5jb2RlZA==",
		Address:         "10.0.0.2/32",
		DNS:             "1.1.1.1",
		ServerPublicKey: "c2VydmVyX3B1YmxpY19rZXlfYmFzZTY0X2VuY29kZWQ=",
		PresharedKey:    "cHJlc2hhcmVkX2tleV9iYXNlNjRfZW5jb2RlZF9oZXJl",
		Endpoint:        "192.168.1.1:51820",
		MTU:             1420,
	}

	result := GenerateClientConfig(cfg)

	// Explicit MTU should be included even without gaming optimize
	if !strings.Contains(result, "MTU = 1420") {
		t.Error("expected MTU = 1420 when explicitly set")
	}

	// Default keepalive should remain 25
	if !strings.Contains(result, "PersistentKeepalive = 25") {
		t.Error("expected PersistentKeepalive = 25 when GamingOptimize=false")
	}
}

func TestGenerateClientConfigGamingOverridesMTU(t *testing.T) {
	cfg := ClientConfig{
		PrivateKey:      "cGVlcl9wcml2YXRlX2tleV9iYXNlNjRfZW5jb2RlZA==",
		Address:         "10.0.0.2/32",
		DNS:             "1.1.1.1",
		ServerPublicKey: "c2VydmVyX3B1YmxpY19rZXlfYmFzZTY0X2VuY29kZWQ=",
		PresharedKey:    "cHJlc2hhcmVkX2tleV9iYXNlNjRfZW5jb2RlZF9oZXJl",
		Endpoint:        "192.168.1.1:51820",
		GamingOptimize:  true,
		MTU:             1420, // Should be overridden to 1280
	}

	result := GenerateClientConfig(cfg)

	// Gaming optimize forces MTU to 1280 regardless of explicit MTU
	if !strings.Contains(result, "MTU = 1280") {
		t.Error("expected MTU = 1280 when GamingOptimize=true, even if MTU field is set differently")
	}
	if strings.Contains(result, "MTU = 1420") {
		t.Error("MTU should be 1280, not the explicit value, when GamingOptimize=true")
	}
}

func TestGenerateClientConfigFormat(t *testing.T) {
	cfg := ClientConfig{
		PrivateKey:      "abc123privatekey==",
		Address:         "10.8.0.5/24",
		DNS:             "9.9.9.9",
		ServerPublicKey: "xyz789serverpubkey==",
		PresharedKey:    "psk000presharedkey==",
		Endpoint:        "192.168.1.1:51820",
	}

	result := GenerateClientConfig(cfg)

	// Verify the [Interface] section comes before [Peer]
	ifaceIdx := strings.Index(result, "[Interface]")
	peerIdx := strings.Index(result, "[Peer]")
	if ifaceIdx < 0 || peerIdx < 0 {
		t.Fatal("missing required sections")
	}
	if ifaceIdx >= peerIdx {
		t.Error("[Interface] should come before [Peer]")
	}

	// Verify there is a blank line separating sections
	between := result[ifaceIdx:peerIdx]
	if !strings.Contains(between, "\n\n") {
		t.Error("expected blank line between [Interface] and [Peer] sections")
	}
}

func TestGenerateClientConfigDualStack(t *testing.T) {
	cfg := ClientConfig{
		PrivateKey:      "cGVlcl9wcml2YXRlX2tleV9iYXNlNjRfZW5jb2RlZA==",
		Address:         "10.66.66.2/24, fd00:1::2/64",
		DNS:             "1.1.1.1, 8.8.8.8",
		ServerPublicKey: "c2VydmVyX3B1YmxpY19rZXlfYmFzZTY0X2VuY29kZWQ=",
		PresharedKey:    "cHJlc2hhcmVkX2tleV9iYXNlNjRfZW5jb2RlZF9oZXJl",
		Endpoint:        "vpn.example.com:51820",
	}

	result := GenerateClientConfig(cfg)

	// Verify the dual-stack address is passed through correctly
	if !strings.Contains(result, "Address = 10.66.66.2/24, fd00:1::2/64") {
		t.Error("expected dual-stack Address with both IPv4 and IPv6")
	}

	// Verify AllowedIPs still covers both families
	if !strings.Contains(result, "AllowedIPs = 0.0.0.0/0, ::/0") {
		t.Error("AllowedIPs should route all traffic for both address families")
	}
}
