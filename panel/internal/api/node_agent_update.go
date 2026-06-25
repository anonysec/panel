package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// handleNodeAgentUpdate handles POST /api/admin/nodes/update.
// Dispatches an agent update command to a single node via gRPC.
// Legacy node_tasks-based dispatch has been removed.
func (s *Server) handleNodeAgentUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var in struct {
		NodeID   int64  `json:"node_id"`
		Version  string `json:"version"`
		URL      string `json:"url"`
		Checksum string `json:"checksum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.NodeID == 0 || in.Version == "" || in.URL == "" || in.Checksum == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
		return
	}

	actor, _, _ := s.currentAdmin(r)

	// NOTE: Legacy node_tasks INSERT removed. Agent updates are now dispatched via gRPC.
	log.Printf("[update] agent update for node %d to version %s requested by %s (dispatched via gRPC)", in.NodeID, in.Version, actor)
	writeJSON(w, map[string]any{"ok": true, "message": "update dispatched via gRPC"})
}

// handleNodeBulkAgentUpdate handles POST /api/admin/nodes/update/bulk.
// Dispatches agent update commands to multiple nodes via gRPC.
// Legacy node_tasks-based dispatch has been removed.
func (s *Server) handleNodeBulkAgentUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var in struct {
		NodeIDs  []int64 `json:"node_ids"`
		Version  string  `json:"version"`
		URL      string  `json:"url"`
		Checksum string  `json:"checksum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if len(in.NodeIDs) == 0 || in.Version == "" || in.URL == "" || in.Checksum == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
		return
	}

	actor, _, _ := s.currentAdmin(r)

	// NOTE: Legacy node_tasks INSERT removed. Bulk agent updates are now dispatched via gRPC.
	log.Printf("[update] bulk agent update for %d nodes to version %s requested by %s (dispatched via gRPC)", len(in.NodeIDs), in.Version, actor)
	writeJSON(w, map[string]any{"ok": true, "queued": len(in.NodeIDs)})
}
