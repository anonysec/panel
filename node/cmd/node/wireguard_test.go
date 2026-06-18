package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIsValidWireGuardKey(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		valid bool
	}{
		{"valid key", "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=", true},
		{"empty", "", false},
		{"too short", "YWJj", false},
		{"too long", "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=aa", false},
		{"invalid base64", "$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$", false},
		{"43 chars valid base64", "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NQ==", false},
		{"newline in key", "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXox\njM0NTY=", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidWireGuardKey(tc.key)
			if got != tc.valid {
				t.Errorf("isValidWireGuardKey(%q) = %v, want %v", tc.key, got, tc.valid)
			}
		})
	}
}

func TestIsValidAllowedIPs(t *testing.T) {
	tests := []struct {
		name  string
		ips   string
		valid bool
	}{
		{"single IPv4 CIDR", "10.0.0.1/32", true},
		{"single IPv4 subnet", "10.0.0.0/24", true},
		{"multiple CIDRs", "10.0.0.1/32, 192.168.1.0/24", true},
		{"IPv6 CIDR", "fd00::/128", true},
		{"mixed IPv4 and IPv6", "10.0.0.1/32, fd00::1/128", true},
		{"empty string", "", false},
		{"newline injection", "10.0.0.1/32\nPublicKey = evil", false},
		{"carriage return injection", "10.0.0.1/32\rPublicKey = evil", false},
		{"not a CIDR", "10.0.0.1", false},
		{"garbage", "not-an-ip/32", false},
		{"empty segment", "10.0.0.1/32,", false},
		{"just comma", ",", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidAllowedIPs(tc.ips)
			if got != tc.valid {
				t.Errorf("isValidAllowedIPs(%q) = %v, want %v", tc.ips, got, tc.valid)
			}
		})
	}
}

func TestRemovePeerFromConfig(t *testing.T) {
	// Test that trailing empty lines are trimmed after removal
	config := `[Interface]
PrivateKey = testkey
Address = 10.0.0.1/24

[Peer]
PublicKey = peer1key
AllowedIPs = 10.0.0.2/32

[Peer]
PublicKey = peer2key
AllowedIPs = 10.0.0.3/32
`
	result := removePeerFromConfig(config, "peer1key")

	// Should not have excessive trailing newlines
	if strings.HasSuffix(result, "\n\n") {
		t.Error("result should not end with multiple newlines")
	}
	// Should still end with a single newline
	if !strings.HasSuffix(result, "\n") {
		t.Error("result should end with a single newline")
	}
	// Should still contain peer2
	if !strings.Contains(result, "peer2key") {
		t.Error("result should still contain peer2key")
	}
	// Should not contain peer1
	if strings.Contains(result, "peer1key") {
		t.Error("result should not contain peer1key")
	}
}

func TestRemovePeerFromConfig_LastPeer(t *testing.T) {
	config := `[Interface]
PrivateKey = testkey
Address = 10.0.0.1/24

[Peer]
PublicKey = peer1key
AllowedIPs = 10.0.0.2/32
`
	result := removePeerFromConfig(config, "peer1key")

	// Should not have excessive trailing newlines
	if strings.HasSuffix(result, "\n\n") {
		t.Errorf("result should not end with multiple newlines, got: %q", result)
	}
	// Should still end with a single newline
	if !strings.HasSuffix(result, "\n") {
		t.Error("result should end with a single newline")
	}
	// Should still contain Interface section
	if !strings.Contains(result, "[Interface]") {
		t.Error("result should still contain [Interface]")
	}
}

