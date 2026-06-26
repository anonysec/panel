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
	rows, err := s.DB.Query(`SELECT id, technique, config_json, is_active, created_at, updated_at FROM node_antidpi WHERE node_id = $1 ORDER BY technique`, nodeID)
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
		var isActive bool
		var configStr string
		if err := rows.Scan(&c.ID, &c.Technique, &configStr, &isActive, &c.CreatedAt, &c.UpdatedAt); err != nil {
			log.Printf("[antidpi] scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		c.IsActive = isActive
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

	// Upsert: INSERT ... ON CONFLICT (node_id + technique is unique)
	_, err = s.DB.Exec(`INSERT INTO node_antidpi (node_id, technique, config_json, is_active) VALUES ($1, $2, $3, $4)
		ON CONFLICT (node_id, technique) DO UPDATE SET config_json = EXCLUDED.config_json, is_active = EXCLUDED.is_active, updated_at = CURRENT_TIMESTAMP`,
		nodeID, in.Technique, string(configBytes), isActive)
	if err != nil {
		log.Printf("[antidpi] upsert failed for node %d technique %s: %v", nodeID, in.Technique, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// NOTE: Legacy node_tasks INSERT removed. Anti-DPI apply is now dispatched via gRPC.
	log.Printf("[antidpi] antidpi_apply for node %d technique %s (dispatched via gRPC)", nodeID, in.Technique)

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

	result, err := s.DB.Exec(`DELETE FROM node_antidpi WHERE node_id = $1 AND technique = $2`, nodeID, technique)
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

	// NOTE: Legacy node_tasks INSERT removed. Anti-DPI remove is now dispatched via gRPC.
	log.Printf("[antidpi] antidpi_remove for node %d technique %s (dispatched via gRPC)", nodeID, technique)

	writeJSON(w, map[string]any{"ok": true})
}
