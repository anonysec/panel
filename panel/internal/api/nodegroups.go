package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

func (s *Server) handleNodeGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listNodeGroups(w, r)
	case http.MethodPost:
		s.createNodeGroup(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleNodeGroupByID(w http.ResponseWriter, r *http.Request) {
	id, _, ok := pathID(r.URL.Path, "/api/node-groups/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		s.updateNodeGroup(w, r, id)
	case http.MethodDelete:
		s.deleteNodeGroup(w, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePortalNodeGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.portalNodeGroups(w, r)
}

func (s *Server) listNodeGroups(w http.ResponseWriter, _ *http.Request) {
	rows, err := s.DB.Query("SELECT ng.id, ng.name, ng.region, COALESCE(ng.description, ''), ng.load_balancing_enabled, ng.max_load_percent, ng.created_at, COUNT(n.id) AS node_count FROM node_groups ng LEFT JOIN nodes n ON n.group_id = ng.id GROUP BY ng.id ORDER BY ng.created_at")
	if err != nil {
		log.Printf("[nodegroups] list query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()
	type nodeGroup struct {
		ID                   int64  `json:"id"`
		Name                 string `json:"name"`
		Region               string `json:"region"`
		Description          string `json:"description"`
		LoadBalancingEnabled bool   `json:"load_balancing_enabled"`
		MaxLoadPercent       int    `json:"max_load_percent"`
		CreatedAt            string `json:"created_at"`
		NodeCount            int    `json:"node_count"`
	}
	var groups []nodeGroup
	for rows.Next() {
		var g nodeGroup
		var lbEnabled int
		if err := rows.Scan(&g.ID, &g.Name, &g.Region, &g.Description, &lbEnabled, &g.MaxLoadPercent, &g.CreatedAt, &g.NodeCount); err != nil {
			log.Printf("[nodegroups] scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		g.LoadBalancingEnabled = lbEnabled == 1
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[nodegroups] rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if groups == nil {
		groups = []nodeGroup{}
	}
	writeJSON(w, map[string]any{"ok": true, "groups": groups})
}

func (s *Server) createNodeGroup(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Name                 string `json:"name"`
		Region               string `json:"region"`
		Description          string `json:"description"`
		LoadBalancingEnabled *bool  `json:"load_balancing_enabled"`
		MaxLoadPercent       *int   `json:"max_load_percent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.Name == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_required"})
		return
	}
	lbEnabled := 0
	if in.LoadBalancingEnabled != nil && *in.LoadBalancingEnabled {
		lbEnabled = 1
	}
	maxLoad := 85
	if in.MaxLoadPercent != nil && *in.MaxLoadPercent > 0 && *in.MaxLoadPercent <= 100 {
		maxLoad = *in.MaxLoadPercent
	}
	result, err := s.DB.Exec("INSERT INTO node_groups (name, region, description, load_balancing_enabled, max_load_percent) VALUES (?, ?, ?, ?, ?)", in.Name, in.Region, in.Description, lbEnabled, maxLoad)
	if err != nil {
		log.Printf("[nodegroups] insert failed: %v", err)
		if strings.Contains(err.Error(), "Duplicate") {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "group_already_exists"})
			return
		}
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) updateNodeGroup(w http.ResponseWriter, r *http.Request, id int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Name                 *string `json:"name"`
		Region               *string `json:"region"`
		Description          *string `json:"description"`
		LoadBalancingEnabled *bool   `json:"load_balancing_enabled"`
		MaxLoadPercent       *int    `json:"max_load_percent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	var setClauses []string
	var args []any
	if in.Name != nil {
		setClauses = append(setClauses, "name = $1")
		args = append(args, *in.Name)
	}
	if in.Region != nil {
		setClauses = append(setClauses, "region = $1")
		args = append(args, *in.Region)
	}
	if in.Description != nil {
		setClauses = append(setClauses, "description = $1")
		args = append(args, *in.Description)
	}
	if in.LoadBalancingEnabled != nil {
		lbEnabled := 0
		if *in.LoadBalancingEnabled {
			lbEnabled = 1
		}
		setClauses = append(setClauses, "load_balancing_enabled = $1")
		args = append(args, lbEnabled)
	}
	if in.MaxLoadPercent != nil {
		if *in.MaxLoadPercent > 0 && *in.MaxLoadPercent <= 100 {
			setClauses = append(setClauses, "max_load_percent = $1")
			args = append(args, *in.MaxLoadPercent)
		}
	}
	if len(setClauses) == 0 {
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	args = append(args, id)
	query := "UPDATE node_groups SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	result, err := s.DB.Exec(query, args...)
	if err != nil {
		log.Printf("[nodegroups] update failed: %v", err)
		if strings.Contains(err.Error(), "Duplicate") {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "group_already_exists"})
			return
		}
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) deleteNodeGroup(w http.ResponseWriter, id int64) {
	result, err := s.DB.Exec("DELETE FROM node_groups WHERE id = ?", id)
	if err != nil {
		log.Printf("[nodegroups] delete failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) assignNodeToGroup(w http.ResponseWriter, r *http.Request, nodeID int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		GroupID *int64 `json:"group_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.GroupID != nil && *in.GroupID > 0 {
		var exists int
		if err := s.DB.QueryRow("SELECT COUNT(*) FROM node_groups WHERE id = ?", *in.GroupID).Scan(&exists); err != nil || exists == 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "group_not_found"})
			return
		}
	}
	var groupVal any
	if in.GroupID != nil && *in.GroupID > 0 {
		groupVal = *in.GroupID
	}
	result, err := s.DB.Exec("UPDATE nodes SET group_id = ? WHERE id = ?", groupVal, nodeID)
	if err != nil {
		log.Printf("[nodegroups] assign node %d to group failed: %v", nodeID, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "node_not_found"})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) portalNodeGroups(w http.ResponseWriter, _ *http.Request) {
	rows, err := s.DB.Query("SELECT ng.id, ng.name, ng.region, COUNT(n.id) AS node_count, COALESCE(SUM(n.max_capacity), 0) AS total_capacity, COALESCE(SUM(CASE WHEN n.status = 'online' THEN 1 ELSE 0 END), 0) AS available_nodes FROM node_groups ng LEFT JOIN nodes n ON n.group_id = ng.id GROUP BY ng.id ORDER BY ng.name")
	if err != nil {
		log.Printf("[nodegroups] portal list query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()
	type portalGroup struct {
		ID             int64  `json:"id"`
		Name           string `json:"name"`
		Region         string `json:"region"`
		NodeCount      int    `json:"node_count"`
		TotalCapacity  int    `json:"total_capacity"`
		AvailableNodes int    `json:"available_nodes"`
	}
	var groups []portalGroup
	for rows.Next() {
		var g portalGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Region, &g.NodeCount, &g.TotalCapacity, &g.AvailableNodes); err != nil {
			log.Printf("[nodegroups] portal scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[nodegroups] portal rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if groups == nil {
		groups = []portalGroup{}
	}
	writeJSON(w, map[string]any{"ok": true, "groups": groups})
}
