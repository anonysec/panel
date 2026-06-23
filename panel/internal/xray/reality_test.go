//go:build !lite

package xray

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"golang.org/x/crypto/curve25519"
)

func TestGenerateRealityKeyPair(t *testing.T) {
	t.Run("produces valid key pair", func(t *testing.T) {
		privKey, pubKey, err := GenerateRealityKeyPair()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if privKey == "" {
			t.Fatal("private key should not be empty")
		}
		if pubKey == "" {
			t.Fatal("public key should not be empty")
		}
		// Base64url-no-padding encoded 32 bytes = 43 characters.
		if len(privKey) != 43 {
			t.Errorf("private key length should be 43, got %d", len(privKey))
		}
		if len(pubKey) != 43 {
			t.Errorf("public key length should be 43, got %d", len(pubKey))
		}
	})

	t.Run("produces unique keys each time", func(t *testing.T) {
		priv1, pub1, _ := GenerateRealityKeyPair()
		priv2, pub2, _ := GenerateRealityKeyPair()

		if priv1 == priv2 {
			t.Error("two generated private keys should not be identical")
		}
		if pub1 == pub2 {
			t.Error("two generated public keys should not be identical")
		}
	})

	t.Run("public key derives from private key correctly", func(t *testing.T) {
		privKey, pubKey, err := GenerateRealityKeyPair()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Decode private key and verify public key derivation.
		privBytes := decodeBase64URL(privKey)
		pubBytes := decodeBase64URL(pubKey)

		if len(privBytes) != 32 {
			t.Fatalf("private key should decode to 32 bytes, got %d", len(privBytes))
		}
		if len(pubBytes) != 32 {
			t.Fatalf("public key should decode to 32 bytes, got %d", len(pubBytes))
		}

		// Re-derive public key from private key.
		derivedPub, err := curve25519.X25519(privBytes, curve25519.Basepoint)
		if err != nil {
			t.Fatalf("failed to derive public key: %v", err)
		}

		for i := range pubBytes {
			if pubBytes[i] != derivedPub[i] {
				t.Fatal("public key does not match derived key from private key")
			}
		}
	})
}

// decodeBase64URL decodes base64url without padding (test helper).
func decodeBase64URL(s string) []byte {
	const base64URLChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	lookup := make(map[byte]byte)
	for i, c := range []byte(base64URLChars) {
		lookup[c] = byte(i)
	}

	result := make([]byte, 0, len(s)*3/4)
	buf := make([]byte, 0, 4)

	for i := 0; i < len(s); i++ {
		buf = append(buf, lookup[s[i]])
		if len(buf) == 4 {
			result = append(result,
				buf[0]<<2|buf[1]>>4,
				buf[1]<<4|buf[2]>>2,
				buf[2]<<6|buf[3],
			)
			buf = buf[:0]
		}
	}

	switch len(buf) {
	case 3:
		result = append(result,
			buf[0]<<2|buf[1]>>4,
			buf[1]<<4|buf[2]>>2,
		)
	case 2:
		result = append(result,
			buf[0]<<2|buf[1]>>4,
		)
	}

	return result
}

func TestGenerateShortID(t *testing.T) {
	t.Run("produces 8-character hex string", func(t *testing.T) {
		id := GenerateShortID()
		if len(id) != 8 {
			t.Errorf("short ID should be 8 characters, got %d: %q", len(id), id)
		}
		// Verify it's valid hex.
		_, err := hex.DecodeString(id)
		if err != nil {
			t.Errorf("short ID should be valid hex: %v", err)
		}
	})

	t.Run("produces unique IDs", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := GenerateShortID()
			if seen[id] {
				t.Errorf("duplicate short ID generated: %s", id)
			}
			seen[id] = true
		}
	})
}

