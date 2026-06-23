//go:build !lite

// Package xray provides the Xray/VLESS management system for KorisPanel.
// It handles Xray configuration, inbound protocol management (VLESS, VMess,
// Trojan, Shadowsocks), routing rules, TLS/Reality settings, and per-node
// config persistence.
package xray

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Protocol constants for supported Xray inbound types.
const (
	ProtocolVLESS       = "vless"
	ProtocolVMess       = "vmess"
	ProtocolTrojan      = "trojan"
	ProtocolShadowsocks = "shadowsocks"
)

// Transport constants for supported transports.
const (
	TransportTCP  = "tcp"
	TransportWS   = "ws"
	TransportGRPC = "grpc"
	TransportH2   = "h2"
)

// XrayConfig represents the full Xray configuration for a single node.
type XrayConfig struct {
	NodeID        int64          `json:"node_id"`
	Enabled       bool           `json:"enabled"`
	Inbounds      []Inbound      `json:"inbounds"`
	Routing       RoutingConfig  `json:"routing"`
	TLS           TLSConfig      `json:"tls,omitempty"`
	RealityConfig *RealityConfig `json:"reality_config,omitempty"`
	LastSyncedAt  *time.Time     `json:"last_synced_at,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// Inbound defines a single inbound protocol listener for Xray.
type Inbound struct {
	Protocol  string          `json:"protocol"` // vless, vmess, trojan, shadowsocks
	Port      int             `json:"port"`
	Transport string          `json:"transport"` // tcp, ws, grpc, h2
	Tag       string          `json:"tag,omitempty"`
	Settings  json.RawMessage `json:"settings,omitempty"`
}

// RoutingConfig defines domain-based routing rules for Xray.
// Direct domestic traffic stays local; international traffic goes through proxy.
type RoutingConfig struct {
	DomainStrategy string        `json:"domain_strategy,omitempty"` // AsIs, IPIfNonMatch, IPOnDemand
	Rules          []RoutingRule `json:"rules,omitempty"`
}

// RoutingRule represents a single routing rule entry.
type RoutingRule struct {
	Type        string   `json:"type,omitempty"`         // field
	Domain      []string `json:"domain,omitempty"`       // domain patterns
	IP          []string `json:"ip,omitempty"`           // IP/CIDR patterns
	OutboundTag string   `json:"outbound_tag,omitempty"` // direct, proxy, block
}

// TLSConfig holds TLS certificate and server name settings for Xray.
type TLSConfig struct {
	CertPath   string   `json:"cert_path,omitempty"`
	KeyPath    string   `json:"key_path,omitempty"`
	ServerName string   `json:"server_name,omitempty"`
	ALPN       []string `json:"alpn,omitempty"`
}

// RealityConfig holds Reality protocol settings for fingerprint-resistant connections.
type RealityConfig struct {
	ServerNames []string `json:"server_names,omitempty"`
	PrivateKey  string   `json:"private_key,omitempty"`
	PublicKey   string   `json:"public_key,omitempty"`
	ShortIDs    []string `json:"short_ids,omitempty"`
}

// XrayService manages Xray configuration operations backed by MariaDB.
type XrayService struct {
	db     *sql.DB
	notify func(msg string)
}

// New creates a new XrayService with the given database connection.
func New(db *sql.DB) *XrayService {
	return &XrayService{
		db:     db,
		notify: func(msg string) { log.Printf("[xray] %s", msg) },
	}
}

// SetNotify sets a custom notification function for xray events.
func (s *XrayService) SetNotify(fn func(string)) {
	if fn != nil {
		s.notify = fn
	}
}

// GetConfig retrieves the Xray configuration for a given node.
func (s *XrayService) GetConfig(ctx context.Context, nodeID int64) (*XrayConfig, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT node_id, enabled, config_json, reality_config_json,
		       last_synced_at, created_at, updated_at
		FROM xray_configs WHERE node_id = ?`, nodeID)

	var cfg XrayConfig
	var configJSON []byte
	var realityJSON sql.NullString
	var lastSynced sql.NullTime

	err := row.Scan(
		&cfg.NodeID, &cfg.Enabled, &configJSON, &realityJSON,
		&lastSynced, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("xray config not found for node %d", nodeID)
	}
	if err != nil {
		return nil, fmt.Errorf("query xray config: %w", err)
	}

	// Parse the main config JSON (inbounds, routing, TLS).
	var stored struct {
		Inbounds []Inbound     `json:"inbounds"`
		Routing  RoutingConfig `json:"routing"`
		TLS      TLSConfig     `json:"tls"`
	}
	if err := json.Unmarshal(configJSON, &stored); err != nil {
		return nil, fmt.Errorf("unmarshal config_json: %w", err)
	}
	cfg.Inbounds = stored.Inbounds
	cfg.Routing = stored.Routing
	cfg.TLS = stored.TLS

	// Parse optional Reality config.
	if realityJSON.Valid && realityJSON.String != "" {
		var rc RealityConfig
		if err := json.Unmarshal([]byte(realityJSON.String), &rc); err != nil {
			return nil, fmt.Errorf("unmarshal reality_config_json: %w", err)
		}
		cfg.RealityConfig = &rc
	}

	if lastSynced.Valid {
		cfg.LastSyncedAt = &lastSynced.Time
	}

	return &cfg, nil
}

// SaveConfig persists the Xray configuration for a node (upsert).
func (s *XrayService) SaveConfig(ctx context.Context, cfg *XrayConfig) error {
	// Marshal the main config (inbounds + routing + TLS) into JSON.
	stored := struct {
		Inbounds []Inbound     `json:"inbounds"`
		Routing  RoutingConfig `json:"routing"`
		TLS      TLSConfig     `json:"tls"`
	}{
		Inbounds: cfg.Inbounds,
		Routing:  cfg.Routing,
		TLS:      cfg.TLS,
	}
	configJSON, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("marshal config_json: %w", err)
	}

	// Marshal optional Reality config.
	var realityJSON *string
	if cfg.RealityConfig != nil {
		b, err := json.Marshal(cfg.RealityConfig)
		if err != nil {
			return fmt.Errorf("marshal reality_config_json: %w", err)
		}
		str := string(b)
		realityJSON = &str
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO xray_configs (node_id, enabled, config_json, reality_config_json)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			enabled = VALUES(enabled),
			config_json = VALUES(config_json),
			reality_config_json = VALUES(reality_config_json),
			updated_at = CURRENT_TIMESTAMP`,
		cfg.NodeID, cfg.Enabled, configJSON, realityJSON,
	)
	if err != nil {
		return fmt.Errorf("save xray config: %w", err)
	}

	s.notify(fmt.Sprintf("saved xray config for node %d", cfg.NodeID))
	return nil
}

// DeleteConfig removes the Xray configuration for a node.
func (s *XrayService) DeleteConfig(ctx context.Context, nodeID int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM xray_configs WHERE node_id = ?`, nodeID)
	if err != nil {
		return fmt.Errorf("delete xray config: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("xray config not found for node %d", nodeID)
	}

	s.notify(fmt.Sprintf("deleted xray config for node %d", nodeID))
	return nil
}
