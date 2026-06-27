//go:build !lite

package antidpi

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetConfig(t *testing.T) {
	tests := []struct {
		name       string
		nodeID     int64
		hasRow     bool
		wantNil    bool
		wantErr    bool
		method     string
		port       int
		enabled    bool
		bridgeAddr string
		certFP     string
	}{
		{
			name:       "config exists",
			nodeID:     1,
			hasRow:     true,
			method:     "obfs4",
			port:       9443,
			enabled:    true,
			bridgeAddr: "192.168.1.100",
			certFP:     "abc123fingerprint",
		},
		{
			name:    "config not found",
			nodeID:  999,
			hasRow:  false,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()

			svc := New(db)

			if tt.hasRow {
				rows := sqlmock.NewRows([]string{
					"id", "node_id", "method", "port", "bridge_address",
					"cert_fingerprint", "enabled", "extra_settings",
					"created_at", "updated_at",
				}).AddRow(
					1, tt.nodeID, tt.method, tt.port, tt.bridgeAddr,
					tt.certFP, tt.enabled, nil,
					"2024-01-01 00:00:00", "2024-01-01 00:00:00",
				)
				mock.ExpectQuery("SELECT id, node_id, method, port, bridge_address, cert_fingerprint").
					WithArgs(tt.nodeID).
					WillReturnRows(rows)
			} else {
				mock.ExpectQuery("SELECT id, node_id, method, port, bridge_address, cert_fingerprint").
					WithArgs(tt.nodeID).
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "node_id", "method", "port", "bridge_address",
						"cert_fingerprint", "enabled", "extra_settings",
						"created_at", "updated_at",
					}))
			}

			cfg, err := svc.GetConfig(context.Background(), tt.nodeID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if cfg != nil {
					t.Errorf("expected nil config, got %+v", cfg)
				}
				return
			}

			if cfg == nil {
				t.Fatal("expected config, got nil")
			}
			if cfg.NodeID != tt.nodeID {
				t.Errorf("NodeID = %d, want %d", cfg.NodeID, tt.nodeID)
			}
			if string(cfg.Method) != tt.method {
				t.Errorf("Method = %s, want %s", cfg.Method, tt.method)
			}
			if cfg.Port != tt.port {
				t.Errorf("Port = %d, want %d", cfg.Port, tt.port)
			}
			if cfg.Enabled != tt.enabled {
				t.Errorf("Enabled = %v, want %v", cfg.Enabled, tt.enabled)
			}
			if cfg.BridgeAddress != tt.bridgeAddr {
				t.Errorf("BridgeAddress = %q, want %q", cfg.BridgeAddress, tt.bridgeAddr)
			}
			if cfg.CertFingerprint != tt.certFP {
				t.Errorf("CertFingerprint = %q, want %q", cfg.CertFingerprint, tt.certFP)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestSaveConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *AntiDPIConfig
		wantErr bool
	}{
		{
			name: "save valid obfs4 config",
			config: &AntiDPIConfig{
				NodeID:          1,
				Method:          Obfs4,
				Port:            9443,
				BridgeAddress:   "10.0.0.1",
				CertFingerprint: "fingerprint123",
				Enabled:         true,
			},
			wantErr: false,
		},
		{
			name: "save valid quic config",
			config: &AntiDPIConfig{
				NodeID:  2,
				Method:  ObfsQUIC,
				Port:    443,
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "save ws_tunnel config",
			config: &AntiDPIConfig{
				NodeID:  3,
				Method:  ObfsWSTunnel,
				Port:    8080,
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "missing node_id",
			config: &AntiDPIConfig{
				Method: Obfs4,
				Port:   443,
			},
			wantErr: true,
		},
		{
			name: "invalid method",
			config: &AntiDPIConfig{
				NodeID: 1,
				Method: "invalid_method",
				Port:   443,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()

			svc := New(db)

			if !tt.wantErr {
				mock.ExpectExec("INSERT INTO anti_dpi_configs").
					WithArgs(
						tt.config.NodeID, string(tt.config.Method), tt.config.Port,
						sqlmock.AnyArg(), sqlmock.AnyArg(),
						tt.config.Enabled, sqlmock.AnyArg(),
					).
					WillReturnResult(sqlmock.NewResult(1, 1))
			}

			err = svc.SaveConfig(context.Background(), tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestDeleteConfig(t *testing.T) {
	tests := []struct {
		name    string
		nodeID  int64
		rowsAff int64
		wantErr bool
	}{
		{
			name:    "successful deletion",
			nodeID:  1,
			rowsAff: 1,
			wantErr: false,
		},
		{
			name:    "config not found",
			nodeID:  999,
			rowsAff: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()

			svc := New(db)

			mock.ExpectExec("DELETE FROM anti_dpi_configs WHERE node_id = \\$1").
				WithArgs(tt.nodeID).
				WillReturnResult(sqlmock.NewResult(0, tt.rowsAff))

			err = svc.DeleteConfig(context.Background(), tt.nodeID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestGenerateObfs4Bridge(t *testing.T) {
	tests := []struct {
		name       string
		nodeID     int64
		port       int
		bridgeAddr string
		certFP     string
		want       string
	}{
		{
			name:       "valid bridge line",
			nodeID:     1,
			port:       9443,
			bridgeAddr: "192.168.1.100",
			certFP:     "ABCDEF1234567890ABCDEF1234567890ABCDEF12",
			want:       "Bridge obfs4 192.168.1.100:9443 ABCDEF1234567890ABCDEF1234567890ABCDEF12 iat-mode=0",
		},
		{
			name:       "different port and address",
			nodeID:     2,
			port:       443,
			bridgeAddr: "10.0.0.1",
			certFP:     "cert_fp_here_123456",
			want:       "Bridge obfs4 10.0.0.1:443 cert_fp_here_123456 iat-mode=0",
		},
		{
			name:       "domain as bridge address",
			nodeID:     3,
			port:       8443,
			bridgeAddr: "bridge.example.com",
			certFP:     "FP9999",
			want:       "Bridge obfs4 bridge.example.com:8443 FP9999 iat-mode=0",
		},
		{
			name:       "empty bridge address returns empty",
			nodeID:     4,
			port:       443,
			bridgeAddr: "",
			certFP:     "something",
			want:       "",
		},
		{
			name:       "empty cert fingerprint returns empty",
			nodeID:     5,
			port:       443,
			bridgeAddr: "1.2.3.4",
			certFP:     "",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateObfs4Bridge(tt.nodeID, tt.port, tt.bridgeAddr, tt.certFP)
			if got != tt.want {
				t.Errorf("GenerateObfs4Bridge() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsValidMethod(t *testing.T) {
	tests := []struct {
		method ObfuscationMethod
		want   bool
	}{
		{ObfsNone, true},
		{Obfs4, true},
		{ObfsQUIC, true},
		{ObfsWSTunnel, true},
		{"invalid", false},
		{"", false},
		{"OBFS4", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.method), func(t *testing.T) {
			if got := isValidMethod(tt.method); got != tt.want {
				t.Errorf("isValidMethod(%q) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}