func TestBuildRealityInbound(t *testing.T) {
	t.Run("valid params produce correct inbound", func(t *testing.T) {
		params := RealityInboundParams{
			Port:        443,
			ServerNames: []string{"www.google.com"},
			PrivateKey:  "test-private-key-base64url-encoded-value",
			ShortIDs:    []string{"abcdef12"},
			Flow:        "xtls-rprx-vision",
			Dest:        "www.google.com:443",
		}

		inbound, err := BuildRealityInbound(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if inbound.Protocol != ProtocolVLESS {
			t.Errorf("protocol should be %s, got %s", ProtocolVLESS, inbound.Protocol)
		}
		if inbound.Port != 443 {
			t.Errorf("port should be 443, got %d", inbound.Port)
		}
		if inbound.Transport != TransportTCP {
			t.Errorf("transport should be %s, got %s", TransportTCP, inbound.Transport)
		}
		if inbound.Tag != "vless-reality" {
			t.Errorf("tag should be 'vless-reality', got %q", inbound.Tag)
		}

		// Verify the JSON structure.
		var fullConfig realityInboundJSON
		if err := json.Unmarshal(inbound.Settings, &fullConfig); err != nil {
			t.Fatalf("failed to unmarshal settings: %v", err)
		}

		if fullConfig.Listen != "0.0.0.0" {
			t.Errorf("listen should be '0.0.0.0', got %q", fullConfig.Listen)
		}
		if fullConfig.Port != 443 {
			t.Errorf("port should be 443, got %d", fullConfig.Port)
		}
		if fullConfig.Protocol != "vless" {
			t.Errorf("protocol should be 'vless', got %q", fullConfig.Protocol)
		}
		if fullConfig.Settings.Decryption != "none" {
			t.Errorf("decryption should be 'none', got %q", fullConfig.Settings.Decryption)
		}
		if fullConfig.StreamSettings.Network != "tcp" {
			t.Errorf("network should be 'tcp', got %q", fullConfig.StreamSettings.Network)
		}
		if fullConfig.StreamSettings.Security != "reality" {
			t.Errorf("security should be 'reality', got %q", fullConfig.StreamSettings.Security)
		}

		rs := fullConfig.StreamSettings.RealitySettings
		if rs.Dest != "www.google.com:443" {
			t.Errorf("dest should be 'www.google.com:443', got %q", rs.Dest)
		}
		if len(rs.ServerNames) != 1 || rs.ServerNames[0] != "www.google.com" {
			t.Errorf("serverNames should be [www.google.com], got %v", rs.ServerNames)
		}
		if rs.PrivateKey != "test-private-key-base64url-encoded-value" {
			t.Errorf("privateKey mismatch")
		}
		if len(rs.ShortIDs) != 1 || rs.ShortIDs[0] != "abcdef12" {
			t.Errorf("shortIds should be [abcdef12], got %v", rs.ShortIDs)
		}
		if fullConfig.Tag != "vless-reality" {
			t.Errorf("tag should be 'vless-reality', got %q", fullConfig.Tag)
		}
	})

	t.Run("default flow is xtls-rprx-vision", func(t *testing.T) {
		params := RealityInboundParams{
			Port:        443,
			ServerNames: []string{"www.google.com"},
			PrivateKey:  "some-key",
			ShortIDs:    []string{"11223344"},
			Flow:        "", // empty — should default
			Dest:        "www.google.com:443",
		}

		_, err := BuildRealityInbound(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("default dest uses first server name", func(t *testing.T) {
		params := RealityInboundParams{
			Port:        443,
			ServerNames: []string{"example.com", "another.com"},
			PrivateKey:  "some-key",
			ShortIDs:    []string{"aabbccdd"},
			Dest:        "", // empty — should default to example.com:443
		}

		inbound, err := BuildRealityInbound(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var fullConfig realityInboundJSON
		json.Unmarshal(inbound.Settings, &fullConfig)

		if fullConfig.StreamSettings.RealitySettings.Dest != "example.com:443" {
			t.Errorf("dest should default to 'example.com:443', got %q",
				fullConfig.StreamSettings.RealitySettings.Dest)
		}
	})

	t.Run("error on zero port", func(t *testing.T) {
		params := RealityInboundParams{
			Port:        0,
			ServerNames: []string{"www.google.com"},
			PrivateKey:  "key",
			ShortIDs:    []string{"abcdef12"},
		}
		_, err := BuildRealityInbound(params)
		if err == nil {
			t.Fatal("expected error for zero port")
		}
	})

	t.Run("error on empty server names", func(t *testing.T) {
		params := RealityInboundParams{
			Port:        443,
			ServerNames: []string{},
			PrivateKey:  "key",
			ShortIDs:    []string{"abcdef12"},
		}
		_, err := BuildRealityInbound(params)
		if err == nil {
			t.Fatal("expected error for empty server names")
		}
	})

	t.Run("error on empty private key", func(t *testing.T) {
		params := RealityInboundParams{
			Port:        443,
			ServerNames: []string{"www.google.com"},
			PrivateKey:  "",
			ShortIDs:    []string{"abcdef12"},
		}
		_, err := BuildRealityInbound(params)
		if err == nil {
			t.Fatal("expected error for empty private key")
		}
	})

	t.Run("error on empty short IDs", func(t *testing.T) {
		params := RealityInboundParams{
			Port:        443,
			ServerNames: []string{"www.google.com"},
			PrivateKey:  "key",
			ShortIDs:    []string{},
		}
		_, err := BuildRealityInbound(params)
		if err == nil {
			t.Fatal("expected error for empty short IDs")
		}
	})

	t.Run("multiple server names and short IDs", func(t *testing.T) {
		params := RealityInboundParams{
			Port:        443,
			ServerNames: []string{"www.google.com", "www.microsoft.com", "www.apple.com"},
			PrivateKey:  "multi-key",
			ShortIDs:    []string{"11111111", "22222222", "33333333"},
			Dest:        "www.google.com:443",
		}

		inbound, err := BuildRealityInbound(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var fullConfig realityInboundJSON
		json.Unmarshal(inbound.Settings, &fullConfig)

		if len(fullConfig.StreamSettings.RealitySettings.ServerNames) != 3 {
			t.Errorf("expected 3 server names, got %d",
				len(fullConfig.StreamSettings.RealitySettings.ServerNames))
		}
		if len(fullConfig.StreamSettings.RealitySettings.ShortIDs) != 3 {
			t.Errorf("expected 3 short IDs, got %d",
				len(fullConfig.StreamSettings.RealitySettings.ShortIDs))
		}
	})
}

func TestGetRealityPublicKey(t *testing.T) {
	t.Run("returns public key from DB", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		realityJSON := `{"server_names":["www.google.com"],"private_key":"priv","public_key":"test-pub-key-123","short_ids":["abcdef12"]}`
		mock.ExpectQuery("SELECT reality_config_json FROM xray_configs WHERE node_id").
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"reality_config_json"}).AddRow(realityJSON))

		pubKey, err := svc.GetRealityPublicKey(context.Background(), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pubKey != "test-pub-key-123" {
			t.Errorf("expected 'test-pub-key-123', got %q", pubKey)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("error when reality not configured", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		mock.ExpectQuery("SELECT reality_config_json FROM xray_configs WHERE node_id").
			WithArgs(int64(2)).
			WillReturnRows(sqlmock.NewRows([]string{"reality_config_json"}).AddRow(nil))

		_, err = svc.GetRealityPublicKey(context.Background(), 2)
		if err == nil {
			t.Fatal("expected error when reality not configured")
		}
	})

	t.Run("error when public key is empty", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		realityJSON := `{"server_names":["www.google.com"],"private_key":"priv","public_key":"","short_ids":["abcdef12"]}`
		mock.ExpectQuery("SELECT reality_config_json FROM xray_configs WHERE node_id").
			WithArgs(int64(3)).
			WillReturnRows(sqlmock.NewRows([]string{"reality_config_json"}).AddRow(realityJSON))

		_, err = svc.GetRealityPublicKey(context.Background(), 3)
		if err == nil {
			t.Fatal("expected error when public key is empty")
		}
	})
}

func TestSetupReality(t *testing.T) {
	t.Run("generates keys and short IDs when not provided", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		// GetConfig will return not found — new config.
		mock.ExpectQuery("SELECT node_id, enabled, config_json, reality_config_json").
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{
				"node_id", "enabled", "config_json", "reality_config_json",
				"last_synced_at", "created_at", "updated_at",
			}))

		// SaveConfig upsert.
		mock.ExpectExec("INSERT INTO xray_configs").
			WithArgs(
				int64(1),         // node_id
				true,             // enabled
				sqlmock.AnyArg(), // config_json
				sqlmock.AnyArg(), // reality_config_json
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		params := RealityInboundParams{
			Port:        443,
			ServerNames: []string{"www.google.com"},
			// No keys or short IDs — should be generated.
		}

		err = svc.SetupReality(context.Background(), 1, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("uses provided keys and short IDs", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		// GetConfig will return not found.
		mock.ExpectQuery("SELECT node_id, enabled, config_json, reality_config_json").
			WithArgs(int64(5)).
			WillReturnRows(sqlmock.NewRows([]string{
				"node_id", "enabled", "config_json", "reality_config_json",
				"last_synced_at", "created_at", "updated_at",
			}))

		// SaveConfig upsert.
		mock.ExpectExec("INSERT INTO xray_configs").
			WithArgs(
				int64(5),
				true,
				sqlmock.AnyArg(),
				sqlmock.AnyArg(),
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		params := RealityInboundParams{
			Port:        8443,
			ServerNames: []string{"www.microsoft.com"},
			PrivateKey:  "my-private-key",
			PublicKey:   "my-public-key",
			ShortIDs:    []string{"aabb1122", "ccdd3344"},
			Flow:        "xtls-rprx-vision",
			Dest:        "www.microsoft.com:443",
		}

		err = svc.SetupReality(context.Background(), 5, params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}
