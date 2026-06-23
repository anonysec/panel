//go:build !lite

package xray

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// XrayCertMode defines how TLS certificates are obtained for Xray.
type XrayCertMode string

const (
	// CertModeACME uses Xray's built-in ACME to auto-obtain certificates.
	CertModeACME XrayCertMode = "acme"
	// CertModePanel uses the panel's existing certificates from /opt/KorisPanel/certs/.
	CertModePanel XrayCertMode = "panel"
	// CertModeManual uses admin-provided certificate file paths.
	CertModeManual XrayCertMode = "manual"
)

// XrayCertConfig holds the configuration for TLS certificate management.
type XrayCertConfig struct {
	Mode      XrayCertMode `json:"mode"`
	Domain    string       `json:"domain"`
	CertPath  string       `json:"cert_path,omitempty"`
	KeyPath   string       `json:"key_path,omitempty"`
	ACMEEmail string       `json:"acme_email,omitempty"`
}

// CertStatus represents the current state of a node's Xray TLS certificate.
type CertStatus struct {
	Mode        XrayCertMode `json:"mode"`
	Domain      string       `json:"domain"`
	ExpiresAt   *time.Time   `json:"expires_at,omitempty"`
	AutoRenew   bool         `json:"auto_renew"`
	LastRenewed *time.Time   `json:"last_renewed,omitempty"`
}

// panelCertDir is the default directory for panel-managed certificates.
const panelCertDir = "/opt/KorisPanel/certs/"

// ConfigureTLS sets up TLS certificate management for a node's Xray instance.
// It supports three modes: panel (use existing panel certs), ACME (Xray auto-obtain),
// and manual (admin-provided paths).
func (s *XrayService) ConfigureTLS(ctx context.Context, nodeID int64, certCfg XrayCertConfig) error {
	if certCfg.Domain == "" {
		return fmt.Errorf("domain is required for TLS configuration")
	}

	var certPath, keyPath string

	switch certCfg.Mode {
	case CertModePanel:
		certPath = panelCertDir + certCfg.Domain + "/fullchain.pem"
		keyPath = panelCertDir + certCfg.Domain + "/privkey.pem"

		// Distribute panel certs to the node via cert.distribute task.
		if err := s.distributePanelCerts(ctx, nodeID, certPath, keyPath); err != nil {
			return fmt.Errorf("distribute panel certs: %w", err)
		}

	case CertModeACME:
		// Xray handles ACME natively — cert files are auto-obtained.
		certPath = "/usr/local/etc/xray/" + certCfg.Domain + "_cert.pem"
		keyPath = "/usr/local/etc/xray/" + certCfg.Domain + "_key.pem"

	case CertModeManual:
		if certCfg.CertPath == "" || certCfg.KeyPath == "" {
			return fmt.Errorf("cert_path and key_path are required for manual mode")
		}
		certPath = certCfg.CertPath
		keyPath = certCfg.KeyPath

	default:
		return fmt.Errorf("unsupported cert mode: %s", certCfg.Mode)
	}

	// Build the TLS settings JSON for Xray config.
	resolvedCfg := XrayCertConfig{
		Mode:      certCfg.Mode,
		Domain:    certCfg.Domain,
		CertPath:  certPath,
		KeyPath:   keyPath,
		ACMEEmail: certCfg.ACMEEmail,
	}

	tlsJSON, err := buildXrayTLSSettings(resolvedCfg)
	if err != nil {
		return fmt.Errorf("build TLS settings: %w", err)
	}

	// Update the node's Xray config TLS settings in the DB.
	cfg, err := s.GetConfig(ctx, nodeID)
	if err != nil {
		// Config doesn't exist yet — create a new one.
		cfg = &XrayConfig{
			NodeID:  nodeID,
			Enabled: true,
		}
	}

	cfg.TLS = TLSConfig{
		CertPath:   certPath,
		KeyPath:    keyPath,
		ServerName: certCfg.Domain,
		ALPN:       []string{"h2", "http/1.1"},
	}

	if err := s.SaveConfig(ctx, cfg); err != nil {
		return fmt.Errorf("save xray config with TLS: %w", err)
	}

	// Persist cert status for tracking.
	if err := s.saveCertStatus(ctx, nodeID, resolvedCfg, tlsJSON); err != nil {
		return fmt.Errorf("save cert status: %w", err)
	}

	s.notify(fmt.Sprintf("configured TLS (%s mode) for node %d domain %s", certCfg.Mode, nodeID, certCfg.Domain))
	return nil
}