func TestExtractPrivateKey(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{
			name:   "standard config",
			config: "[Interface]\nPrivateKey = abcdef123456\nAddress = 10.0.0.1/24\n",
			want:   "abcdef123456",
		},
		{
			name:   "with spaces around equals",
			config: "[Interface]\nPrivateKey  =  mykey123  \nListenPort = 51820\n",
			want:   "mykey123",
		},
		{
			name:   "no private key",
			config: "[Interface]\nAddress = 10.0.0.1/24\nListenPort = 51820\n",
			want:   "",
		},
		{
			name:   "empty config",
			config: "",
			want:   "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPrivateKey(tc.config)
			if got != tc.want {
				t.Errorf("extractPrivateKey() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractPeerBlocks(t *testing.T) {
	t.Run("multiple peers", func(t *testing.T) {
		config := `[Interface]
PrivateKey = serverkey
Address = 10.0.0.1/24
ListenPort = 51820

[Peer]
PublicKey = peer1key
AllowedIPs = 10.0.0.2/32

[Peer]
PublicKey = peer2key
AllowedIPs = 10.0.0.3/32
PresharedKey = psk123
`
		peers := extractPeerBlocks(config)
		if len(peers) != 2 {
			t.Fatalf("expected 2 peers, got %d", len(peers))
		}
		if !strings.Contains(peers[0], "peer1key") {
			t.Error("first peer should contain peer1key")
		}
		if !strings.Contains(peers[1], "peer2key") {
			t.Error("second peer should contain peer2key")
		}
		if !strings.Contains(peers[1], "psk123") {
			t.Error("second peer should contain preshared key")
		}
	})

	t.Run("no peers", func(t *testing.T) {
		config := "[Interface]\nPrivateKey = key\nAddress = 10.0.0.1/24\n"
		peers := extractPeerBlocks(config)
		if len(peers) != 0 {
			t.Fatalf("expected 0 peers, got %d", len(peers))
		}
	})

	t.Run("single peer", func(t *testing.T) {
		config := `[Interface]
PrivateKey = key
Address = 10.0.0.1/24

[Peer]
PublicKey = onlypeer
AllowedIPs = 10.0.0.2/32
`
		peers := extractPeerBlocks(config)
		if len(peers) != 1 {
			t.Fatalf("expected 1 peer, got %d", len(peers))
		}
		if !strings.Contains(peers[0], "onlypeer") {
			t.Error("peer should contain onlypeer")
		}
		if !strings.Contains(peers[0], "[Peer]") {
			t.Error("peer block should start with [Peer]")
		}
	})

	t.Run("peer blocks end with newline", func(t *testing.T) {
		config := "[Interface]\nPrivateKey = key\n\n[Peer]\nPublicKey = p1\nAllowedIPs = 10.0.0.2/32\n"
		peers := extractPeerBlocks(config)
		if len(peers) != 1 {
			t.Fatalf("expected 1 peer, got %d", len(peers))
		}
		if !strings.HasSuffix(peers[0], "\n") {
			t.Error("peer block should end with newline")
		}
	})
}

func TestParseWgDump(t *testing.T) {
	now := time.Now().Unix()

	tests := []struct {
		name           string
		output         string
		expectedPeers  int
		expectedActive int
	}{
		{
			name:           "empty output",
			output:         "",
			expectedPeers:  0,
			expectedActive: 0,
		},
		{
			name:           "interface line only",
			output:         "privatekey123\t51820\toff\n",
			expectedPeers:  0,
			expectedActive: 0,
		},
		{
			name: "one active peer",
			output: "privatekey123\t51820\toff\n" +
				"YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=\t(none)\t1.2.3.4:51820\t10.0.0.2/32\t" +
				strings.Replace(string(rune(0)), string(rune(0)), func() string {
					return ""
				}(), 0) +
				itoa(now-60) + "\t12345\t67890\toff\n",
			expectedPeers:  1,
			expectedActive: 1,
		},
		{
			name: "one stale peer (handshake over 3 min ago)",
			output: "privatekey123\t51820\toff\n" +
				"YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=\t(none)\t1.2.3.4:51820\t10.0.0.2/32\t" +
				itoa(now-300) + "\t12345\t67890\toff\n",
			expectedPeers:  1,
			expectedActive: 0,
		},
		{
			name: "peer with zero handshake (never connected)",
			output: "privatekey123\t51820\toff\n" +
				"YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=\t(none)\t(none)\t10.0.0.2/32\t0\t0\t0\toff\n",
			expectedPeers:  1,
			expectedActive: 0,
		},
		{
			name: "multiple peers mixed active and stale",
			output: "privatekey123\t51820\toff\n" +
				"peer1pubkey1234567890123456789012345678901234=\t(none)\t1.2.3.4:51820\t10.0.0.2/32\t" + itoa(now-30) + "\t1000\t2000\toff\n" +
				"peer2pubkey1234567890123456789012345678901234=\t(none)\t5.6.7.8:51820\t10.0.0.3/32\t" + itoa(now-200) + "\t3000\t4000\toff\n" +
				"peer3pubkey1234567890123456789012345678901234=\t(none)\t9.8.7.6:51820\t10.0.0.4/32\t" + itoa(now-100) + "\t5000\t6000\toff\n",
			expectedPeers:  3,
			expectedActive: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			peers, active := parseWgDump(tc.output, now)
			if len(peers) != tc.expectedPeers {
				t.Errorf("expected %d peers, got %d", tc.expectedPeers, len(peers))
			}
			if active != tc.expectedActive {
				t.Errorf("expected %d active peers, got %d", tc.expectedActive, active)
			}
		})
	}
}

func TestParseWgDump_FieldValues(t *testing.T) {
	now := int64(1700000000)
	output := "privatekey123\t51820\toff\n" +
		"peerABCDpubkey12345678901234567890123456789AB=\t(none)\t1.2.3.4:51820\t10.0.0.2/32\t1699999900\t123456\t789012\toff\n"

	peers, active := parseWgDump(output, now)
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	if active != 1 {
		t.Errorf("expected 1 active peer, got %d", active)
	}

	peer := peers[0]
	if peer.PublicKey != "peerABCDpubkey12345678901234567890123456789AB=" {
		t.Errorf("unexpected PublicKey: %s", peer.PublicKey)
	}
	if peer.Endpoint != "1.2.3.4:51820" {
		t.Errorf("expected Endpoint=1.2.3.4:51820, got %s", peer.Endpoint)
	}
	if peer.AllowedIPs != "10.0.0.2/32" {
		t.Errorf("expected AllowedIPs=10.0.0.2/32, got %s", peer.AllowedIPs)
	}
	if peer.LatestHandshake != 1699999900 {
		t.Errorf("expected LatestHandshake=1699999900, got %d", peer.LatestHandshake)
	}
	if peer.RxBytes != 123456 {
		t.Errorf("expected RxBytes=123456, got %d", peer.RxBytes)
	}
	if peer.TxBytes != 789012 {
		t.Errorf("expected TxBytes=789012, got %d", peer.TxBytes)
	}
	if !peer.Active {
		t.Error("expected Active=true for peer with handshake 100s ago")
	}
}

func TestParseWgDump_BoundaryThreeMinutes(t *testing.T) {
	now := int64(1700000000)

	// Exactly 180 seconds ago - should NOT be active (< 180 required, not <=)
	output179 := "privatekey123\t51820\toff\n" +
		"peer1key\t(none)\t1.2.3.4:51820\t10.0.0.2/32\t" + itoa(now-179) + "\t100\t200\toff\n"
	output180 := "privatekey123\t51820\toff\n" +
		"peer2key\t(none)\t1.2.3.4:51820\t10.0.0.2/32\t" + itoa(now-180) + "\t100\t200\toff\n"
	output181 := "privatekey123\t51820\toff\n" +
		"peer3key\t(none)\t1.2.3.4:51820\t10.0.0.2/32\t" + itoa(now-181) + "\t100\t200\toff\n"

	peers179, active179 := parseWgDump(output179, now)
	peers180, active180 := parseWgDump(output180, now)
	peers181, active181 := parseWgDump(output181, now)

	if active179 != 1 {
		t.Errorf("179s ago should be active, got %d", active179)
	}
	if !peers179[0].Active {
		t.Error("179s ago peer Active field should be true")
	}
	if active180 != 0 {
		t.Errorf("180s ago should NOT be active, got %d", active180)
	}
	if peers180[0].Active {
		t.Error("180s ago peer Active field should be false")
	}
	if active181 != 0 {
		t.Errorf("181s ago should NOT be active, got %d", active181)
	}
	if peers181[0].Active {
		t.Error("181s ago peer Active field should be false")
	}
}

// itoa converts an int64 to string for building test dump output.
func itoa(n int64) string {
	return strings.TrimSpace(strings.Replace(
		func() string { s := ""; s = fmt.Sprintf("%d", n); return s }(),
		" ", "", -1))
}
