package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// handleCores handles GET /api/cores (list) and POST /api/cores (register).
func (s *Server) handleCores(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listCores(w, r)
	case http.MethodPost:
		s.registerCore(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listCores returns all available core plugins from the registry.
func (s *Server) listCores(w http.ResponseWriter, _ *http.Request) {
	rows, err := s.DB.Query(`SELECT id, name, version, download_url, checksum_sha256, protocols_json, COALESCE(config_template, ''), created_at FROM core_plugins ORDER BY name, version`)
	if err != nil {
		log.Printf("[cores] list query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type corePlugin struct {
		ID             int64    `json:"id"`
		Name           string   `json:"name"`
		Version        string   `json:"version"`
		DownloadURL    string   `json:"download_url"`
		ChecksumSHA256 string   `json:"checksum_sha256"`
		Protocols      []string `json:"protocols"`
		ConfigTemplate string   `json:"config_template"`
		CreatedAt      string   `json:"created_at"`
	}

	var cores []corePlugin
	for rows.Next() {
		var c corePlugin
		var protocolsJSON string
		if err := rows.Scan(&c.ID, &c.Name, &c.Version, &c.DownloadURL, &c.ChecksumSHA256, &protocolsJSON, &c.ConfigTemplate, &c.CreatedAt); err != nil {
			log.Printf("[cores] scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		if protocolsJSON != "" {
			_ = json.Unmarshal([]byte(protocolsJSON), &c.Protocols)
		}
		if c.Protocols == nil {
			c.Protocols = []string{}
		}
		cores = append(cores, c)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[cores] rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if cores == nil {
		cores = []corePlugin{}
	}
	writeJSON(w, map[string]any{"ok": true, "cores": cores})
}

// registerCore registers a new core plugin version.
func (s *Server) registerCore(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Name           string   `json:"name"`
		Version        string   `json:"version"`
		DownloadURL    string   `json:"download_url"`
		ChecksumSHA256 string   `json:"checksum_sha256"`
		Protocols      []string `json:"protocols"`
		ConfigTemplate string   `json:"config_template"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.Name == "" || in.Version == "" || in.DownloadURL == "" || in.ChecksumSHA256 == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
		return
	}
	if len(in.Protocols) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "protocols_required"})
		return
	}

	protocolsBytes, err := json.Marshal(in.Protocols)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "encoding_error"})
		return
	}

	result, err := s.DB.Exec(`INSERT INTO core_plugins (name, version, download_url, checksum_sha256, protocols_json, config_template) VALUES (?, ?, ?, ?, ?, ?)`,
		in.Name, in.Version, in.DownloadURL, in.ChecksumSHA256, string(protocolsBytes), in.ConfigTemplate)
	if err != nil {
		log.Printf("[cores] insert failed: %v", err)
		if strings.Contains(err.Error(), "Duplicate") {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "core_version_exists"})
			return
		}
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// dispatchNodeCores routes /api/nodes/{id}/cores/{sub} to the appropriate handler.
// Called from nodeByID when action == "cores".
func (s *Server) dispatchNodeCores(w http.ResponseWriter, r *http.Request, nodeID int64) {
	// Parse the sub-action from the URL: /api/nodes/{id}/cores/{sub}
	// pathID already consumed parts[0]=id, parts[1]="cores"
	// We need parts[2] which is "install", "update", or the core name for DELETE
	rest := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
	parts := strings.Split(rest, "/")
	// parts: ["{id}", "cores", "{sub}"]
	sub := ""
	if len(parts) >= 3 {
		sub = parts[2]
	}

	switch {
	case r.Method == http.MethodPost && sub == "install":
		s.nodeCoresInstall(w, r, nodeID)
	case r.Method == http.MethodPost && sub == "update":
		s.nodeCoresUpdate(w, r, nodeID)
	case r.Method == http.MethodDelete && sub != "":
		s.nodeCoresRemove(w, r, nodeID, sub)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

// nodeCoresInstall handles POST /api/nodes/{id}/cores/install.
// Uses direct gRPC EnableCore call instead of creating a node_task.
func (s *Server) nodeCoresInstall(w http.ResponseWriter, r *http.Request, nodeID int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		CoreName string `json:"core_name"`
		Version  string `json:"version"`
		Port     int    `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.CoreName == "" || in.Version == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
		return
	}

	// Look up core plugin
	var downloadURL, checksum string
	err := s.DB.QueryRow(`SELECT download_url, checksum_sha256 FROM core_plugins WHERE name = ? AND version = ?`, in.CoreName, in.Version).Scan(&downloadURL, &checksum)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "core_not_found"})
		return
	}

	// Insert into node_cores
	_, err = s.DB.Exec(`INSERT INTO node_cores (node_id, core_name, version, status) VALUES (?, ?, ?, 'pending') ON DUPLICATE KEY UPDATE version = VALUES(version), status = 'pending'`,
		nodeID, in.CoreName, in.Version)
	if err != nil {
		log.Printf("[cores] node_cores insert failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// Call EnableCore via gRPC instead of creating a node_task
	if s.CoreMgr != nil {
		extraConfig, _ := json.Marshal(map[string]string{
			"download_url":    downloadURL,
			"checksum_sha256": checksum,
			"version":         in.Version,
		})
		if err := s.CoreMgr.EnableCore(r.Context(), nodeID, in.CoreName, in.Port, extraConfig); err != nil {
			log.Printf("[cores] EnableCore gRPC failed for node %d core %s: %v", nodeID, in.CoreName, err)
			writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		// Update node_cores status to installed on success
		_, _ = s.DB.Exec(`UPDATE node_cores SET status = 'installed' WHERE node_id = ? AND core_name = ?`, nodeID, in.CoreName)
	} else {
		log.Printf("[cores] gRPC pool not configured, cannot install core %s on node %d", in.CoreName, nodeID)
	}

	writeJSON(w, map[string]any{"ok": true})
}

// nodeCoresUpdate handles POST /api/nodes/{id}/cores/update.
// Uses direct gRPC EnableCore call instead of creating a node_task.
func (s *Server) nodeCoresUpdate(w http.ResponseWriter, r *http.Request, nodeID int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		CoreName string `json:"core_name"`
		Version  string `json:"version"`
		Port     int    `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.CoreName == "" || in.Version == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
		return
	}

	// Look up new version from core_plugins
	var downloadURL, checksum string
	err := s.DB.QueryRow(`SELECT download_url, checksum_sha256 FROM core_plugins WHERE name = ? AND version = ?`, in.CoreName, in.Version).Scan(&downloadURL, &checksum)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "core_not_found"})
		return
	}

	// Update node_cores
	result, err := s.DB.Exec(`UPDATE node_cores SET version = ?, status = 'pending' WHERE node_id = ? AND core_name = ?`,
		in.Version, nodeID, in.CoreName)
	if err != nil {
		log.Printf("[cores] node_cores update failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "core_not_installed"})
		return
	}

	// Call EnableCore via gRPC (update is effectively re-enable with new version config)
	if s.CoreMgr != nil {
		extraConfig, _ := json.Marshal(map[string]string{
			"download_url":    downloadURL,
			"checksum_sha256": checksum,
			"version":         in.Version,
		})
		if err := s.CoreMgr.EnableCore(r.Context(), nodeID, in.CoreName, in.Port, extraConfig); err != nil {
			log.Printf("[cores] EnableCore (update) gRPC failed for node %d core %s: %v", nodeID, in.CoreName, err)
			writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		// Update node_cores status to installed on success
		_, _ = s.DB.Exec(`UPDATE node_cores SET status = 'installed' WHERE node_id = ? AND core_name = ?`, nodeID, in.CoreName)
	} else {
		log.Printf("[cores] gRPC pool not configured, cannot update core %s on node %d", in.CoreName, nodeID)
	}

	writeJSON(w, map[string]any{"ok": true})
}

// nodeCoresRemove handles DELETE /api/nodes/{id}/cores/{name}.
// Uses direct gRPC DisableCore call instead of creating a node_task.
func (s *Server) nodeCoresRemove(w http.ResponseWriter, _ *http.Request, nodeID int64, coreName string) {
	// Check if any active xray_inbounds depend on this core
	var activeCount int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM xray_inbounds WHERE node_id = ? AND core_name = ? AND status = 'active'`, nodeID, coreName).Scan(&activeCount)
	if err != nil {
		log.Printf("[cores] active inbounds check failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if activeCount > 0 {
		writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "core_has_active_inbounds"})
		return
	}

	// Call DisableCore via gRPC
	if s.CoreMgr != nil {
		ctx := context.Background()
		if err := s.CoreMgr.DisableCore(ctx, nodeID, coreName); err != nil {
			log.Printf("[cores] DisableCore gRPC failed for node %d core %s: %v", nodeID, coreName, err)
			writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	} else {
		log.Printf("[cores] gRPC pool not configured, cannot remove core %s on node %d", coreName, nodeID)
	}

	// Delete from node_cores
	_, err = s.DB.Exec(`DELETE FROM node_cores WHERE node_id = ? AND core_name = ?`, nodeID, coreName)
	if err != nil {
		log.Printf("[cores] node_cores delete failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}