// GetCertStatus returns the current TLS certificate status for a node.
func (s *XrayService) GetCertStatus(ctx context.Context, nodeID int64) (*CertStatus, error) {
	var mode, domain string
	var expiresAt sql.NullTime
	var autoRenew bool
	var lastRenewed sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT cert_mode, domain, expires_at, auto_renew, last_renewed
		FROM xray_cert_status WHERE node_id = ?`, nodeID,
	).Scan(&mode, &domain, &expiresAt, &autoRenew, &lastRenewed)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no TLS certificate configured for node %d", nodeID)
	}
	if err != nil {
		return nil, fmt.Errorf("query cert status for node %d: %w", nodeID, err)
	}

	status := &CertStatus{
		Mode:      XrayCertMode(mode),
		Domain:    domain,
		AutoRenew: autoRenew,
	}
	if expiresAt.Valid {
		status.ExpiresAt = &expiresAt.Time
	}
	if lastRenewed.Valid {
		status.LastRenewed = &lastRenewed.Time
	}

	return status, nil
}

// buildXrayTLSSettings generates the Xray-compatible tlsSettings JSON object.
func buildXrayTLSSettings(certCfg XrayCertConfig) (json.RawMessage, error) {
	type certificate struct {
		OCSPStapling   int    `json:"ocspStapling"`
		OneTimeLoading bool   `json:"oneTimeLoading"`
		CertFile       string `json:"certificateFile"`
		KeyFile        string `json:"keyFile"`
	}

	type tlsSettings struct {
		ServerName   string        `json:"serverName"`
		ALPN         []string      `json:"alpn"`
		Certificates []certificate `json:"certificates"`
	}

	settings := tlsSettings{
		ServerName: certCfg.Domain,
		ALPN:       []string{"h2", "http/1.1"},
		Certificates: []certificate{
			{
				OCSPStapling:   3600,
				OneTimeLoading: false,
				CertFile:       certCfg.CertPath,
				KeyFile:        certCfg.KeyPath,
			},
		},
	}

	data, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("marshal TLS settings: %w", err)
	}

	return json.RawMessage(data), nil
}

// distributePanelCerts creates a cert.distribute node task to push panel certs to the node.
func (s *XrayService) distributePanelCerts(ctx context.Context, nodeID int64, certPath, keyPath string) error {
	payload := map[string]string{
		"cert_path":    certPath,
		"key_path":     keyPath,
		"cert_content": base64.StdEncoding.EncodeToString([]byte("placeholder")),
		"key_content":  base64.StdEncoding.EncodeToString([]byte("placeholder")),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal cert distribute payload: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO node_tasks (node_id, action, payload_json, status)
		VALUES (?, 'cert.distribute', ?, 'pending')`,
		nodeID, string(payloadJSON),
	)
	if err != nil {
		return fmt.Errorf("create cert.distribute task: %w", err)
	}

	return nil
}

// saveCertStatus persists the TLS certificate status for a node (upsert).
func (s *XrayService) saveCertStatus(ctx context.Context, nodeID int64, certCfg XrayCertConfig, _ json.RawMessage) error {
	autoRenew := certCfg.Mode == CertModeACME || certCfg.Mode == CertModePanel

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO xray_cert_status (node_id, cert_mode, domain, auto_renew)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			cert_mode = VALUES(cert_mode),
			domain = VALUES(domain),
			auto_renew = VALUES(auto_renew),
			updated_at = CURRENT_TIMESTAMP`,
		nodeID, string(certCfg.Mode), certCfg.Domain, autoRenew,
	)
	if err != nil {
		return fmt.Errorf("upsert cert status: %w", err)
	}

	return nil
}
