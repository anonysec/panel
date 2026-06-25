package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// handleNodeFirewall routes firewall requests by method.
//
// GET  /api/admin/nodes/firewall?node_id=X — list firewall rules
// POST /api/admin/nodes/firewall            — open a port
// DELETE /api/admin/nodes/firewall          — close a port
func (s *Server) handleNodeFirewall(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listFirewallRules(w, r)
	case http.MethodPost:
		s.openFirewallPort(w, r)
	case http.MethodDelete:
		s.closeFirewallPort(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listFirewallRules handles GET /api/admin/nodes/firewall?node_id=X.
// Calls ListFirewallRules on the target knode via gRPC.
func (s *Server) listFirewallRules(w http.ResponseWriter, r *http.Request) {
	nodeIDStr := r.URL.Query().Get("node_id")
	nodeID, err := strconv.ParseInt(nodeIDStr, 10, 64)
	if err != nil || nodeID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node_id"})
		return
	}

	if s.FirewallMgr == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	rules, err := s.FirewallMgr.ListFirewallRules(r.Context(), nodeID)
	if err != nil {
		log.Printf("[knode] ListFirewallRules failed for node %d: %v", nodeID, err)
		writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "rules": rules})
}

// openFirewallPort handles POST /api/admin/nodes/firewall.
// Body: {"node_id": X, "port": N, "protocol": "tcp|udp|both", "comment": "..."}
func (s *Server) openFirewallPort(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		NodeID   int64  `json:"node_id"`
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
		Comment  string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.NodeID <= 0 || in.Port <= 0 || in.Port > 65535 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_params"})
		return
	}
	if in.Protocol == "" {
		in.Protocol = "both"
	}
	if in.Protocol != "tcp" && in.Protocol != "udp" && in.Protocol != "both" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_protocol"})
		return
	}

	if s.FirewallMgr == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	if err := s.FirewallMgr.OpenPort(r.Context(), in.NodeID, in.Port, in.Protocol, in.Comment); err != nil {
		log.Printf("[knode] OpenPort failed for node %d port %d/%s: %v", in.NodeID, in.Port, in.Protocol, err)
		writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// closeFirewallPort handles DELETE /api/admin/nodes/firewall.
// Body: {"node_id": X, "port": N, "protocol": "tcp|udp|both"}
func (s *Server) closeFirewallPort(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		NodeID   int64  `json:"node_id"`
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.NodeID <= 0 || in.Port <= 0 || in.Port > 65535 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_params"})
		return
	}
	if in.Protocol == "" {
		in.Protocol = "both"
	}
	if in.Protocol != "tcp" && in.Protocol != "udp" && in.Protocol != "both" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_protocol"})
		return
	}

	if s.FirewallMgr == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	if err := s.FirewallMgr.ClosePort(r.Context(), in.NodeID, in.Port, in.Protocol); err != nil {
		log.Printf("[knode] ClosePort failed for node %d port %d/%s: %v", in.NodeID, in.Port, in.Protocol, err)
		writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}
