package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"KorisPanel/panel/internal/grpcclient"
	"KorisPanel/panel/internal/noderegistry"
)

// handleKnodeNodes routes requests for the knode node collection.
//
// GET  /api/admin/knode/nodes — list all nodes with status
// POST /api/admin/knode/nodes — create a new node (validate, test connection, save)
func (s *Server) handleKnodeNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listKnodeNodes(w, r)
	case http.MethodPost:
		s.createKnodeNode(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleKnodeNodeByID routes requests for a specific knode node.
//
// GET    /api/admin/knode/nodes/{id}      — get node detail
// PUT    /api/admin/knode/nodes/{id}      — update node (reconnect with new creds)
// DELETE /api/admin/knode/nodes/{id}      — delete node (disconnect, remove record)
// POST   /api/admin/knode/nodes/{id}/test — test connection without saving
func (s *Server) handleKnodeNodeByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/admin/knode/nodes/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	switch {
	case action == "" && r.Method == http.MethodGet:
		s.getKnodeNode(w, r, id)
	case action == "" && r.Method == http.MethodPut:
		s.updateKnodeNode(w, r, id)
	case action == "" && r.Method == http.MethodDelete:
		s.deleteKnodeNode(w, r, id)
	case action == "test" && r.Method == http.MethodPost:
		s.testKnodeNode(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// knodeNodeRequest is the JSON body for creating/updating a knode connection.
type knodeNodeRequest struct {
	Name          string `json:"name"`
	Address       string `json:"address"`
	Port          int    `json:"port"`
	APIKey        string `json:"api_key"`
	ClientCertPEM string `json:"client_cert_pem"`
	ClientKeyPEM  string `json:"client_key_pem"`
	CACertPEM     string `json:"ca_cert_pem"`
}

// knodeNodeResponse is the JSON response for a single knode node.
type knodeNodeResponse struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Address         string `json:"address"`
	Port            int    `json:"port"`
	Enabled         bool   `json:"enabled"`
	Status          string `json:"status"`
	LastSeenAt      string `json:"last_seen_at,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	LastMetricsAt   string `json:"last_metrics_at,omitempty"`
	ConnectionState string `json:"connection_state,omitempty"`
	SyncFailures    int    `json:"sync_failures_count"`
}

// listKnodeNodes handles GET /api/admin/knode/nodes — list all nodes with status.
func (s *Server) listKnodeNodes(w http.ResponseWriter, r *http.Request) {
	if s.NodeRegistry == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "node_registry_not_configured"})
		return
	}

	records, err := s.NodeRegistry.ListEnabled(r.Context())
	if err != nil {
		log.Printf("[knode] ListEnabled failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "list_failed"})
		return
	}

	nodes := make([]knodeNodeResponse, 0, len(records))
	for _, rec := range records {
		nodes = append(nodes, nodeRecordToResponse(rec))
	}

	writeJSON(w, map[string]any{"ok": true, "nodes": nodes})
}

// createKnodeNode handles POST /api/admin/knode/nodes — create node.
// Validates input, tests connection via Health RPC, then saves.
func (s *Server) createKnodeNode(w http.ResponseWriter, r *http.Request) {
	if s.NodeRegistry == nil || s.GRPCPool == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	limitBody(w, r, maxJSONBody)
	var in knodeNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Default port
	if in.Port == 0 {
		in.Port = 2083
	}

	// Build NodeInput for registry
	input := &noderegistry.NodeInput{
		Name:          in.Name,
		Address:       in.Address,
		Port:          in.Port,
		APIKey:        []byte(in.APIKey),
		ClientCertPEM: []byte(in.ClientCertPEM),
		ClientKeyPEM:  []byte(in.ClientKeyPEM),
		CACertPEM:     []byte(in.CACertPEM),
		Enabled:       true,
	}

	// Test connection before saving (Health RPC stub — dial + disconnect)
	if err := s.testNodeConnection(r.Context(), in); err != nil {
		log.Printf("[knode] Test connection failed for %q at %s:%d: %v", in.Name, in.Address, in.Port, err)
		writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": "connection_failed", "detail": err.Error()})
		return
	}

	// Save to registry
	id, err := s.NodeRegistry.Create(r.Context(), input)
	if err != nil {
		log.Printf("[knode] Create node failed for %q: %v", in.Name, err)
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Establish the persistent gRPC connection
	nodeCfg, cfgErr := s.buildNodeConfigFromInput(id, in)
	if cfgErr == nil {
		connCtx, connCancel := context.WithTimeout(r.Context(), 10*time.Second)
		_ = s.GRPCPool.Connect(connCtx, nodeCfg)
		connCancel()
	}

	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// getKnodeNode handles GET /api/admin/knode/nodes/{id} — get node detail.
func (s *Server) getKnodeNode(w http.ResponseWriter, r *http.Request, id int64) {
	if s.NodeRegistry == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "node_registry_not_configured"})
		return
	}

	rec, err := s.NodeRegistry.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, noderegistry.ErrNodeNotFound) {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		log.Printf("[knode] Get node id=%d failed: %v", id, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "get_failed"})
		return
	}

	resp := nodeRecordToResponse(rec)

	// Enrich with pool status if available
	if s.GRPCPool != nil {
		poolStatus := s.GRPCPool.Status(id)
		resp.Status = string(poolStatus)
		resp.ConnectionState = string(poolStatus)

		// Get last metrics timestamp from pool connection
		conn, connErr := s.GRPCPool.Get(id)
		if connErr == nil && !conn.LastMetrics.IsZero() {
			resp.LastMetricsAt = conn.LastMetrics.UTC().Format(time.RFC3339)
		}
	}

	// Query unresolved sync_failures count for this node
	resp.SyncFailures = s.getNodeSyncFailuresCount(r.Context(), id)

	writeJSON(w, map[string]any{"ok": true, "node": resp})
}

// updateKnodeNode handles PUT /api/admin/knode/nodes/{id} — update node.
// Reconnects with new credentials after updating the record.
func (s *Server) updateKnodeNode(w http.ResponseWriter, r *http.Request, id int64) {
	if s.NodeRegistry == nil || s.GRPCPool == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	limitBody(w, r, maxJSONBody)
	var in knodeNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.Port == 0 {
		in.Port = 2083
	}

	input := &noderegistry.NodeInput{
		Name:          in.Name,
		Address:       in.Address,
		Port:          in.Port,
		APIKey:        []byte(in.APIKey),
		ClientCertPEM: []byte(in.ClientCertPEM),
		ClientKeyPEM:  []byte(in.ClientKeyPEM),
		CACertPEM:     []byte(in.CACertPEM),
		Enabled:       true,
	}

	if err := s.NodeRegistry.Update(r.Context(), id, input); err != nil {
		if errors.Is(err, noderegistry.ErrNodeNotFound) {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		log.Printf("[knode] Update node id=%d failed: %v", id, err)
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Reconnect with new credentials: disconnect existing, then connect with new config
	_ = s.GRPCPool.Disconnect(id)

	nodeCfg, cfgErr := s.buildNodeConfigFromInput(id, in)
	if cfgErr == nil {
		connCtx, connCancel := context.WithTimeout(r.Context(), 10*time.Second)
		_ = s.GRPCPool.Connect(connCtx, nodeCfg)
		connCancel()
	}

	writeJSON(w, map[string]any{"ok": true})
}

// deleteKnodeNode handles DELETE /api/admin/knode/nodes/{id} — delete node.
// Disconnects the gRPC connection and removes the record from the registry.
func (s *Server) deleteKnodeNode(w http.ResponseWriter, r *http.Request, id int64) {
	if s.NodeRegistry == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "node_registry_not_configured"})
		return
	}

	// Disconnect from the pool first (ignore errors if not connected)
	if s.GRPCPool != nil {
		_ = s.GRPCPool.Disconnect(id)
	}

	if err := s.NodeRegistry.Delete(r.Context(), id); err != nil {
		if errors.Is(err, noderegistry.ErrNodeNotFound) {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		log.Printf("[knode] Delete node id=%d failed: %v", id, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "delete_failed"})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// testKnodeNode handles POST /api/admin/knode/nodes/{id}/test — test connection without saving.
// Tests the connection for an existing node using its stored credentials.
func (s *Server) testKnodeNode(w http.ResponseWriter, r *http.Request, id int64) {
	if s.NodeRegistry == nil || s.GRPCPool == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "grpc_not_configured"})
		return
	}

	rec, err := s.NodeRegistry.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, noderegistry.ErrNodeNotFound) {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		log.Printf("[knode] Get node for test id=%d failed: %v", id, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "get_failed"})
		return
	}

	// Build a NodeConfig from the record (requires decryption)
	// Since the Registry interface doesn't expose decrypt, we attempt a pool-level test.
	// For existing nodes, we check the pool's current connection state.
	conn, getErr := s.GRPCPool.Get(id)
	if getErr != nil {
		writeJSONCode(w, http.StatusBadGateway, map[string]any{
			"ok":      false,
			"error":   "connection_failed",
			"detail":  "node is not connected to the pool",
			"status":  string(grpcclient.StatusOffline),
			"address": rec.Address,
			"port":    rec.Port,
		})
		return
	}

	writeJSON(w, map[string]any{
		"ok":      true,
		"status":  string(conn.Status),
		"address": conn.Address,
		"port":    conn.Port,
		"name":    conn.NodeName,
	})
}

// testNodeConnection attempts a gRPC dial to the node to verify connectivity.
// This is a stub: it dials with mTLS, checks if the connection succeeds, then disconnects.
func (s *Server) testNodeConnection(ctx context.Context, in knodeNodeRequest) error {
	nodeCfg, err := s.buildNodeConfigFromInput(0, in)
	if err != nil {
		return err
	}

	// Use a temporary connection via the pool mechanism:
	// Create a short-lived context for the test dial
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// We use a temporary node ID (negative) to avoid conflicting with real nodes
	testNodeID := int64(-1)
	nodeCfg.NodeID = testNodeID
	nodeCfg.Name = "test-connection"

	if err := s.GRPCPool.Connect(testCtx, nodeCfg); err != nil {
		// Clean up any leftover entry
		_ = s.GRPCPool.Disconnect(testNodeID)
		return err
	}

	// Connection succeeded — disconnect immediately (test only)
	_ = s.GRPCPool.Disconnect(testNodeID)
	return nil
}

// buildNodeConfigFromInput constructs a grpcclient.NodeConfig from the JSON input.
func (s *Server) buildNodeConfigFromInput(id int64, in knodeNodeRequest) (grpcclient.NodeConfig, error) {
	cfg := grpcclient.NodeConfig{
		NodeID:  id,
		Name:    in.Name,
		Address: in.Address,
		Port:    in.Port,
		APIKey:  in.APIKey,
		CACert:  []byte(in.CACertPEM),
	}

	// mTLS client cert is optional — if both client cert and key are provided, use them.
	// Otherwise, connect with just server-cert verification + API key auth.
	if in.ClientCertPEM != "" && in.ClientKeyPEM != "" {
		clientCert, err := tls.X509KeyPair([]byte(in.ClientCertPEM), []byte(in.ClientKeyPEM))
		if err != nil {
			return grpcclient.NodeConfig{}, fmt.Errorf("invalid client cert/key pair: %w", err)
		}
		cfg.ClientCert = clientCert
	}

	return cfg, nil
}

// nodeRecordToResponse converts a NodeRecord to the API response format.
func nodeRecordToResponse(rec *noderegistry.NodeRecord) knodeNodeResponse {
	resp := knodeNodeResponse{
		ID:        rec.ID,
		Name:      rec.Name,
		Address:   rec.Address,
		Port:      rec.Port,
		Enabled:   rec.Enabled,
		Status:    rec.Status,
		CreatedAt: rec.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: rec.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if rec.LastSeenAt.Valid {
		resp.LastSeenAt = rec.LastSeenAt.Time.UTC().Format(time.RFC3339)
	}
	return resp
}

// getNodeSyncFailuresCount returns the count of unresolved sync_failures for a node.
func (s *Server) getNodeSyncFailuresCount(ctx context.Context, nodeID int64) int {
	var count int
	err := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sync_failures WHERE node_id = ? AND resolved = FALSE`,
		nodeID,
	).Scan(&count)
	if err != nil {
		// Table might not exist yet or query error — return 0
		return 0
	}
	return count
}
