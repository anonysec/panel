package api

import (
	"KorisPanel/panel/internal/grpcclient"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// ========== Tunnel Management via gRPC ==========

// handleNodeTunnels routes tunnel requests by method.
//
// GET    /api/admin/nodes/tunnels?node_id=X — list active tunnels
// POST   /api/admin/nodes/tunnels           — set up a tunnel
// DELETE /api/admin/nodes/tunnels           — tear down a tunnel
func (s *Server) handleNodeTunnels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listNodeTunnels(w, r)
	case http.MethodPost:
		s.setupNodeTunnel(w, r)
	case http.MethodDelete:
		s.teardownNodeTunnel(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listNodeTunnels handles GET /api/admin/nodes/tunnels?node_id=X.
// Calls TunnelStatus on the target knode via gRPC.
func (s *Server) listNodeTunnels(w http.ResponseWriter, r *http.Request) {
	nodeIDStr := r.URL.Query().Get("node_id")
	nodeID, err := strconv.ParseInt(nodeIDStr, 10, 64)
	if err != nil || nodeID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node_id"})
		return
	}

	if s.TunnelMgr == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	tunnels, err := s.TunnelMgr.TunnelStatus(r.Context(), nodeID)
	if err != nil {
		log.Printf("[knode] TunnelStatus failed for node %d: %v", nodeID, err)
		writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "tunnels": tunnels})
}

// setupNodeTunnel handles POST /api/admin/nodes/tunnels.
// Body: {"node_id": X, "protocol": "...", "exit_address": "...", "exit_port": N, "extra_config": {...}}
func (s *Server) setupNodeTunnel(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		NodeID      int64             `json:"node_id"`
		Protocol    string            `json:"protocol"`
		ExitAddress string            `json:"exit_address"`
		ExitPort    int               `json:"exit_port"`
		ExtraConfig map[string]string `json:"extra_config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.NodeID <= 0 || in.Protocol == "" || in.ExitAddress == "" || in.ExitPort <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
		return
	}

	if s.TunnelMgr == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	cfg := grpcclient.TunnelConfig{
		Protocol:    in.Protocol,
		ExitAddress: in.ExitAddress,
		ExitPort:    in.ExitPort,
		ExtraConfig: in.ExtraConfig,
	}

	tunnelID, err := s.TunnelMgr.SetupTunnel(r.Context(), in.NodeID, cfg)
	if err != nil {
		log.Printf("[knode] SetupTunnel failed for node %d: %v", in.NodeID, err)
		writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "tunnel_id": tunnelID})
}

// teardownNodeTunnel handles DELETE /api/admin/nodes/tunnels.
// Body: {"node_id": X, "tunnel_id": "..."}
func (s *Server) teardownNodeTunnel(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		NodeID   int64  `json:"node_id"`
		TunnelID string `json:"tunnel_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.NodeID <= 0 || in.TunnelID == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
		return
	}

	if s.TunnelMgr == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	if err := s.TunnelMgr.TeardownTunnel(r.Context(), in.NodeID, in.TunnelID); err != nil {
		log.Printf("[knode] TeardownTunnel failed for node %d tunnel %s: %v", in.NodeID, in.TunnelID, err)
		writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// ========== Certificate Management via gRPC ==========

// handleNodeCerts routes cert requests by method.
//
// GET  /api/admin/nodes/certs?node_id=X — get cert info from knode
// POST /api/admin/nodes/certs           — push certificates to knode
func (s *Server) handleNodeCerts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getNodeCertInfo(w, r)
	case http.MethodPost:
		s.pushNodeCerts(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getNodeCertInfo handles GET /api/admin/nodes/certs?node_id=X.
// Calls GetCertInfo on the target knode via gRPC.
func (s *Server) getNodeCertInfo(w http.ResponseWriter, r *http.Request) {
	nodeIDStr := r.URL.Query().Get("node_id")
	nodeID, err := strconv.ParseInt(nodeIDStr, 10, 64)
	if err != nil || nodeID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node_id"})
		return
	}

	if s.CertMgr == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	certs, err := s.CertMgr.GetCertInfo(r.Context(), nodeID)
	if err != nil {
		log.Printf("[knode] GetCertInfo failed for node %d: %v", nodeID, err)
		writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "certs": certs})
}

// pushNodeCerts handles POST /api/admin/nodes/certs.
// Body: {"node_id": X, "core_type": "...", "ca_cert": "...", "cert": "...", "key": "..."}
func (s *Server) pushNodeCerts(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		NodeID   int64  `json:"node_id"`
		CoreType string `json:"core_type"`
		CACert   string `json:"ca_cert"`
		Cert     string `json:"cert"`
		Key      string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.NodeID <= 0 || in.CoreType == "" || in.CACert == "" || in.Cert == "" || in.Key == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
		return
	}

	if s.CertMgr == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	if err := s.CertMgr.SetCertificates(r.Context(), in.NodeID, in.CoreType, []byte(in.CACert), []byte(in.Cert), []byte(in.Key)); err != nil {
		log.Printf("[knode] SetCertificates failed for node %d core %s: %v", in.NodeID, in.CoreType, err)
		writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// ========== Session Management via gRPC ==========

// handleNodeSessions routes session requests by method.
//
// GET    /api/admin/nodes/sessions?node_id=X — list active VPN sessions
// DELETE /api/admin/nodes/sessions           — disconnect a user
func (s *Server) handleNodeSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listNodeSessions(w, r)
	case http.MethodDelete:
		s.disconnectNodeUser(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listNodeSessions handles GET /api/admin/nodes/sessions?node_id=X.
// Calls ListSessions on the target knode via gRPC.
func (s *Server) listNodeSessions(w http.ResponseWriter, r *http.Request) {
	nodeIDStr := r.URL.Query().Get("node_id")
	nodeID, err := strconv.ParseInt(nodeIDStr, 10, 64)
	if err != nil || nodeID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node_id"})
		return
	}

	if s.SessionMgr == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	sessions, err := s.SessionMgr.ListSessions(r.Context(), nodeID)
	if err != nil {
		log.Printf("[knode] ListSessions failed for node %d: %v", nodeID, err)
		writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "sessions": sessions})
}

// disconnectNodeUser handles DELETE /api/admin/nodes/sessions.
// Body: {"node_id": X, "username": "...", "core_filter": "..."}
func (s *Server) disconnectNodeUser(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		NodeID     int64  `json:"node_id"`
		Username   string `json:"username"`
		CoreFilter string `json:"core_filter"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.NodeID <= 0 || in.Username == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
		return
	}

	if s.SessionMgr == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	if err := s.SessionMgr.DisconnectUser(r.Context(), in.NodeID, in.Username, in.CoreFilter); err != nil {
		log.Printf("[knode] DisconnectUser failed for node %d user %s: %v", in.NodeID, in.Username, err)
		writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}
