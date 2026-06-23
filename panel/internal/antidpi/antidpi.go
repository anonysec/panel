//go:build !lite

// Package antidpi provides anti-DPI (Deep Packet Inspection) obfuscation
// configuration management for KorisPanel. It handles obfs4, QUIC tunneling,
// and WebSocket tunnel configuration per node.
package antidpi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
)

// ObfuscationMethod represents the type of traffic obfuscation applied.
type ObfuscationMethod string

const (
	ObfsNone     ObfuscationMethod = "none"
	Obfs4        ObfuscationMethod = "obfs4"
	ObfsQUIC     ObfuscationMethod = "quic"
	ObfsWSTunnel ObfuscationMethod = "ws_tunnel"
)

// AntiDPIConfig holds the anti-DPI obfuscation configuration for a single node.
type AntiDPIConfig struct {
	ID              int64             `json:"id"`
	NodeID          int64             `json:"node_id"`
	Method          ObfuscationMethod `json:"method"`
	Port            int               `json:"port"`
	BridgeAddress   string            `json:"bridge_address,omitempty"`
	CertFingerprint string            `json:"cert_fingerprint,omitempty"`
	Enabled         bool              `json:"enabled"`
	ExtraSettings   json.RawMessage   `json:"extra_settings,omitempty"`
	CreatedAt       string            `json:"created_at,omitempty"`
	UpdatedAt       string            `json:"updated_at,omitempty"`
}

// AntiDPIService manages anti-DPI configurations.
type AntiDPIService struct {
	db *sql.DB
}

// New creates a new AntiDPIService with the given database connection.
func New(db *sql.DB) *AntiDPIService {
	return &AntiDPIService{db: db}
}

// GetConfig retrieves the anti-DPI configuration for a specific node.
// Returns nil, nil if no configuration exists for the node.
func (s *AntiDPIService) GetConfig(ctx context.Context, nodeID int64) (*AntiDPIConfig, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, node_id, method, port, bridge_address, cert_fingerprint,
		       enabled, extra_settings, created_at, updated_at
		FROM anti_dpi_configs
		WHERE node_id = ?`, nodeID)

	var cfg AntiDPIConfig
	var bridgeAddr, certFP sql.NullString
	var extraSettings sql.NullString

	err := row.Scan(
		&cfg.ID, &cfg.NodeID, &cfg.Method, &cfg.Port,
		&bridgeAddr, &certFP, &cfg.Enabled, &extraSettings,
		&cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get anti-dpi config for node %d: %w", nodeID, err)
	}

	if bridgeAddr.Valid {
		cfg.BridgeAddress = bridgeAddr.String
	}
	if certFP.Valid {
		cfg.CertFingerprint = certFP.String
	}
	if extraSettings.Valid && extraSettings.String != "" {
		cfg.ExtraSettings = json.RawMessage(extraSettings.String)
	}

	return &cfg, nil
}

// SaveConfig creates or updates the anti-DPI configuration for a node.
// Uses INSERT ... ON DUPLICATE KEY UPDATE for upsert behavior.
func (s *AntiDPIService) SaveConfig(ctx context.Context, config *AntiDPIConfig) error {
	if config.NodeID == 0 {
		return fmt.Errorf("node_id is required")
	}
	if !isValidMethod(config.Method) {
		return fmt.Errorf("invalid obfuscation method: %s", config.Method)
	}

	var extraJSON *string
	if len(config.ExtraSettings) > 0 {
		s := string(config.ExtraSettings)
		extraJSON = &s
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO anti_dpi_configs (node_id, method, port, bridge_address, cert_fingerprint, enabled, extra_settings)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			method = VALUES(method),
			port = VALUES(port),
			bridge_address = VALUES(bridge_address),
			cert_fingerprint = VALUES(cert_fingerprint),
			enabled = VALUES(enabled),
			extra_settings = VALUES(extra_settings)`,
		config.NodeID, config.Method, config.Port,
		nullStr(config.BridgeAddress), nullStr(config.CertFingerprint),
		config.Enabled, extraJSON,
	)
	if err != nil {
		return fmt.Errorf("save anti-dpi config for node %d: %w", config.NodeID, err)
	}

	log.Printf("[antidpi] saved config for node=%d method=%s enabled=%v", config.NodeID, config.Method, config.Enabled)
	return nil
}

// DeleteConfig removes the anti-DPI configuration for a node.
func (s *AntiDPIService) DeleteConfig(ctx context.Context, nodeID int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM anti_dpi_configs WHERE node_id = ?`, nodeID)
	if err != nil {
		return fmt.Errorf("delete anti-dpi config for node %d: %w", nodeID, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("anti-dpi config not found for node %d", nodeID)
	}

	log.Printf("[antidpi] deleted config for node=%d", nodeID)
	return nil
}

// GenerateObfs4Bridge generates an obfs4 bridge line string for client configuration.
// Format: Bridge obfs4 <address>:<port> <cert_fingerprint> iat-mode=0
func GenerateObfs4Bridge(nodeID int64, port int, bridgeAddress, certFingerprint string) string {
	if bridgeAddress == "" || certFingerprint == "" {
		return ""
	}
	return fmt.Sprintf("Bridge obfs4 %s:%d %s iat-mode=0", bridgeAddress, port, certFingerprint)
}

// isValidMethod checks if the given method is one of the allowed obfuscation methods.
func isValidMethod(method ObfuscationMethod) bool {
	switch method {
	case ObfsNone, Obfs4, ObfsQUIC, ObfsWSTunnel:
		return true
	default:
		return false
	}
}

// nullStr returns a *string pointer — nil if the value is empty.
func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
