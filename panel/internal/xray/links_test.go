//go:build !lite

package xray

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestGenerateVLESSLink(t *testing.T) {
	tests := []struct {
		name   string
		params LinkParams
		check  func(t *testing.T, link string)
	}{
		{
			name: "basic VLESS TCP with TLS",
			params: LinkParams{
				UUID:        "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
				Host:        "example.com",
				Port:        443,
				Remark:      "MyNode-vless",
				Transport:   "tcp",
				Security:    "tls",
				ServerName:  "example.com",
				Fingerprint: "chrome",
			},
			check: func(t *testing.T, link string) {
				if !strings.HasPrefix(link, "vless://") {
					t.Error("link should start with vless://")
				}
				if !strings.Contains(link, "a1b2c3d4-e5f6-7890-abcd-ef1234567890@example.com:443") {
					t.Error("link should contain uuid@host:port")
				}
				if !strings.Contains(link, "type=tcp") {
					t.Error("link should contain type=tcp")
				}
				if !strings.Contains(link, "security=tls") {
					t.Error("link should contain security=tls")
				}
				if !strings.Contains(link, "sni=example.com") {
					t.Error("link should contain sni=example.com")
				}
				if !strings.Contains(link, "fp=chrome") {
					t.Error("link should contain fp=chrome")
				}
				if !strings.Contains(link, "encryption=none") {
					t.Error("link should contain encryption=none")
				}
				if !strings.HasSuffix(link, "#MyNode-vless") {
					t.Errorf("link should end with remark, got: %s", link)
				}
			},
		},
		{
			name: "VLESS WebSocket with path",
			params: LinkParams{
				UUID:       "uuid-ws-test",
				Host:       "ws.example.com",
				Port:       8080,
				Remark:     "WS Node",
				Transport:  "ws",
				Security:   "tls",
				ServerName: "ws.example.com",
				Path:       "/proxy",
			},
			check: func(t *testing.T, link string) {
				if !strings.Contains(link, "type=ws") {
					t.Error("link should contain type=ws")
				}
				if !strings.Contains(link, "path=%2Fproxy") {
					t.Error("link should contain URL-encoded path")
				}
				if !strings.Contains(link, "host=ws.example.com") {
					t.Error("link should contain host for WS transport")
				}
			},
		},
		{
			name: "VLESS Reality",
			params: LinkParams{
				UUID:        "uuid-reality",
				Host:        "1.2.3.4",
				Port:        443,
				Remark:      "Reality Node",
				Transport:   "tcp",
				Security:    "reality",
				ServerName:  "www.google.com",
				Fingerprint: "chrome",
				Flow:        "xtls-rprx-vision",
			},
			check: func(t *testing.T, link string) {
				if !strings.Contains(link, "security=reality") {
					t.Error("link should contain security=reality")
				}
				if !strings.Contains(link, "flow=xtls-rprx-vision") {
					t.Error("link should contain flow parameter")
				}
				if !strings.Contains(link, "sni=www.google.com") {
					t.Error("link should contain SNI")
				}
			},
		},
		{
			name: "VLESS gRPC",
			params: LinkParams{
				UUID:        "uuid-grpc",
				Host:        "grpc.example.com",
				Port:        443,
				Remark:      "gRPC Node",
				Transport:   "grpc",
				Security:    "tls",
				ServerName:  "grpc.example.com",
				ServiceName: "mygrpc",
			},
			check: func(t *testing.T, link string) {
				if !strings.Contains(link, "type=grpc") {
					t.Error("link should contain type=grpc")
				}
				if !strings.Contains(link, "serviceName=mygrpc") {
					t.Error("link should contain serviceName")
				}
			},
		},
		{
			name: "VLESS no security",
			params: LinkParams{
				UUID:      "uuid-none",
				Host:      "10.0.0.1",
				Port:      2053,
				Remark:    "Plain",
				Transport: "tcp",
				Security:  "",
			},
			check: func(t *testing.T, link string) {
				if !strings.Contains(link, "security=none") {
					t.Error("link should contain security=none when empty")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := GenerateVLESSLink(tt.params)
			tt.check(t, link)
		})
	}
}

func TestGenerateVMessLink(t *testing.T) {
	tests := []struct {
		name   string
		params LinkParams
		check  func(t *testing.T, link string)
	}{
		{
			name: "basic VMess TCP TLS",
			params: LinkParams{
				UUID:       "vmess-uuid-1234",
				Host:       "vmess.example.com",
				Port:       443,
				Remark:     "VMess Node",
				Transport:  "tcp",
				Security:   "tls",
				ServerName: "vmess.example.com",
			},
			check: func(t *testing.T, link string) {
				if !strings.HasPrefix(link, "vmess://") {
					t.Fatal("link should start with vmess://")
				}

				encoded := strings.TrimPrefix(link, "vmess://")
				decoded, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil {
					t.Fatalf("failed to decode base64: %v", err)
				}

				var obj map[string]string
				if err := json.Unmarshal(decoded, &obj); err != nil {
					t.Fatalf("failed to unmarshal JSON: %v", err)
				}

				if obj["v"] != "2" {
					t.Error("version should be 2")
				}
				if obj["ps"] != "VMess Node" {
					t.Errorf("ps should be 'VMess Node', got %q", obj["ps"])
				}
				if obj["add"] != "vmess.example.com" {
					t.Errorf("add should be 'vmess.example.com', got %q", obj["add"])
				}
				if obj["port"] != "443" {
					t.Errorf("port should be '443', got %q", obj["port"])
				}
				if obj["id"] != "vmess-uuid-1234" {
					t.Errorf("id should be 'vmess-uuid-1234', got %q", obj["id"])
				}
				if obj["aid"] != "0" {
					t.Errorf("aid should be '0', got %q", obj["aid"])
				}
				if obj["scy"] != "auto" {
					t.Errorf("scy should be 'auto', got %q", obj["scy"])
				}
				if obj["net"] != "tcp" {
					t.Errorf("net should be 'tcp', got %q", obj["net"])
				}
				if obj["type"] != "none" {
					t.Errorf("type should be 'none', got %q", obj["type"])
				}
				if obj["tls"] != "tls" {
					t.Errorf("tls should be 'tls', got %q", obj["tls"])
				}
			},
		},
		{
			name: "VMess WS with path and host",
			params: LinkParams{
				UUID:       "vmess-ws-uuid",
				Host:       "ws.example.com",
				Port:       8080,
				Remark:     "WS VMess",
				Transport:  "ws",
				Security:   "tls",
				ServerName: "ws.example.com",
				Path:       "/vmess-ws",
			},
			check: func(t *testing.T, link string) {
				encoded := strings.TrimPrefix(link, "vmess://")
				decoded, _ := base64.StdEncoding.DecodeString(encoded)
				var obj map[string]string
				json.Unmarshal(decoded, &obj)

				if obj["net"] != "ws" {
					t.Errorf("net should be 'ws', got %q", obj["net"])
				}
				if obj["host"] != "ws.example.com" {
					t.Errorf("host should be 'ws.example.com', got %q", obj["host"])
				}
				if obj["path"] != "/vmess-ws" {
					t.Errorf("path should be '/vmess-ws', got %q", obj["path"])
				}
			},
		},
		{
			name: "VMess gRPC uses serviceName as path",
			params: LinkParams{
				UUID:        "vmess-grpc-uuid",
				Host:        "grpc.example.com",
				Port:        443,
				Remark:      "gRPC VMess",
				Transport:   "grpc",
				Security:    "tls",
				ServerName:  "grpc.example.com",
				ServiceName: "myservice",
			},
			check: func(t *testing.T, link string) {
				encoded := strings.TrimPrefix(link, "vmess://")
				decoded, _ := base64.StdEncoding.DecodeString(encoded)
				var obj map[string]string
				json.Unmarshal(decoded, &obj)

				if obj["net"] != "grpc" {
					t.Errorf("net should be 'grpc', got %q", obj["net"])
				}
				if obj["path"] != "myservice" {
					t.Errorf("path should be 'myservice' for gRPC, got %q", obj["path"])
				}
			},
		},
		{
			name: "VMess no TLS",
			params: LinkParams{
				UUID:      "vmess-notls",
				Host:      "10.0.0.1",
				Port:      8888,
				Remark:    "No TLS",
				Transport: "tcp",
				Security:  "none",
			},
			check: func(t *testing.T, link string) {
				encoded := strings.TrimPrefix(link, "vmess://")
				decoded, _ := base64.StdEncoding.DecodeString(encoded)
				var obj map[string]string
				json.Unmarshal(decoded, &obj)

				if obj["tls"] != "" {
					t.Errorf("tls should be empty for no-TLS, got %q", obj["tls"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := GenerateVMessLink(tt.params)
			tt.check(t, link)
		})
	}
}

func TestGenerateTrojanLink(t *testing.T) {
	tests := []struct {
		name   string
		params LinkParams
		check  func(t *testing.T, link string)
	}{
		{
			name: "basic Trojan TCP TLS",
			params: LinkParams{
				UUID:        "trojan-uuid-1234",
				Host:        "trojan.example.com",
				Port:        443,
				Remark:      "Trojan Node",
				Transport:   "tcp",
				Security:    "tls",
				ServerName:  "trojan.example.com",
				Fingerprint: "firefox",
			},
			check: func(t *testing.T, link string) {
				if !strings.HasPrefix(link, "trojan://") {
					t.Error("link should start with trojan://")
				}
				if !strings.Contains(link, "trojan-uuid-1234@trojan.example.com:443") {
					t.Error("link should contain uuid@host:port")
				}
				if !strings.Contains(link, "type=tcp") {
					t.Error("link should contain type=tcp")
				}
				if !strings.Contains(link, "security=tls") {
					t.Error("link should contain security=tls")
				}
				if !strings.Contains(link, "sni=trojan.example.com") {
					t.Error("link should contain sni")
				}
				if !strings.Contains(link, "fp=firefox") {
					t.Error("link should contain fingerprint")
				}
				if !strings.HasSuffix(link, "#Trojan+Node") || !strings.HasSuffix(link, "#Trojan%20Node") {
					// URL encoding of spaces can be + or %20
					if !strings.Contains(link, "#Trojan") {
						t.Errorf("link should end with remark, got: %s", link)
					}
				}
			},
		},
		{
			name: "Trojan WebSocket",
			params: LinkParams{
				UUID:       "trojan-ws-uuid",
				Host:       "ws.example.com",
				Port:       8443,
				Remark:     "Trojan-WS",
				Transport:  "ws",
				Security:   "tls",
				ServerName: "ws.example.com",
				Path:       "/trojan-ws",
			},
			check: func(t *testing.T, link string) {
				if !strings.Contains(link, "type=ws") {
					t.Error("link should contain type=ws")
				}
				if !strings.Contains(link, "path=") {
					t.Error("link should contain path parameter")
				}
				if !strings.Contains(link, "host=ws.example.com") {
					t.Error("link should contain host for WS transport")
				}
			},
		},
		{
			name: "Trojan default security is TLS",
			params: LinkParams{
				UUID:      "trojan-default",
				Host:      "1.2.3.4",
				Port:      443,
				Remark:    "Default",
				Transport: "tcp",
				Security:  "",
			},
			check: func(t *testing.T, link string) {
				if !strings.Contains(link, "security=tls") {
					t.Error("trojan should default to security=tls")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := GenerateTrojanLink(tt.params)
			tt.check(t, link)
		})
	}
}

func TestGenerateShadowsocksLink(t *testing.T) {
	tests := []struct {
		name   string
		params LinkParams
		check  func(t *testing.T, link string)
	}{
		{
			name: "basic SS with chacha20",
			params: LinkParams{
				UUID:   "ss-password-123",
				Host:   "ss.example.com",
				Port:   8388,
				Remark: "SS Node",
				Method: "chacha20-ietf-poly1305",
			},
			check: func(t *testing.T, link string) {
				if !strings.HasPrefix(link, "ss://") {
					t.Fatal("link should start with ss://")
				}

				// Parse the link
				trimmed := strings.TrimPrefix(link, "ss://")
				parts := strings.SplitN(trimmed, "@", 2)
				if len(parts) != 2 {
					t.Fatal("link should contain @ separator")
				}

				// Decode the user info (add padding back)
				userInfo := parts[0]
				// Add padding
				padLen := 4 - len(userInfo)%4
				if padLen < 4 {
					userInfo += strings.Repeat("=", padLen)
				}
				decoded, err := base64.URLEncoding.DecodeString(userInfo)
				if err != nil {
					t.Fatalf("failed to decode user info: %v", err)
				}

				expected := "chacha20-ietf-poly1305:ss-password-123"
				if string(decoded) != expected {
					t.Errorf("decoded userinfo should be %q, got %q", expected, string(decoded))
				}

				if !strings.Contains(parts[1], "ss.example.com:8388") {
					t.Error("link should contain host:port")
				}
				if !strings.Contains(link, "#SS+Node") && !strings.Contains(link, "#SS%20Node") {
					t.Errorf("link should end with remark, got: %s", link)
				}
			},
		},
		{
			name: "SS with aes-256-gcm",
			params: LinkParams{
				UUID:   "uuid-aes",
				Host:   "10.0.0.1",
				Port:   1234,
				Remark: "AES-Node",
				Method: "aes-256-gcm",
			},
			check: func(t *testing.T, link string) {
				trimmed := strings.TrimPrefix(link, "ss://")
				parts := strings.SplitN(trimmed, "@", 2)
				userInfo := parts[0]
				padLen := 4 - len(userInfo)%4
				if padLen < 4 {
					userInfo += strings.Repeat("=", padLen)
				}
				decoded, _ := base64.URLEncoding.DecodeString(userInfo)
				if !strings.HasPrefix(string(decoded), "aes-256-gcm:") {
					t.Error("should use aes-256-gcm method")
				}
			},
		},
		{
			name: "SS default method is chacha20-ietf-poly1305",
			params: LinkParams{
				UUID:   "uuid-default",
				Host:   "1.2.3.4",
				Port:   8388,
				Remark: "Default",
				Method: "",
			},
			check: func(t *testing.T, link string) {
				trimmed := strings.TrimPrefix(link, "ss://")
				parts := strings.SplitN(trimmed, "@", 2)
				userInfo := parts[0]
				padLen := 4 - len(userInfo)%4
				if padLen < 4 {
					userInfo += strings.Repeat("=", padLen)
				}
				decoded, _ := base64.URLEncoding.DecodeString(userInfo)
				if !strings.HasPrefix(string(decoded), "chacha20-ietf-poly1305:") {
					t.Errorf("should default to chacha20, got: %s", string(decoded))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := GenerateShadowsocksLink(tt.params)
			tt.check(t, link)
		})
	}
}

func TestGenerateAllLinks(t *testing.T) {
	params := LinkParams{
		UUID:       "all-uuid",
		Host:       "multi.example.com",
		Port:       443,
		Remark:     "Multi",
		Transport:  "tcp",
		Security:   "tls",
		ServerName: "multi.example.com",
		Method:     "aes-256-gcm",
	}

	t.Run("all four protocols", func(t *testing.T) {
		protocols := []string{ProtocolVLESS, ProtocolVMess, ProtocolTrojan, ProtocolShadowsocks}
		configs := GenerateAllLinks(params, protocols)

		if len(configs) != 4 {
			t.Fatalf("expected 4 configs, got %d", len(configs))
		}

		expectedPrefixes := map[string]string{
			ProtocolVLESS:       "vless://",
			ProtocolVMess:       "vmess://",
			ProtocolTrojan:      "trojan://",
			ProtocolShadowsocks: "ss://",
		}

		for _, cfg := range configs {
			prefix, ok := expectedPrefixes[cfg.Protocol]
			if !ok {
				t.Errorf("unexpected protocol: %s", cfg.Protocol)
				continue
			}
			if !strings.HasPrefix(cfg.Link, prefix) {
				t.Errorf("protocol %s link should start with %s, got: %s", cfg.Protocol, prefix, cfg.Link)
			}
			if cfg.QRData != cfg.Link {
				t.Error("QRData should equal Link")
			}
		}
	})

	t.Run("subset of protocols", func(t *testing.T) {
		protocols := []string{ProtocolVLESS, ProtocolTrojan}
		configs := GenerateAllLinks(params, protocols)

		if len(configs) != 2 {
			t.Fatalf("expected 2 configs, got %d", len(configs))
		}
		if configs[0].Protocol != ProtocolVLESS {
			t.Error("first config should be VLESS")
		}
		if configs[1].Protocol != ProtocolTrojan {
			t.Error("second config should be Trojan")
		}
	})

	t.Run("unknown protocol is skipped", func(t *testing.T) {
		protocols := []string{"unknown", ProtocolVLESS}
		configs := GenerateAllLinks(params, protocols)

		if len(configs) != 1 {
			t.Fatalf("expected 1 config, got %d", len(configs))
		}
		if configs[0].Protocol != ProtocolVLESS {
			t.Error("should only contain VLESS")
		}
	})

	t.Run("empty protocols returns empty", func(t *testing.T) {
		configs := GenerateAllLinks(params, []string{})
		if len(configs) != 0 {
			t.Error("expected empty configs")
		}
	})
}

func TestVLESSLinkParseable(t *testing.T) {
	// Ensure generated VLESS links are valid URLs
	params := LinkParams{
		UUID:        "test-uuid-1234",
		Host:        "example.com",
		Port:        443,
		Remark:      "Test Node (Special Chars!)",
		Transport:   "ws",
		Security:    "tls",
		ServerName:  "example.com",
		Path:        "/path/to/ws",
		Fingerprint: "chrome",
	}

	link := GenerateVLESSLink(params)

	// Remove vless:// prefix and parse as URL
	withScheme := strings.Replace(link, "vless://", "https://", 1)
	parsed, err := url.Parse(withScheme)
	if err != nil {
		t.Fatalf("generated link is not parseable as URL: %v", err)
	}

	if parsed.Host != "example.com:443" {
		t.Errorf("host should be example.com:443, got %s", parsed.Host)
	}

	query := parsed.Query()
	if query.Get("type") != "ws" {
		t.Error("type should be ws")
	}
	if query.Get("security") != "tls" {
		t.Error("security should be tls")
	}
	if query.Get("path") != "/path/to/ws" {
		t.Errorf("path should be '/path/to/ws', got %q", query.Get("path"))
	}
}
