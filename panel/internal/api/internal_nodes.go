package api

import (
	"KorisPanel/panel/internal/grpcclient"
	"net/http"
	"time"
)

// internalNodes returns the node list for CLI consumption.
// It does not require authentication since it is only exposed on the
// Unix socket or localhost internal listener.
// GET /internal/nodes
func (s *Server) internalNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	rows, err := s.DB.Query(`SELECT id, name, public_ip, status, COALESCE(last_seen_at, created_at) FROM nodes ORDER BY id`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type nodeEntry struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		IP          string  `json:"ip"`
		HealthScore float64 `json:"health_score"`
		Status      string  `json:"status"`
		LastSeen    string  `json:"last_seen"`
	}

	nodes := []nodeEntry{}
	for rows.Next() {
		var n nodeEntry
		var lastSeen time.Time
		if err := rows.Scan(&n.ID, &n.Name, &n.IP, &n.Status, &lastSeen); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "scan_error"})
			return
		}
		n.LastSeen = lastSeen.UTC().Format(time.RFC3339)
		// Populate health score from gRPC pool status
		if s.GRPCPool != nil {
			switch s.GRPCPool.Status(n.ID) {
			case grpcclient.StatusOnline:
				n.HealthScore = 1.0
			case grpcclient.StatusStale:
				n.HealthScore = 0.5
			default:
				n.HealthScore = 0.0
			}
		}
		nodes = append(nodes, n)
	}

	writeJSON(w, map[string]any{
		"ok":    true,
		"nodes": nodes,
	})
}
