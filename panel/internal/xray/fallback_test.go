//go:build !lite

package xray

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBuildFallbackChain(t *testing.T) {
	t.Run("valid config produces correct inbound array", func(t *testing.T) {
		config := MultiPortConfig{
			Port:         443,
			MainProtocol: ProtocolVLESS,
			TLS: &TLSConfig{
				CertPath:   "/etc/ssl/cert.pem",
				KeyPath:    "/etc/ssl/key.pem",
				ServerName: "example.com",
				ALPN:       []string{"h2", "http/1.1"},
			},
			Fallbacks: []FallbackConfig{
				{Dest: "31001", Path: "/vmess-ws", Xver: 1},
				{Dest: "31002", Path: "/trojan-ws", Xver: 1},
				{Dest: "31003", ALPN: "h2", Xver: 1},
				{Dest: "80", Xver: 0},
			},
		}

		result, err := BuildFallbackChain(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Parse the result as a JSON array.
		var inbounds []json.RawMessage
		if err := json.Unmarshal(result, &inbounds); err != nil {
			t.Fatalf("failed to parse result as JSON array: %v", err)
		}

		// Should have main inbound + 2 helper inbounds (for path-based fallbacks).
		if len(inbounds) != 3 {
			t.Fatalf("expected 3 inbounds (main + 2 helpers), got %d", len(inbounds))
		}

		// Verify the main inbound structure.
		var main fallbackInboundJSON
		if err := json.Unmarshal(inbounds[0], &main); err != nil {
			t.Fatalf("failed to parse main inbound: %v", err)
		}

		if main.Port != 443 {
			t.Errorf("main port should be 443, got %d", main.Port)
		}
		if main.Protocol != "vless" {
			t.Errorf("main protocol should be 'vless', got %q", main.Protocol)
		}
		if main.Listen != "0.0.0.0" {
			t.Errorf("main listen should be '0.0.0.0', got %q", main.Listen)
		}
		if main.Settings.Decryption != "none" {
			t.Errorf("decryption should be 'none', got %q", main.Settings.Decryption)
		}
		if len(main.Settings.Fallbacks) != 4 {
			t.Fatalf("expected 4 fallback entries, got %d", len(main.Settings.Fallbacks))
		}
		if main.Tag != "main-fallback" {
			t.Errorf("tag should be 'main-fallback', got %q", main.Tag)
		}

		// Verify first fallback entry.
		fb0 := main.Settings.Fallbacks[0]
		if fb0.Path != "/vmess-ws" {
			t.Errorf("first fallback path should be '/vmess-ws', got %q", fb0.Path)
		}
		if fb0.Xver != 1 {
			t.Errorf("first fallback xver should be 1, got %d", fb0.Xver)
		}

		// Verify default fallback (last entry).
		fb3 := main.Settings.Fallbacks[3]
		if fb3.Path != "" {
			t.Errorf("default fallback should have no path, got %q", fb3.Path)
		}
		if fb3.ALPN != "" {
			t.Errorf("default fallback should have no ALPN, got %q", fb3.ALPN)
		}

		// Verify stream settings has TLS.
		var streamCheck struct {
			Network     string `json:"network"`
			Security    string `json:"security"`
			TLSSettings struct {
				ServerName   string `json:"serverName"`
				Certificates []struct {
					CertFile string `json:"certificateFile"`
					KeyFile  string `json:"keyFile"`
				} `json:"certificates"`
				ALPN []string `json:"alpn"`
			} `json:"tlsSettings"`
		}
		if err := json.Unmarshal(main.StreamSettings, &streamCheck); err != nil {
			t.Fatalf("failed to parse stream settings: %v", err)
		}
		if streamCheck.Network != "tcp" {
			t.Errorf("network should be 'tcp', got %q", streamCheck.Network)
		}
		if streamCheck.Security != "tls" {
			t.Errorf("security should be 'tls', got %q", streamCheck.Security)
		}
		if streamCheck.TLSSettings.ServerName != "example.com" {
			t.Errorf("serverName should be 'example.com', got %q", streamCheck.TLSSettings.ServerName)
		}
		if len(streamCheck.TLSSettings.ALPN) != 2 {
			t.Errorf("ALPN should have 2 entries, got %d", len(streamCheck.TLSSettings.ALPN))
		}

		// Verify first helper inbound (vmess-ws).
		var helper1 helperInboundJSON
		if err := json.Unmarshal(inbounds[1], &helper1); err != nil {
			t.Fatalf("failed to parse helper 1: %v", err)
		}
		if helper1.Listen != "127.0.0.1" {
			t.Errorf("helper 1 listen should be '127.0.0.1', got %q", helper1.Listen)
		}
		if helper1.Port != 31001 {
			t.Errorf("helper 1 port should be 31001, got %d", helper1.Port)
		}
		if helper1.Protocol != "vmess" {
			t.Errorf("helper 1 protocol should be 'vmess', got %q", helper1.Protocol)
		}
		if helper1.Tag != "vmess-ws-in" {
			t.Errorf("helper 1 tag should be 'vmess-ws-in', got %q", helper1.Tag)
		}

		// Verify second helper inbound (trojan-ws).
		var helper2 helperInboundJSON
		if err := json.Unmarshal(inbounds[2], &helper2); err != nil {
			t.Fatalf("failed to parse helper 2: %v", err)
		}
		if helper2.Listen != "127.0.0.1" {
			t.Errorf("helper 2 listen should be '127.0.0.1', got %q", helper2.Listen)
		}
		if helper2.Port != 31002 {
			t.Errorf("helper 2 port should be 31002, got %d", helper2.Port)
		}
		if helper2.Protocol != "trojan" {
			t.Errorf("helper 2 protocol should be 'trojan', got %q", helper2.Protocol)
		}
		if helper2.Tag != "trojan-ws-in" {
			t.Errorf("helper 2 tag should be 'trojan-ws-in', got %q", helper2.Tag)
		}
	})

	t.Run("reality config produces correct stream settings", func(t *testing.T) {
		config := MultiPortConfig{
			Port: 443,
			Reality: &RealityConfig{
				ServerNames: []string{"www.google.com"},
				PrivateKey:  "test-private-key",
				ShortIDs:    []string{"abcdef12"},
			},
			Fallbacks: []FallbackConfig{
				{Dest: "80"},
			},
		}

		result, err := BuildFallbackChain(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var inbounds []json.RawMessage
		json.Unmarshal(result, &inbounds)

		var main struct {
			StreamSettings json.RawMessage `json:"streamSettings"`
		}
		json.Unmarshal(inbounds[0], &main)

		var stream struct {
			Security        string `json:"security"`
			RealitySettings struct {
				ServerNames []string `json:"serverNames"`
				PrivateKey  string   `json:"privateKey"`
				ShortIDs    []string `json:"shortIds"`
			} `json:"realitySettings"`
		}
		if err := json.Unmarshal(main.StreamSettings, &stream); err != nil {
			t.Fatalf("failed to parse stream settings: %v", err)
		}

		if stream.Security != "reality" {
			t.Errorf("security should be 'reality', got %q", stream.Security)
		}
		if len(stream.RealitySettings.ServerNames) != 1 || stream.RealitySettings.ServerNames[0] != "www.google.com" {
			t.Errorf("serverNames should be [www.google.com], got %v", stream.RealitySettings.ServerNames)
		}
		if stream.RealitySettings.PrivateKey != "test-private-key" {
			t.Errorf("privateKey mismatch")
		}
	})

	t.Run("defaults main protocol to vless", func(t *testing.T) {
		config := MultiPortConfig{
			Port:         443,
			MainProtocol: "", // should default to vless
			Fallbacks:    []FallbackConfig{{Dest: "80"}},
		}

		result, err := BuildFallbackChain(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var inbounds []json.RawMessage
		json.Unmarshal(result, &inbounds)

		var main struct {
			Protocol string `json:"protocol"`
		}
		json.Unmarshal(inbounds[0], &main)

		if main.Protocol != "vless" {
			t.Errorf("protocol should default to 'vless', got %q", main.Protocol)
		}
	})

	t.Run("error on zero port", func(t *testing.T) {
		config := MultiPortConfig{
			Port:      0,
			Fallbacks: []FallbackConfig{{Dest: "80"}},
		}
		_, err := BuildFallbackChain(config)
		if err == nil {
			t.Fatal("expected error for zero port")
		}
	})

	t.Run("error on empty fallbacks", func(t *testing.T) {
		config := MultiPortConfig{
			Port:      443,
			Fallbacks: []FallbackConfig{},
		}
		_, err := BuildFallbackChain(config)
		if err == nil {
			t.Fatal("expected error for empty fallbacks")
		}
	})

	t.Run("no TLS or Reality produces security none", func(t *testing.T) {
		config := MultiPortConfig{
			Port:      8080,
			Fallbacks: []FallbackConfig{{Dest: "80"}},
		}

		result, err := BuildFallbackChain(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var inbounds []json.RawMessage
		json.Unmarshal(result, &inbounds)

		var main struct {
			StreamSettings json.RawMessage `json:"streamSettings"`
		}
		json.Unmarshal(inbounds[0], &main)

		var stream struct {
			Security string `json:"security"`
		}
		json.Unmarshal(main.StreamSettings, &stream)

		if stream.Security != "none" {
			t.Errorf("security should be 'none' without TLS/Reality, got %q", stream.Security)
		}
	})
}

func TestGenerateDefaultFallbackConfig(t *testing.T) {
	t.Run("vmess and trojan protocols", func(t *testing.T) {
		config := GenerateDefaultFallbackConfig([]string{ProtocolVMess, ProtocolTrojan})

		if config.Port != 443 {
			t.Errorf("port should be 443, got %d", config.Port)
		}
		if config.MainProtocol != ProtocolVLESS {
			t.Errorf("main protocol should be 'vless', got %q", config.MainProtocol)
		}

		// Should have vmess fallback + trojan fallback + default catch-all.
		if len(config.Fallbacks) != 3 {
			t.Fatalf("expected 3 fallbacks, got %d", len(config.Fallbacks))
		}

		// VMess fallback.
		if config.Fallbacks[0].Path != "/vmess-ws" {
			t.Errorf("first fallback path should be '/vmess-ws', got %q", config.Fallbacks[0].Path)
		}
		if config.Fallbacks[0].Dest != "31001" {
			t.Errorf("first fallback dest should be '31001', got %q", config.Fallbacks[0].Dest)
		}
		if config.Fallbacks[0].Xver != 1 {
			t.Errorf("first fallback xver should be 1, got %d", config.Fallbacks[0].Xver)
		}

		// Trojan fallback.
		if config.Fallbacks[1].Path != "/trojan-ws" {
			t.Errorf("second fallback path should be '/trojan-ws', got %q", config.Fallbacks[1].Path)
		}
		if config.Fallbacks[1].Dest != "31002" {
			t.Errorf("second fallback dest should be '31002', got %q", config.Fallbacks[1].Dest)
		}

		// Default catch-all.
		if config.Fallbacks[2].Dest != "80" {
			t.Errorf("default fallback dest should be '80', got %q", config.Fallbacks[2].Dest)
		}
		if config.Fallbacks[2].Path != "" {
			t.Errorf("default fallback should have no path, got %q", config.Fallbacks[2].Path)
		}
		if config.Fallbacks[2].Xver != 0 {
			t.Errorf("default fallback xver should be 0, got %d", config.Fallbacks[2].Xver)
		}
	})

	t.Run("skips vless as fallback target", func(t *testing.T) {
		config := GenerateDefaultFallbackConfig([]string{ProtocolVLESS, ProtocolVMess})

		// VLESS is the main protocol, should not appear as fallback.
		// Should have vmess + default catch-all = 2.
		if len(config.Fallbacks) != 2 {
			t.Fatalf("expected 2 fallbacks (vmess + catch-all), got %d", len(config.Fallbacks))
		}
		if config.Fallbacks[0].Path != "/vmess-ws" {
			t.Errorf("first fallback should be vmess-ws, got %q", config.Fallbacks[0].Path)
		}
	})

	t.Run("empty protocols produces only default fallback", func(t *testing.T) {
		config := GenerateDefaultFallbackConfig([]string{})

		if len(config.Fallbacks) != 1 {
			t.Fatalf("expected 1 fallback (catch-all only), got %d", len(config.Fallbacks))
		}
		if config.Fallbacks[0].Dest != "80" {
			t.Errorf("default fallback dest should be '80', got %q", config.Fallbacks[0].Dest)
		}
	})

	t.Run("shadowsocks protocol", func(t *testing.T) {
		config := GenerateDefaultFallbackConfig([]string{ProtocolShadowsocks})

		if len(config.Fallbacks) != 2 {
			t.Fatalf("expected 2 fallbacks, got %d", len(config.Fallbacks))
		}
		if config.Fallbacks[0].Path != "/ss-ws" {
			t.Errorf("first fallback path should be '/ss-ws', got %q", config.Fallbacks[0].Path)
		}
	})

	t.Run("all protocols get sequential ports", func(t *testing.T) {
		config := GenerateDefaultFallbackConfig([]string{ProtocolVMess, ProtocolTrojan, ProtocolShadowsocks})

		// 3 protocol fallbacks + 1 catch-all.
		if len(config.Fallbacks) != 4 {
			t.Fatalf("expected 4 fallbacks, got %d", len(config.Fallbacks))
		}
		if config.Fallbacks[0].Dest != "31001" {
			t.Errorf("first dest should be '31001', got %q", config.Fallbacks[0].Dest)
		}
		if config.Fallbacks[1].Dest != "31002" {
			t.Errorf("second dest should be '31002', got %q", config.Fallbacks[1].Dest)
		}
		if config.Fallbacks[2].Dest != "31003" {
			t.Errorf("third dest should be '31003', got %q", config.Fallbacks[2].Dest)
		}
	})
}

func TestSetupMultiPort(t *testing.T) {
	t.Run("saves multi-port config to new node", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		// GetConfig will return not found.
		mock.ExpectQuery("SELECT node_id, enabled, config_json, reality_config_json").
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{
				"node_id", "enabled", "config_json", "reality_config_json",
				"last_synced_at", "created_at", "updated_at",
			}))

		// SaveConfig upsert.
		mock.ExpectExec("INSERT INTO xray_configs").
			WithArgs(
				int64(1),
				true,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		config := MultiPortConfig{
			Port: 443,
			TLS: &TLSConfig{
				CertPath:   "/etc/ssl/cert.pem",
				KeyPath:    "/etc/ssl/key.pem",
				ServerName: "example.com",
			},
			Fallbacks: []FallbackConfig{
				{Dest: "31001", Path: "/vmess-ws", Xver: 1},
				{Dest: "80"},
			},
		}

		err = svc.SetupMultiPort(context.Background(), 1, config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("error on zero port", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		config := MultiPortConfig{
			Port:      0,
			Fallbacks: []FallbackConfig{{Dest: "80"}},
		}

		err = svc.SetupMultiPort(context.Background(), 1, config)
		if err == nil {
			t.Fatal("expected error for zero port")
		}
	})

	t.Run("error on empty fallbacks", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		config := MultiPortConfig{
			Port:      443,
			Fallbacks: []FallbackConfig{},
		}

		err = svc.SetupMultiPort(context.Background(), 1, config)
		if err == nil {
			t.Fatal("expected error for empty fallbacks")
		}
	})
}

func TestParseFallbackDest(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType string
		wantVal  interface{}
	}{
		{"numeric port", "31001", "int", 31001},
		{"port 80", "80", "int", 80},
		{"address with port", "127.0.0.1:8080", "string", "127.0.0.1:8080"},
		{"unix socket", "/var/run/proxy.sock", "string", "/var/run/proxy.sock"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFallbackDest(tt.input)
			switch tt.wantType {
			case "int":
				v, ok := result.(int)
				if !ok {
					t.Fatalf("expected int, got %T", result)
				}
				if v != tt.wantVal.(int) {
					t.Errorf("expected %d, got %d", tt.wantVal.(int), v)
				}
			case "string":
				v, ok := result.(string)
				if !ok {
					t.Fatalf("expected string, got %T", result)
				}
				if v != tt.wantVal.(string) {
					t.Errorf("expected %q, got %q", tt.wantVal.(string), v)
				}
			}
		})
	}
}

func TestInferProtocolFromPath(t *testing.T) {
	tests := []struct {
		path         string
		wantProtocol string
		wantTag      string
	}{
		{"/vmess-ws", ProtocolVMess, "vmess-ws-in"},
		{"/trojan-ws", ProtocolTrojan, "trojan-ws-in"},
		{"/vless-ws", ProtocolVLESS, "vless-ws-in"},
		{"/ss-ws", ProtocolShadowsocks, "ss-ws-in"},
		{"/shadowsocks", ProtocolShadowsocks, "ss-ws-in"},
		{"/unknown-path", ProtocolVMess, "fallback-ws-in"},
		{"/VMess-WS", ProtocolVMess, "vmess-ws-in"}, // case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			proto, tag := inferProtocolFromPath(tt.path)
			if proto != tt.wantProtocol {
				t.Errorf("protocol: expected %q, got %q", tt.wantProtocol, proto)
			}
			if tag != tt.wantTag {
				t.Errorf("tag: expected %q, got %q", tt.wantTag, tag)
			}
		})
	}
}
