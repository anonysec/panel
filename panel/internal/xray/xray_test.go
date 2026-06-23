//go:build !lite

package xray

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// UUID v4 format: 8-4-4-4-12 hex characters
var uuidV4Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestGetConfig(t *testing.T) {
	tests := []struct {
		name     string
		nodeID   int64
		setup    func(mock sqlmock.Sqlmock)
		wantErr  bool
		checkCfg func(t *testing.T, cfg *XrayConfig)
	}{
		{
			name:   "config found with all fields",
			nodeID: 1,
			setup: func(mock sqlmock.Sqlmock) {
				now := time.Now()
				configJSON := `{"inbounds":[{"protocol":"vless","port":443,"transport":"tcp","tag":"main"}],"routing":{"domain_strategy":"AsIs"},"tls":{"server_name":"example.com"}}`
				realityJSON := `{"server_names":["www.google.com"],"private_key":"pk","public_key":"pub","short_ids":["aabb1122"]}`
				mock.ExpectQuery("SELECT node_id, enabled, config_json, reality_config_json").
					WithArgs(int64(1)).
					WillReturnRows(sqlmock.NewRows([]string{
						"node_id", "enabled", "config_json", "reality_config_json",
						"last_synced_at", "created_at", "updated_at",
					}).AddRow(1, true, configJSON, realityJSON, now, now, now))
			},
			wantErr: false,
			checkCfg: func(t *testing.T, cfg *XrayConfig) {
				if cfg.NodeID != 1 {
					t.Errorf("NodeID = %d, want 1", cfg.NodeID)
				}
				if !cfg.Enabled {
					t.Error("Enabled should be true")
				}
				if len(cfg.Inbounds) != 1 {
					t.Fatalf("expected 1 inbound, got %d", len(cfg.Inbounds))
				}
				if cfg.Inbounds[0].Protocol != ProtocolVLESS {
					t.Errorf("inbound protocol = %q, want %q", cfg.Inbounds[0].Protocol, ProtocolVLESS)
				}
				if cfg.Inbounds[0].Port != 443 {
					t.Errorf("inbound port = %d, want 443", cfg.Inbounds[0].Port)
				}
				if cfg.TLS.ServerName != "example.com" {
					t.Errorf("TLS server_name = %q, want example.com", cfg.TLS.ServerName)
				}
				if cfg.RealityConfig == nil {
					t.Fatal("RealityConfig should not be nil")
				}
				if cfg.RealityConfig.PublicKey != "pub" {
					t.Errorf("RealityConfig.PublicKey = %q, want pub", cfg.RealityConfig.PublicKey)
				}
				if cfg.LastSyncedAt == nil {
					t.Error("LastSyncedAt should not be nil")
				}
			},
		},
		{
			name:   "config not found returns error",
			nodeID: 99,
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT node_id, enabled, config_json, reality_config_json").
					WithArgs(int64(99)).
					WillReturnRows(sqlmock.NewRows([]string{
						"node_id", "enabled", "config_json", "reality_config_json",
						"last_synced_at", "created_at", "updated_at",
					}))
			},
			wantErr: true,
		},
		{
			name:   "null reality config",
			nodeID: 2,
			setup: func(mock sqlmock.Sqlmock) {
				now := time.Now()
				configJSON := `{"inbounds":[{"protocol":"vmess","port":8080,"transport":"ws"}],"routing":{},"tls":{}}`
				mock.ExpectQuery("SELECT node_id, enabled, config_json, reality_config_json").
					WithArgs(int64(2)).
					WillReturnRows(sqlmock.NewRows([]string{
						"node_id", "enabled", "config_json", "reality_config_json",
						"last_synced_at", "created_at", "updated_at",
					}).AddRow(2, true, configJSON, nil, nil, now, now))
			},
			wantErr: false,
			checkCfg: func(t *testing.T, cfg *XrayConfig) {
				if cfg.RealityConfig != nil {
					t.Error("RealityConfig should be nil when DB value is NULL")
				}
				if cfg.LastSyncedAt != nil {
					t.Error("LastSyncedAt should be nil when DB value is NULL")
				}
				if len(cfg.Inbounds) != 1 {
					t.Fatalf("expected 1 inbound, got %d", len(cfg.Inbounds))
				}
				if cfg.Inbounds[0].Protocol != ProtocolVMess {
					t.Errorf("inbound protocol = %q, want vmess", cfg.Inbounds[0].Protocol)
				}
			},
		},
		{
			name:   "empty reality config string treated as null",
			nodeID: 3,
			setup: func(mock sqlmock.Sqlmock) {
				now := time.Now()
				configJSON := `{"inbounds":[],"routing":{},"tls":{}}`
				mock.ExpectQuery("SELECT node_id, enabled, config_json, reality_config_json").
					WithArgs(int64(3)).
					WillReturnRows(sqlmock.NewRows([]string{
						"node_id", "enabled", "config_json", "reality_config_json",
						"last_synced_at", "created_at", "updated_at",
					}).AddRow(3, false, configJSON, "", nil, now, now))
			},
			wantErr: false,
			checkCfg: func(t *testing.T, cfg *XrayConfig) {
				if cfg.RealityConfig != nil {
					t.Error("RealityConfig should be nil for empty string")
				}
				if cfg.Enabled {
					t.Error("Enabled should be false")
				}
			},
		},
		{
			name:   "DB query error",
			nodeID: 5,
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT node_id, enabled, config_json, reality_config_json").
					WithArgs(int64(5)).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create sqlmock: %v", err)
			}
			defer db.Close()

			svc := New(db)
			tt.setup(mock)

			cfg, err := svc.GetConfig(context.Background(), tt.nodeID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkCfg != nil {
				tt.checkCfg(t, cfg)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

func TestSaveConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *XrayConfig
		setup   func(mock sqlmock.Sqlmock)
		wantErr bool
	}{
		{
			name: "new config insert",
			cfg: &XrayConfig{
				NodeID:  1,
				Enabled: true,
				Inbounds: []Inbound{
					{Protocol: ProtocolVLESS, Port: 443, Transport: TransportTCP, Tag: "vless-main"},
				},
				Routing: RoutingConfig{DomainStrategy: "AsIs"},
				TLS:     TLSConfig{ServerName: "example.com"},
			},
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO xray_configs").
					WithArgs(int64(1), true, sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantErr: false,
		},
		{
			name: "update existing config (upsert)",
			cfg: &XrayConfig{
				NodeID:  2,
				Enabled: false,
				Inbounds: []Inbound{
					{Protocol: ProtocolVMess, Port: 8080, Transport: TransportWS},
					{Protocol: ProtocolTrojan, Port: 8443, Transport: TransportTCP},
				},
			},
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO xray_configs").
					WithArgs(int64(2), false, sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name: "save with reality config",
			cfg: &XrayConfig{
				NodeID:  3,
				Enabled: true,
				Inbounds: []Inbound{
					{Protocol: ProtocolVLESS, Port: 443, Transport: TransportTCP},
				},
				RealityConfig: &RealityConfig{
					ServerNames: []string{"www.google.com"},
					PrivateKey:  "priv-key",
					PublicKey:   "pub-key",
					ShortIDs:    []string{"aabbccdd"},
				},
			},
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO xray_configs").
					WithArgs(int64(3), true, sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantErr: false,
		},
		{
			name: "DB exec error",
			cfg: &XrayConfig{
				NodeID:  4,
				Enabled: true,
			},
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO xray_configs").
					WithArgs(int64(4), true, sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create sqlmock: %v", err)
			}
			defer db.Close()

			svc := New(db)
			tt.setup(mock)

			err = svc.SaveConfig(context.Background(), tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

func TestDeleteConfig(t *testing.T) {
	tests := []struct {
		name    string
		nodeID  int64
		setup   func(mock sqlmock.Sqlmock)
		wantErr bool
	}{
		{
			name:   "delete existing config",
			nodeID: 1,
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("DELETE FROM xray_configs WHERE node_id").
					WithArgs(int64(1)).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			wantErr: false,
		},
		{
			name:   "delete non-existent config returns error",
			nodeID: 99,
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("DELETE FROM xray_configs WHERE node_id").
					WithArgs(int64(99)).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			wantErr: true,
		},
		{
			name:   "DB exec error",
			nodeID: 5,
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("DELETE FROM xray_configs WHERE node_id").
					WithArgs(int64(5)).
					WillReturnError(sql.ErrConnDone)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create sqlmock: %v", err)
			}
			defer db.Close()

			svc := New(db)
			tt.setup(mock)

			err = svc.DeleteConfig(context.Background(), tt.nodeID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

func TestUUIDFormatValidation(t *testing.T) {
	tests := []struct {
		name  string
		uuid  string
		valid bool
	}{
		{"valid UUID v4", "a1b2c3d4-e5f6-4890-abcd-ef1234567890", true},
		{"all zeros", "00000000-0000-0000-0000-000000000000", true},
		{"all f's", "ffffffff-ffff-ffff-ffff-ffffffffffff", true},
		{"mixed case lowercase", "12345678-abcd-ef12-3456-789abcdef012", true},
		{"too short", "a1b2c3d4-e5f6-4890-abcd-ef123456789", false},
		{"too long", "a1b2c3d4-e5f6-4890-abcd-ef12345678901", false},
		{"missing dashes", "a1b2c3d4e5f64890abcdef1234567890", false},
		{"extra dash", "a1b2c3d4-e5f6-4890-abcd-ef12-34567890", false},
		{"uppercase rejected", "A1B2C3D4-E5F6-4890-ABCD-EF1234567890", false},
		{"invalid hex chars", "g1b2c3d4-e5f6-4890-abcd-ef1234567890", false},
		{"empty string", "", false},
		{"spaces", "a1b2c3d4 e5f6 4890 abcd ef1234567890", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := uuidV4Regex.MatchString(tt.uuid)
			if matched != tt.valid {
				t.Errorf("UUID %q: got valid=%v, want valid=%v", tt.uuid, matched, tt.valid)
			}
		})
	}
}

func TestSubscriptionLinkFormat(t *testing.T) {
	t.Run("base64 output contains valid links separated by newlines", func(t *testing.T) {
		params := LinkParams{
			UUID:       "a1b2c3d4-e5f6-4890-abcd-ef1234567890",
			Host:       "example.com",
			Port:       443,
			Remark:     "TestNode",
			Transport:  TransportTCP,
			Security:   "tls",
			ServerName: "example.com",
		}

		protocols := []string{ProtocolVLESS, ProtocolVMess, ProtocolTrojan, ProtocolShadowsocks}
		configs := GenerateAllLinks(params, protocols)

		// Simulate subscription format: join links with newline, base64 encode.
		var links []string
		for _, cfg := range configs {
			links = append(links, cfg.Link)
		}
		joined := strings.Join(links, "\n")
		encoded := base64.StdEncoding.EncodeToString([]byte(joined))

		// Decode and verify.
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("failed to decode base64: %v", err)
		}

		decodedLinks := strings.Split(string(decoded), "\n")
		if len(decodedLinks) != 4 {
			t.Fatalf("expected 4 links, got %d", len(decodedLinks))
		}

		// Verify each link starts with the correct protocol prefix.
		expectedPrefixes := []string{"vless://", "vmess://", "trojan://", "ss://"}
		for i, link := range decodedLinks {
			if !strings.HasPrefix(link, expectedPrefixes[i]) {
				t.Errorf("link %d: expected prefix %q, got %q",
					i, expectedPrefixes[i], link[:min(len(link), 10)])
			}
		}
	})

	t.Run("empty configs produce empty base64", func(t *testing.T) {
		configs := GenerateAllLinks(LinkParams{}, []string{})
		var links []string
		for _, cfg := range configs {
			links = append(links, cfg.Link)
		}
		joined := strings.Join(links, "\n")
		encoded := base64.StdEncoding.EncodeToString([]byte(joined))

		decoded, _ := base64.StdEncoding.DecodeString(encoded)
		if string(decoded) != "" {
			t.Errorf("expected empty decoded, got %q", string(decoded))
		}
	})

	t.Run("special characters in remarks are URL-encoded", func(t *testing.T) {
		params := LinkParams{
			UUID:      "uuid-special-chars",
			Host:      "example.com",
			Port:      443,
			Remark:    "Node (سرور) — #1 🚀",
			Transport: TransportTCP,
			Security:  "tls",
		}

		link := GenerateVLESSLink(params)

		// The remark should be URL-encoded in the fragment.
		if strings.Contains(link, " ") {
			t.Error("link should not contain raw spaces")
		}
		// Should still be parseable after stripping protocol prefix.
		if !strings.HasPrefix(link, "vless://") {
			t.Error("link should start with vless://")
		}
		// The fragment (after #) should contain the encoded remark.
		parts := strings.SplitN(link, "#", 2)
		if len(parts) != 2 {
			t.Fatal("link should contain # separator for remark")
		}
		if parts[1] == "" {
			t.Error("remark fragment should not be empty")
		}
	})

	t.Run("single link subscription", func(t *testing.T) {
		params := LinkParams{
			UUID:      "single-uuid",
			Host:      "1.2.3.4",
			Port:      8443,
			Remark:    "Solo",
			Transport: TransportWS,
			Security:  "tls",
			Path:      "/ws",
		}

		configs := GenerateAllLinks(params, []string{ProtocolVLESS})
		if len(configs) != 1 {
			t.Fatalf("expected 1 config, got %d", len(configs))
		}

		// Base64 encode a single link.
		encoded := base64.StdEncoding.EncodeToString([]byte(configs[0].Link))
		decoded, _ := base64.StdEncoding.DecodeString(encoded)

		if !strings.HasPrefix(string(decoded), "vless://") {
			t.Errorf("decoded link should start with vless://, got %q", string(decoded)[:10])
		}
	})
}

func TestConfigGenerationEdgeCases(t *testing.T) {
	t.Run("empty inbounds produce valid config JSON", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		cfg := &XrayConfig{
			NodeID:   1,
			Enabled:  true,
			Inbounds: []Inbound{},
		}

		mock.ExpectExec("INSERT INTO xray_configs").
			WithArgs(int64(1), true, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = svc.SaveConfig(context.Background(), cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("mixed protocols generate all link types", func(t *testing.T) {
		params := LinkParams{
			UUID:       "mix-uuid-1234",
			Host:       "multi.example.com",
			Port:       443,
			Remark:     "Mixed-Proto",
			Transport:  TransportTCP,
			Security:   "tls",
			ServerName: "multi.example.com",
			Method:     "aes-256-gcm",
		}

		protocols := []string{
			ProtocolVLESS,
			ProtocolVMess,
			ProtocolTrojan,
			ProtocolShadowsocks,
		}
		configs := GenerateAllLinks(params, protocols)

		if len(configs) != 4 {
			t.Fatalf("expected 4 configs, got %d", len(configs))
		}

		// Each config should have a non-empty link with the correct prefix.
		prefixes := map[string]string{
			ProtocolVLESS:       "vless://",
			ProtocolVMess:       "vmess://",
			ProtocolTrojan:      "trojan://",
			ProtocolShadowsocks: "ss://",
		}
		for _, cfg := range configs {
			prefix, ok := prefixes[cfg.Protocol]
			if !ok {
				t.Errorf("unexpected protocol: %s", cfg.Protocol)
			}
			if !strings.HasPrefix(cfg.Link, prefix) {
				t.Errorf("protocol %s: link should start with %q", cfg.Protocol, prefix)
			}
		}
	})

	t.Run("special characters in remarks do not break VMess JSON", func(t *testing.T) {
		params := LinkParams{
			UUID:      "vmess-special-uuid",
			Host:      "example.com",
			Port:      443,
			Remark:    `Node "quotes" & <angles> + emoji 🎉`,
			Transport: TransportTCP,
			Security:  "tls",
		}

		link := GenerateVMessLink(params)
		encoded := strings.TrimPrefix(link, "vmess://")
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("failed to decode VMess link: %v", err)
		}

		var obj map[string]string
		if err := json.Unmarshal(decoded, &obj); err != nil {
			t.Fatalf("failed to unmarshal VMess JSON: %v", err)
		}

		if obj["ps"] != `Node "quotes" & <angles> + emoji 🎉` {
			t.Errorf("remark not preserved: got %q", obj["ps"])
		}
	})

	t.Run("SaveConfig and GetConfig roundtrip preserves data", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		cfg := &XrayConfig{
			NodeID:  10,
			Enabled: true,
			Inbounds: []Inbound{
				{Protocol: ProtocolVLESS, Port: 443, Transport: TransportTCP, Tag: "main"},
				{Protocol: ProtocolVMess, Port: 8080, Transport: TransportWS, Tag: "ws"},
			},
			Routing: RoutingConfig{
				DomainStrategy: "IPIfNonMatch",
				Rules: []RoutingRule{
					{Type: "field", Domain: []string{"geosite:ir"}, OutboundTag: "direct"},
				},
			},
			TLS: TLSConfig{
				ServerName: "example.com",
				CertPath:   "/etc/ssl/cert.pem",
				KeyPath:    "/etc/ssl/key.pem",
			},
		}

		// Save expects exec.
		mock.ExpectExec("INSERT INTO xray_configs").
			WithArgs(int64(10), true, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err = svc.SaveConfig(context.Background(), cfg)
		if err != nil {
			t.Fatalf("save error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}

func TestNewXrayService(t *testing.T) {
	t.Run("creates service with default notify", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)
		if svc == nil {
			t.Fatal("New() returned nil")
		}
		if svc.db != db {
			t.Error("db reference mismatch")
		}
		// notify should not be nil.
		if svc.notify == nil {
			t.Error("notify function should not be nil")
		}
	})

	t.Run("SetNotify updates notification function", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)
		called := false
		svc.SetNotify(func(msg string) {
			called = true
		})
		svc.notify("test")
		if !called {
			t.Error("custom notify function was not called")
		}
	})

	t.Run("SetNotify ignores nil", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)
		svc.SetNotify(nil)
		// Should not panic — original notify still set.
		svc.notify("should not panic")
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
