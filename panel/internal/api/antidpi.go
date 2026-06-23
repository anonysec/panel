//go:build !lite

package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// handleNodeAntiDPI handles /api/nodes/{id}/antidpi and /api/nodes/{id}/antidpi/{technique}.
// It dispatches based on method: GET lists configs, POST adds/updates, DELETE removes.
func (s *Server) handleNodeAntiDPI(w http.ResponseWriter, r *http.Request, nodeID int64, technique string) {
	switch r.Method {
	case http.MethodGet:
		s.listAntiDPIConfigs(w, nodeID)
	case http.MethodPost:
		s.upsertAntiDPIConfig(w, r, nodeID)
	case http.MethodDelete:
		if technique == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "technique_required"})
			return
		}
		s.deleteAntiDPIConfig(w, nodeID, technique)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listAntiDPIConfigs returns all anti-DPI configurations for a node.
func (s *Server) listAntiDPIConfigs(w http.ResponseWriter, nodeID int64) {
	rows, err := s.DB.Query(`SELECT id, technique, config_json, is_active, created_at, updated_at FROM node_antidpi WHERE node_id = ? ORDER BY technique`, nodeID)
	if err != nil {
		log.Printf("[antidpi] list query failed for node %d: %v", nodeID, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type antiDPIConfig struct {
		ID         int64           `json:"id"`
		Technique  string          `json:"technique"`
		ConfigJSON json.RawMessage `json:"config_json"`
		IsActive   bool            `json:"is_active"`
		CreatedAt  string          `json:"created_at"`
		UpdatedAt  string          `json:"updated_at"`
	}

	var configs []antiDPIConfig
	for rows.Next() {
		var c antiDPIConfig
		var isActive int
		var configStr string
		if err := rows.Scan(&c.ID, &c.Technique, &configStr, &isActive, &c.CreatedAt, &c.UpdatedAt); err != nil {
			log.Printf("[antidpi] scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		c.IsActive = isActive == 1
		c.ConfigJSON = json.RawMessage(configStr)
		configs = append(configs, c)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[antidpi] rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if configs == nil {
		configs = []antiDPIConfig{}
	}
	writeJSON(w, map[string]any{"ok": true, "configs": configs})
}

// upsertAntiDPIConfig adds or updates an anti-DPI technique for a node.
func (s *Server) upsertAntiDPIConfig(w http.ResponseWriter, r *http.Request, nodeID int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Technique  string         `json:"technique"`
		ConfigJSON map[string]any `json:"config_json"`
		IsActive   *bool          `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.Technique == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "technique_required"})
		return
	}

	// Validate the config
	if err := validateAntiDPIConfig(in.Technique, in.ConfigJSON); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	isActive := 1
	if in.IsActive != nil && !*in.IsActive {
		isActive = 0
	}

	configBytes, err := json.Marshal(in.ConfigJSON)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_config_json"})
		return
	}

	// Upsert: INSERT ... ON DUPLICATE KEY UPDATE (node_id + technique is unique)
	_, err = s.DB.Exec(`INSERT INTO node_antidpi (node_id, technique, config_json, is_active) VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE config_json = VALUES(config_json), is_active = VALUES(is_active), updated_at = CURRENT_TIMESTAMP`,
		nodeID, in.Technique, string(configBytes), isActive)
	if err != nil {
		log.Printf("[antidpi] upsert failed for node %d technique %s: %v", nodeID, in.Technique, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// Push antidpi_apply task to the node
	payload, _ := json.Marshal(map[string]any{
		"technique":   in.Technique,
		"config_json": in.ConfigJSON,
	})
	_, err = s.DB.Exec(`INSERT INTO node_tasks (node_id, action, payload_json, status, created_by) VALUES (?, 'antidpi_apply', ?, 'pending', 'system')`,
		nodeID, string(payload))
	if err != nil {
		log.Printf("[antidpi] failed to create antidpi_apply task for node %d: %v", nodeID, err)
	}

	writeJSON(w, map[string]any{"ok": true})
}

// deleteAntiDPIConfig removes an anti-DPI technique from a node.
func (s *Server) deleteAntiDPIConfig(w http.ResponseWriter, nodeID int64, technique string) {
	// Normalize technique name
	technique = strings.TrimSpace(technique)
	if technique == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "technique_required"})
		return
	}

	result, err := s.DB.Exec(`DELETE FROM node_antidpi WHERE node_id = ? AND technique = ?`, nodeID, technique)
	if err != nil {
		log.Printf("[antidpi] delete failed for node %d technique %s: %v", nodeID, technique, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// Push antidpi_remove task to the node
	payload, _ := json.Marshal(map[string]any{
		"technique": technique,
	})
	_, err = s.DB.Exec(`INSERT INTO node_tasks (node_id, action, payload_json, status, created_by) VALUES (?, 'antidpi_remove', ?, 'pending', 'system')`,
		nodeID, string(payload))
	if err != nil {
		log.Printf("[antidpi] failed to create antidpi_remove task for node %d: %v", nodeID, err)
	}

	writeJSON(w, map[string]any{"ok": true})
}
