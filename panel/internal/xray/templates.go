//go:build !lite

package xray

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// XrayTemplate represents an admin-editable config template for Xray settings.
type XrayTemplate struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	ConfigJSON  json.RawMessage `json:"config_json"`
	Category    string          `json:"category"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ListTemplates returns all xray config templates.
func (s *XrayService) ListTemplates(ctx context.Context) ([]XrayTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(description,''), config_json, COALESCE(category,'general'),
		       created_at, updated_at
		FROM xray_templates
		ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list xray templates: %w", err)
	}
	defer rows.Close()

	var templates []XrayTemplate
	for rows.Next() {
		var t XrayTemplate
		var configRaw []byte
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &configRaw, &t.Category, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan xray template: %w", err)
		}
		t.ConfigJSON = json.RawMessage(configRaw)
		templates = append(templates, t)
	}
	if templates == nil {
		templates = []XrayTemplate{}
	}
	return templates, nil
}

// GetTemplate retrieves a single template by ID.
func (s *XrayService) GetTemplate(ctx context.Context, id int64) (*XrayTemplate, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, COALESCE(description,''), config_json, COALESCE(category,'general'),
		       created_at, updated_at
		FROM xray_templates WHERE id = ?`, id)

	var t XrayTemplate
	var configRaw []byte
	err := row.Scan(&t.ID, &t.Name, &t.Description, &configRaw, &t.Category, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("template not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get xray template: %w", err)
	}
	t.ConfigJSON = json.RawMessage(configRaw)
	return &t, nil
}

// CreateTemplate inserts a new template and returns its ID.
func (s *XrayService) CreateTemplate(ctx context.Context, tmpl *XrayTemplate) (int64, error) {
	if tmpl.Name == "" {
		return 0, fmt.Errorf("template name is required")
	}
	if len(tmpl.ConfigJSON) == 0 {
		return 0, fmt.Errorf("config_json is required")
	}
	// Validate config_json is valid JSON.
	if !json.Valid(tmpl.ConfigJSON) {
		return 0, fmt.Errorf("config_json is not valid JSON")
	}
	if tmpl.Category == "" {
		tmpl.Category = "general"
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO xray_templates (name, description, config_json, category)
		VALUES (?, ?, ?, ?)`,
		tmpl.Name, tmpl.Description, string(tmpl.ConfigJSON), tmpl.Category,
	)
	if err != nil {
		return 0, fmt.Errorf("create xray template: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}

	s.notify(fmt.Sprintf("created xray template %q (id=%d)", tmpl.Name, id))
	return id, nil
}

// UpdateTemplate updates an existing template by ID.
func (s *XrayService) UpdateTemplate(ctx context.Context, id int64, tmpl *XrayTemplate) error {
	if tmpl.Name == "" {
		return fmt.Errorf("template name is required")
	}
	if len(tmpl.ConfigJSON) == 0 {
		return fmt.Errorf("config_json is required")
	}
	if !json.Valid(tmpl.ConfigJSON) {
		return fmt.Errorf("config_json is not valid JSON")
	}
	if tmpl.Category == "" {
		tmpl.Category = "general"
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE xray_templates
		SET name = ?, description = ?, config_json = ?, category = ?
		WHERE id = ?`,
		tmpl.Name, tmpl.Description, string(tmpl.ConfigJSON), tmpl.Category, id,
	)
	if err != nil {
		return fmt.Errorf("update xray template: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("template not found")
	}

	s.notify(fmt.Sprintf("updated xray template %q (id=%d)", tmpl.Name, id))
	return nil
}

// DeleteTemplate removes a template by ID.
func (s *XrayService) DeleteTemplate(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM xray_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete xray template: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("template not found")
	}

	s.notify(fmt.Sprintf("deleted xray template id=%d", id))
	return nil
}

// ApplyTemplate applies a template's config to a node's xray configuration.
// It loads the template's config_json and merges it into the node's xray_configs entry.
func (s *XrayService) ApplyTemplate(ctx context.Context, nodeID int64, templateID int64) error {
	// Load template.
	tmpl, err := s.GetTemplate(ctx, templateID)
	if err != nil {
		return fmt.Errorf("load template: %w", err)
	}

	// Parse template config.
	var templateConfig struct {
		Inbounds []Inbound     `json:"inbounds"`
		Routing  RoutingConfig `json:"routing"`
		TLS      TLSConfig     `json:"tls"`
	}
	if err := json.Unmarshal(tmpl.ConfigJSON, &templateConfig); err != nil {
		return fmt.Errorf("parse template config: %w", err)
	}

	// Build the xray config for the node.
	cfg := &XrayConfig{
		NodeID:   nodeID,
		Enabled:  true,
		Inbounds: templateConfig.Inbounds,
		Routing:  templateConfig.Routing,
		TLS:      templateConfig.TLS,
	}

	// Check if the template includes a reality block.
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(tmpl.ConfigJSON, &rawMap); err == nil {
		if realityRaw, ok := rawMap["reality"]; ok {
			var rc RealityConfig
			if err := json.Unmarshal(realityRaw, &rc); err == nil {
				cfg.RealityConfig = &rc
			}
		}
	}

	// Save the config (upsert).
	if err := s.SaveConfig(ctx, cfg); err != nil {
		return fmt.Errorf("apply template to node %d: %w", nodeID, err)
	}

	s.notify(fmt.Sprintf("applied template %q to node %d", tmpl.Name, nodeID))
	return nil
}
