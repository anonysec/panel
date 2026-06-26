package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// handleNodeUpdate handles GET /api/node/update.
// Authenticated by node bearer token (X-Node-Token header).
// Returns the latest node binary version, download URL, and checksum
// from the panel_settings table.
func (s *Server) handleNodeUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate node via X-Node-Token header
	_, ok := s.authNode(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "bad_token"})
		return
	}

	// Read node update info from panel_settings
	keys := []string{"node_version", "node_binary_url", "node_binary_checksum"}
	settings := map[string]string{}
	for _, k := range keys {
		var v string
		if err := s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = $1`, k).Scan(&v); err == nil {
			settings[k] = v
		}
	}

	writeJSON(w, map[string]any{
		"ok":           true,
		"version":      settings["node_version"],
		"download_url": settings["node_binary_url"],
		"checksum":     settings["node_binary_checksum"],
	})
}

// handleAdminNodeUpdate handles POST /api/admin/node-update.
// Allows the admin to configure the node update info (version, download URL, checksum).
func (s *Server) handleAdminNodeUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var in struct {
		Version     string `json:"version"`
		DownloadURL string `json:"download_url"`
		Checksum    string `json:"checksum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Upsert each setting
	pairs := map[string]string{
		"node_version":         in.Version,
		"node_binary_url":      in.DownloadURL,
		"node_binary_checksum": in.Checksum,
	}
	for key, val := range pairs {
		_, err := s.DB.Exec(
			`INSERT INTO panel_settings(setting_key, setting_value) VALUES($1, $2) ON CONFLICT (setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value`,
			key, val,
		)
		if err != nil {
			log.Printf("[node-update] failed to save %s: %v", key, err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "settings.node_update", "panel_settings", "", nil, map[string]any{
		"version":      in.Version,
		"download_url": in.DownloadURL,
		"checksum":     in.Checksum,
	}, clientIP(r))

	writeJSON(w, map[string]any{"ok": true})
}
