package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// NodeBulkRequest represents a request to perform bulk operations on nodes.
type NodeBulkRequest struct {
	Action  string         `json:"action"`
	NodeIDs []int64        `json:"node_ids"`
	Params  map[string]any `json:"params"`
}

// NodeBulkResult represents the result for a single node in a bulk operation.
type NodeBulkResult struct {
	NodeID  int64  `json:"node_id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// validNodeBulkActions defines the set of supported bulk actions.
var validNodeBulkActions = map[string]bool{
	"restart_openvpn":  true,
	"restart_all":      true,
	"push_config":      true,
	"enable_protocol":  true,
	"disable_protocol": true,
	"run_command":      true,
	"maintenance_on":   true,
	"maintenance_off":  true,
}

// nodeBulk handles POST /api/admin/nodes/bulk
func (s *Server) nodeBulk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var req NodeBulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Validate action
	if !validNodeBulkActions[req.Action] {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_action"})
		return
	}

	// Validate node_ids
	if len(req.NodeIDs) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_ids_required"})
		return
	}
	if len(req.NodeIDs) > 50 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "too_many_nodes"})
		return
	}

	// Validate params for actions that require them
	if req.Action == "enable_protocol" || req.Action == "disable_protocol" {
		proto, _ := req.Params["protocol"].(string)
		if proto == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "protocol_required"})
			return
		}
	}
	if req.Action == "run_command" {
		cmd, _ := req.Params["command"].(string)
		if cmd == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "command_required"})
			return
		}
	}

	actor, _, _ := s.currentAdmin(r)
	ip := clientIP(r)

	results := make([]NodeBulkResult, 0, len(req.NodeIDs))

	for _, nodeID := range req.NodeIDs {
		result := NodeBulkResult{NodeID: nodeID}

		// Check node exists
		var exists int
		err := s.DB.QueryRow(`SELECT 1 FROM nodes WHERE id=$1 LIMIT 1`, nodeID).Scan(&exists)
		if err != nil {
			result.Error = "node not found"
			results = append(results, result)
			continue
		}

		switch req.Action {
		case "maintenance_on":
			err = s.nodeBulkSetMaintenance(nodeID, true)
		case "maintenance_off":
			err = s.nodeBulkSetMaintenance(nodeID, false)
		case "enable_protocol":
			err = s.nodeBulkEnableProtocol(r.Context(), nodeID, req.Params)
		case "disable_protocol":
			err = s.nodeBulkDisableProtocol(r.Context(), nodeID, req.Params)
		default:
			err = s.nodeBulkDispatchGRPC(r.Context(), nodeID, req.Action, req.Params)
		}

		if err != nil {
			result.Error = err.Error()
		} else {
			result.Success = true
		}
		results = append(results, result)
	}

	// Log audit trail
	s.logAudit(actor, "nodes.bulk_action", "node", "", nil, map[string]any{
		"action":   req.Action,
		"node_ids": req.NodeIDs,
		"params":   req.Params,
	}, ip)

	log.Printf("[nodes] bulk action=%s nodes=%d by=%s", req.Action, len(req.NodeIDs), actor)

	writeJSON(w, map[string]any{
		"ok":      true,
		"results": results,
	})
}

// nodeBulkEnableProtocol enables a protocol (core) on a node via gRPC.
func (s *Server) nodeBulkEnableProtocol(ctx context.Context, nodeID int64, params map[string]any) error {
	proto, _ := params["protocol"].(string)
	if proto == "" {
		return fmt.Errorf("protocol required")
	}

	if s.CoreMgr == nil {
		return fmt.Errorf("grpc not configured")
	}

	port := 0
	if p, ok := params["port"].(float64); ok {
		port = int(p)
	}

	return s.CoreMgr.EnableCore(ctx, nodeID, proto, port, nil)
}

// nodeBulkDisableProtocol disables a protocol (core) on a node via gRPC.
func (s *Server) nodeBulkDisableProtocol(ctx context.Context, nodeID int64, params map[string]any) error {
	proto, _ := params["protocol"].(string)
	if proto == "" {
		return fmt.Errorf("protocol required")
	}

	if s.CoreMgr == nil {
		return fmt.Errorf("grpc not configured")
	}

	return s.CoreMgr.DisableCore(ctx, nodeID, proto)
}

// nodeBulkDispatchGRPC dispatches bulk actions via gRPC.
// For actions that don't have a direct gRPC mapping, logs and returns success
// (these will be fully handled when xray/custom command gRPC wrappers are added).
func (s *Server) nodeBulkDispatchGRPC(ctx context.Context, nodeID int64, action string, params map[string]any) error {
	switch action {
	case "restart_openvpn":
		// Restart = disable + enable
		if s.CoreMgr != nil {
			_ = s.CoreMgr.DisableCore(ctx, nodeID, "openvpn")
			return s.CoreMgr.EnableCore(ctx, nodeID, "openvpn", 0, nil)
		}
		return fmt.Errorf("grpc not configured")
	case "restart_all":
		// Restart all cores on the node
		if s.CoreMgr != nil {
			statuses, err := s.CoreMgr.AllCoreStatuses(ctx, nodeID)
			if err != nil {
				return err
			}
			for _, cs := range statuses {
				if cs.State == "running" {
					_ = s.CoreMgr.DisableCore(ctx, nodeID, cs.Type)
					_ = s.CoreMgr.EnableCore(ctx, nodeID, cs.Type, 0, nil)
				}
			}
			return nil
		}
		return fmt.Errorf("grpc not configured")
	case "push_config":
		// Trigger user sync for the node
		if s.UserSync != nil {
			return s.UserSync.FullSyncForNode(ctx, nodeID)
		}
		return fmt.Errorf("grpc not configured")
	case "run_command":
		// Custom commands not yet supported via gRPC — log and report
		cmd, _ := params["command"].(string)
		log.Printf("[knode] bulk run_command for node %d: %q — not yet supported via gRPC", nodeID, cmd)
		return fmt.Errorf("run_command not yet supported via gRPC")
	default:
		return fmt.Errorf("unsupported action: %s", action)
	}
}

// nodeBulkSetMaintenance updates the maintenance_mode flag on a node.
func (s *Server) nodeBulkSetMaintenance(nodeID int64, enabled bool) error {
	_, err := s.DB.Exec(`UPDATE nodes SET maintenance_mode=$1 WHERE id=$2`, enabled, nodeID)
	if err != nil {
		return fmt.Errorf("failed to update maintenance mode: %v", err)
	}
	return nil
}
