package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// nodeTagsAll handles GET /api/admin/nodes/tags — returns all unique tags across all nodes.
func (s *Server) nodeTagsAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	rows, err := s.DB.Query(`SELECT DISTINCT tag FROM node_tags ORDER BY tag`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	tags := []string{}
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "scan_error"})
			return
		}
		tags = append(tags, tag)
	}

	writeJSON(w, map[string]any{"ok": true, "tags": tags})
}

// nodeTagsByID handles GET/POST/DELETE /api/admin/nodes/:id/tags and /api/admin/nodes/:id/alerts
func (s *Server) nodeTagsByID(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/admin/nodes/{id}/{sub}
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/nodes/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// Dispatch to alerts handler
	if parts[1] == "alerts" {
		s.nodeAlerts(w, r)
		return
	}

	// Dispatch to quota handler
	if parts[1] == "quota" || parts[1] == "bandwidth-quota" {
		s.nodeQuota(w, r)
		return
	}

	// Dispatch to maintenance handler
	if parts[1] == "maintenance" {
		nodeID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || nodeID <= 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node_id"})
			return
		}
		s.nodeMaintenance(w, r, nodeID)
		return
	}

	// Dispatch to SLA handler
	if parts[1] == "sla" {
		nodeID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || nodeID <= 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node_id"})
			return
		}
		s.nodeSLA(w, r, nodeID)
		return
	}

	// Dispatch to anti-DPI handler
	if parts[1] == "anti-dpi" {
		nodeID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || nodeID <= 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node_id"})
			return
		}
		technique := ""
		if len(parts) >= 3 {
			technique = parts[2]
		}
		s.handleNodeAntiDPI(w, r, nodeID, technique)
		return
	}

	// Dispatch to metrics/history handler
	if parts[1] == "metrics" {
		nodeID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || nodeID <= 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node_id"})
			return
		}
		if len(parts) >= 3 && parts[2] == "history" {
			s.handleNodeMetricsHistory(w, r, nodeID)
			return
		}
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	if parts[1] != "tags" {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	nodeID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || nodeID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node_id"})
		return
	}

	// Verify node exists
	var exists int
	if err := s.DB.QueryRow(`SELECT 1 FROM nodes WHERE id=$1 LIMIT 1`, nodeID).Scan(&exists); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "node_not_found"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getNodeTags(w, nodeID)
	case http.MethodPost:
		s.addNodeTag(w, r, nodeID)
	case http.MethodDelete:
		s.removeNodeTag(w, r, nodeID)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// getNodeTags returns tags for a specific node.
func (s *Server) getNodeTags(w http.ResponseWriter, nodeID int64) {
	rows, err := s.DB.Query(`SELECT tag FROM node_tags WHERE node_id=$1 ORDER BY tag`, nodeID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	tags := []string{}
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "scan_error"})
			return
		}
		tags = append(tags, tag)
	}

	writeJSON(w, map[string]any{"ok": true, "tags": tags})
}

// addNodeTag adds a tag to a node (INSERT IGNORE for idempotency).
func (s *Server) addNodeTag(w http.ResponseWriter, r *http.Request, nodeID int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Tag string `json:"tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Tag = strings.TrimSpace(in.Tag)
	if in.Tag == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "tag_required"})
		return
	}
	if len(in.Tag) > 50 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "tag_too_long"})
		return
	}

	_, err := s.DB.Exec(`INSERT INTO node_tags (node_id, tag) VALUES ($1, $2) ON CONFLICT (node_id, tag) DO NOTHING`, nodeID, in.Tag)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	if s.Cache != nil {
		s.Cache.InvalidatePrefix("nodes:")
	}

	writeJSON(w, map[string]any{"ok": true})
}

// removeNodeTag removes a tag from a node.
func (s *Server) removeNodeTag(w http.ResponseWriter, r *http.Request, nodeID int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Tag string `json:"tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Tag = strings.TrimSpace(in.Tag)
	if in.Tag == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "tag_required"})
		return
	}

	_, err := s.DB.Exec(`DELETE FROM node_tags WHERE node_id=$1 AND tag=$2`, nodeID, in.Tag)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	if s.Cache != nil {
		s.Cache.InvalidatePrefix("nodes:")
	}

	writeJSON(w, map[string]any{"ok": true})
}
