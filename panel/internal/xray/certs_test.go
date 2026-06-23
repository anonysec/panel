//go:build !lite

package xray

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBuildXrayTLSSettings(t *testing.T) {
	t.Run("produces valid TLS settings JSON", func(t *testing.T) {
		cfg := XrayCertConfig{
			Mode:     CertModeManual,
			Domain:   "example.com",
			CertPath: "/etc/xray/cert.pem",
			KeyPath:  "/etc/xray/key.pem",
		}

		result, err := buildXrayTLSSettings(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed struct {
			ServerName   string   `json:"serverName"`
			ALPN         []string `json:"alpn"`
			Certificates []struct {
				OCSPStapling   int    `json:"ocspStapling"`
				OneTimeLoading bool   `json:"oneTimeLoading"`
				CertFile       string `json:"certificateFile"`
				KeyFile        string `json:"keyFile"`
			} `json:"certificates"`
		}
		if err := json.Unmarshal(result, &parsed); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		if parsed.ServerName != "example.com" {
			t.Errorf("serverName should be 'example.com', got %q", parsed.ServerName)
		}
		if len(parsed.ALPN) != 2 || parsed.ALPN[0] != "h2" || parsed.ALPN[1] != "http/1.1" {
			t.Errorf("ALPN should be [h2, http/1.1], got %v", parsed.ALPN)
		}
		if len(parsed.Certificates) != 1 {
			t.Fatalf("expected 1 certificate entry, got %d", len(parsed.Certificates))
		}

		cert := parsed.Certificates[0]
		if cert.OCSPStapling != 3600 {
			t.Errorf("ocspStapling should be 3600, got %d", cert.OCSPStapling)
		}
		if cert.OneTimeLoading != false {
			t.Error("oneTimeLoading should be false")
		}
		if cert.CertFile != "/etc/xray/cert.pem" {
			t.Errorf("certificateFile should be '/etc/xray/cert.pem', got %q", cert.CertFile)
		}
		if cert.KeyFile != "/etc/xray/key.pem" {
			t.Errorf("keyFile should be '/etc/xray/key.pem', got %q", cert.KeyFile)
		}
	})

	t.Run("ACME mode paths are used correctly", func(t *testing.T) {
		cfg := XrayCertConfig{
			Mode:     CertModeACME,
			Domain:   "vpn.example.org",
			CertPath: "/usr/local/etc/xray/vpn.example.org_cert.pem",
			KeyPath:  "/usr/local/etc/xray/vpn.example.org_key.pem",
		}

		result, err := buildXrayTLSSettings(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed struct {
			ServerName   string `json:"serverName"`
			Certificates []struct {
				CertFile string `json:"certificateFile"`
				KeyFile  string `json:"keyFile"`
			} `json:"certificates"`
		}
		json.Unmarshal(result, &parsed)

		if parsed.ServerName != "vpn.example.org" {
			t.Errorf("serverName should be 'vpn.example.org', got %q", parsed.ServerName)
		}
		if parsed.Certificates[0].CertFile != "/usr/local/etc/xray/vpn.example.org_cert.pem" {
			t.Errorf("certificateFile mismatch, got %q", parsed.Certificates[0].CertFile)
		}
		if parsed.Certificates[0].KeyFile != "/usr/local/etc/xray/vpn.example.org_key.pem" {
			t.Errorf("keyFile mismatch, got %q", parsed.Certificates[0].KeyFile)
		}
	})
}

func TestConfigureTLS(t *testing.T) {
	t.Run("panel mode distributes certs and saves config", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		// distributePanelCerts: insert cert.distribute task.
		mock.ExpectExec("INSERT INTO node_tasks").
			WithArgs(int64(1), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// GetConfig: returns existing config.
		now := time.Now()
		configJSON := `{"inbounds":[],"routing":{},"tls":{}}`
		mock.ExpectQuery("SELECT node_id, enabled, config_json, reality_config_json").
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{
				"node_id", "enabled", "config_json", "reality_config_json",
				"last_synced_at", "created_at", "updated_at",
			}).AddRow(1, true, configJSON, nil, nil, now, now))

		// SaveConfig: upsert.
		mock.ExpectExec("INSERT INTO xray_configs").
			WithArgs(int64(1), true, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// saveCertStatus: upsert.
		mock.ExpectExec("INSERT INTO xray_cert_status").
			WithArgs(int64(1), "panel", "example.com", true).
			WillReturnResult(sqlmock.NewResult(1, 1))

		certCfg := XrayCertConfig{
			Mode:   CertModePanel,
			Domain: "example.com",
		}

		err = svc.ConfigureTLS(context.Background(), 1, certCfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("ACME mode sets correct paths", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		// GetConfig: not found — new config.
		mock.ExpectQuery("SELECT node_id, enabled, config_json, reality_config_json").
			WithArgs(int64(2)).
			WillReturnRows(sqlmock.NewRows([]string{
				"node_id", "enabled", "config_json", "reality_config_json",
				"last_synced_at", "created_at", "updated_at",
			}))

		// SaveConfig: upsert.
		mock.ExpectExec("INSERT INTO xray_configs").
			WithArgs(int64(2), true, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// saveCertStatus: upsert.
		mock.ExpectExec("INSERT INTO xray_cert_status").
			WithArgs(int64(2), "acme", "vpn.example.org", true).
			WillReturnResult(sqlmock.NewResult(1, 1))

		certCfg := XrayCertConfig{
			Mode:      CertModeACME,
			Domain:    "vpn.example.org",
			ACMEEmail: "admin@example.org",
		}

		err = svc.ConfigureTLS(context.Background(), 2, certCfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("manual mode uses provided paths", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		// GetConfig: not found.
		mock.ExpectQuery("SELECT node_id, enabled, config_json, reality_config_json").
			WithArgs(int64(3)).
			WillReturnRows(sqlmock.NewRows([]string{
				"node_id", "enabled", "config_json", "reality_config_json",
				"last_synced_at", "created_at", "updated_at",
			}))

		// SaveConfig: upsert.
		mock.ExpectExec("INSERT INTO xray_configs").
			WithArgs(int64(3), true, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// saveCertStatus: upsert.
		mock.ExpectExec("INSERT INTO xray_cert_status").
			WithArgs(int64(3), "manual", "my.domain.com", false).
			WillReturnResult(sqlmock.NewResult(1, 1))

		certCfg := XrayCertConfig{
			Mode:     CertModeManual,
			Domain:   "my.domain.com",
			CertPath: "/custom/certs/fullchain.pem",
			KeyPath:  "/custom/certs/privkey.pem",
		}

		err = svc.ConfigureTLS(context.Background(), 3, certCfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("error when domain is empty", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		certCfg := XrayCertConfig{
			Mode:   CertModePanel,
			Domain: "",
		}

		err = svc.ConfigureTLS(context.Background(), 1, certCfg)
		if err == nil {
			t.Fatal("expected error for empty domain")
		}
	})

	t.Run("error when manual mode missing paths", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		certCfg := XrayCertConfig{
			Mode:   CertModeManual,
			Domain: "example.com",
			// Missing CertPath and KeyPath.
		}

		err = svc.ConfigureTLS(context.Background(), 1, certCfg)
		if err == nil {
			t.Fatal("expected error for missing cert/key paths in manual mode")
		}
	})

	t.Run("error for unsupported cert mode", func(t *testing.T) {
		db, _, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		certCfg := XrayCertConfig{
			Mode:   XrayCertMode("invalid"),
			Domain: "example.com",
		}

		err = svc.ConfigureTLS(context.Background(), 1, certCfg)
		if err == nil {
			t.Fatal("expected error for unsupported cert mode")
		}
	})
}

func TestGetCertStatus(t *testing.T) {
	t.Run("returns cert status from DB", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		expiry := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
		renewed := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery("SELECT cert_mode, domain, expires_at, auto_renew, last_renewed FROM xray_cert_status").
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{
				"cert_mode", "domain", "expires_at", "auto_renew", "last_renewed",
			}).AddRow("acme", "vpn.example.com", expiry, true, renewed))

		status, err := svc.GetCertStatus(context.Background(), 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if status.Mode != CertModeACME {
			t.Errorf("mode should be 'acme', got %q", status.Mode)
		}
		if status.Domain != "vpn.example.com" {
			t.Errorf("domain should be 'vpn.example.com', got %q", status.Domain)
		}
		if status.ExpiresAt == nil || !status.ExpiresAt.Equal(expiry) {
			t.Errorf("expires_at should be %v, got %v", expiry, status.ExpiresAt)
		}
		if !status.AutoRenew {
			t.Error("auto_renew should be true")
		}
		if status.LastRenewed == nil || !status.LastRenewed.Equal(renewed) {
			t.Errorf("last_renewed should be %v, got %v", renewed, status.LastRenewed)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("returns error when no cert configured", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		mock.ExpectQuery("SELECT cert_mode, domain, expires_at, auto_renew, last_renewed FROM xray_cert_status").
			WithArgs(int64(99)).
			WillReturnRows(sqlmock.NewRows([]string{
				"cert_mode", "domain", "expires_at", "auto_renew", "last_renewed",
			}))

		_, err = svc.GetCertStatus(context.Background(), 99)
		if err == nil {
			t.Fatal("expected error when no cert is configured")
		}
	})

	t.Run("handles null expires_at and last_renewed", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)

		mock.ExpectQuery("SELECT cert_mode, domain, expires_at, auto_renew, last_renewed FROM xray_cert_status").
			WithArgs(int64(5)).
			WillReturnRows(sqlmock.NewRows([]string{
				"cert_mode", "domain", "expires_at", "auto_renew", "last_renewed",
			}).AddRow("manual", "test.com", nil, false, nil))

		status, err := svc.GetCertStatus(context.Background(), 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if status.Mode != CertModeManual {
			t.Errorf("mode should be 'manual', got %q", status.Mode)
		}
		if status.ExpiresAt != nil {
			t.Errorf("expires_at should be nil, got %v", status.ExpiresAt)
		}
		if status.AutoRenew {
			t.Error("auto_renew should be false for manual mode")
		}
		if status.LastRenewed != nil {
			t.Errorf("last_renewed should be nil, got %v", status.LastRenewed)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}
