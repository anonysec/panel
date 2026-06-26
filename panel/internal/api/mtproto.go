//go:build !lite

package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// handleMTProto dispatches /api/mtproto requests.
func (s *Server) handleMTProto(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleMTProtoList(w, r)
	case http.MethodPost:
		s.handleMTProtoCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleMTProtoByID dispatches /api/mtproto/{id} and /api/mtproto/{id}/{action}.
func (s *Server) handleMTProtoByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/mtproto/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	switch {
	case action == "" && r.Method == http.MethodDelete:
		s.handleMTProtoDelete(w, r, id)
	case action == "rotate" && r.Method == http.MethodPost:
		s.handleMTProtoRotate(w, r, id)
	case action == "link" && r.Method == http.MethodGet:
		s.handleMTProtoLink(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

// handleMTProtoList returns all MTProto proxies joined with nodes for IP.
// GET /api/mtproto
func (s *Server) handleMTProtoList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(`
		SELECT m.id, m.node_id, m.port, m.secret, m.status, m.connections,
		       m.rx_bytes, m.tx_bytes, m.created_at, m.updated_at,
		       COALESCE(n.public_ip, '') AS node_ip, COALESCE(n.name, '') AS node_name
		FROM mtproto_proxies m
		LEFT JOIN nodes n ON n.id = m.node_id
		ORDER BY m.created_at DESC`)
	if err != nil {
		log.Printf("[mtproto] list query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type mtprotoProxy struct {
		ID          int64  `json:"id"`
		NodeID      int64  `json:"node_id"`
		Port        int    `json:"port"`
		Secret      string `json:"secret"`
		Status      string `json:"status"`
		Connections int    `json:"connections"`
		RxBytes     int64  `json:"rx_bytes"`
		TxBytes     int64  `json:"tx_bytes"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
		NodeIP      string `json:"node_ip"`
		NodeName    string `json:"node_name"`
	}

	var proxies []mtprotoProxy
	for rows.Next() {
		var p mtprotoProxy
		if err := rows.Scan(&p.ID, &p.NodeID, &p.Port, &p.Secret, &p.Status,
			&p.Connections, &p.RxBytes, &p.TxBytes, &p.CreatedAt, &p.UpdatedAt,
			&p.NodeIP, &p.NodeName); err != nil {
			log.Printf("[mtproto] scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		proxies = append(proxies, p)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[mtproto] rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	if proxies == nil {
		proxies = []mtprotoProxy{}
	}

	writeJSON(w, map[string]any{"ok": true, "proxies": proxies})
}

// handleMTProtoCreate creates a new MTProto proxy on a node.
// POST /api/mtproto
func (s *Server) handleMTProtoCreate(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		NodeID int64 `json:"node_id"`
		Port   int   `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.NodeID == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_id_required"})
		return
	}
	if in.Port == 0 {
		in.Port = 443
	}
	if in.Port < 1 || in.Port > 65535 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_port"})
		return
	}

	// Generate random 32-byte hex secret
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		log.Printf("[mtproto] failed to generate secret: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "internal_error"})
		return
	}
	secret := hex.EncodeToString(secretBytes)

	// Insert into mtproto_proxies
	result, err := s.DB.Exec(
		`INSERT INTO mtproto_proxies (node_id, port, secret, status) VALUES ($1, $2, $3, 'pending')`,
		in.NodeID, in.Port, secret,
	)
	if err != nil {
		log.Printf("[mtproto] insert failed: %v", err)
		if strings.Contains(err.Error(), "Duplicate") {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "proxy_already_exists"})
			return
		}
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	id, _ := result.LastInsertId()

	// Enable MTProto core on node via gRPC
	if s.CoreMgr != nil {
		extraConfig, _ := json.Marshal(map[string]any{
			"port":   in.Port,
			"secret": secret,
		})
		if err := s.CoreMgr.EnableCore(r.Context(), in.NodeID, "mtproto", in.Port, extraConfig); err != nil {
			log.Printf("[knode] EnableCore (mtproto) failed for node %d: %v", in.NodeID, err)
			writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		// Update mtproto_proxies status to active on success
		_, _ = s.DB.Exec(`UPDATE mtproto_proxies SET status = 'active' WHERE id = $1`, id)
	} else {
		log.Printf("[knode] gRPC not configured, cannot enable mtproto on node %d", in.NodeID)
	}

	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// handleMTProtoDelete removes an MTProto proxy and sends a disable task.
// DELETE /api/mtproto/{id}
func (s *Server) handleMTProtoDelete(w http.ResponseWriter, r *http.Request, id int64) {
	// Get node_id from the proxy record
	var nodeID int64
	err := s.DB.QueryRow(`SELECT node_id FROM mtproto_proxies WHERE id = $1`, id).Scan(&nodeID)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// Disable MTProto core on node via gRPC
	if s.CoreMgr != nil {
		if err := s.CoreMgr.DisableCore(r.Context(), nodeID, "mtproto"); err != nil {
			log.Printf("[knode] DisableCore (mtproto) failed for node %d: %v", nodeID, err)
			// Continue with deletion even if gRPC fails — the record will be cleaned up
		}
	} else {
		log.Printf("[knode] gRPC not configured, cannot disable mtproto on node %d", nodeID)
	}

	// Delete from mtproto_proxies
	_, err = s.DB.Exec(`DELETE FROM mtproto_proxies WHERE id = $1`, id)
	if err != nil {
		log.Printf("[mtproto] delete failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// handleMTProtoRotate generates a new secret for a proxy and pushes a rotate task.
// POST /api/mtproto/{id}/rotate
func (s *Server) handleMTProtoRotate(w http.ResponseWriter, r *http.Request, id int64) {
	// Verify proxy exists and get node_id
	var nodeID int64
	err := s.DB.QueryRow(`SELECT node_id FROM mtproto_proxies WHERE id = $1`, id).Scan(&nodeID)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// Generate new random secret
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		log.Printf("[mtproto] failed to generate secret: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "internal_error"})
		return
	}
	newSecret := hex.EncodeToString(secretBytes)

	// Update secret in database
	_, err = s.DB.Exec(`UPDATE mtproto_proxies SET secret = $1 WHERE id = $2`, newSecret, id)
	if err != nil {
		log.Printf("[mtproto] update secret failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// Push updated secret to node via gRPC EnableCore (reconfigure)
	if s.CoreMgr != nil {
		// Get port for the reconfigure
		var port int
		_ = s.DB.QueryRow(`SELECT port FROM mtproto_proxies WHERE id = $1`, id).Scan(&port)

		extraConfig, _ := json.Marshal(map[string]any{
			"secret": newSecret,
		})
		if err := s.CoreMgr.EnableCore(r.Context(), nodeID, "mtproto", port, extraConfig); err != nil {
			log.Printf("[knode] EnableCore (mtproto rotate) failed for node %d: %v", nodeID, err)
			// Non-fatal: secret is already updated in DB
		}
	} else {
		log.Printf("[knode] gRPC not configured, cannot rotate mtproto secret on node %d", nodeID)
	}

	writeJSON(w, map[string]any{"ok": true, "secret": newSecret})
}

// handleMTProtoLink returns the tg://proxy share link for a proxy.
// GET /api/mtproto/{id}/link
func (s *Server) handleMTProtoLink(w http.ResponseWriter, r *http.Request, id int64) {
	var nodeIP, secret string
	var port int
	err := s.DB.QueryRow(`
		SELECT COALESCE(n.public_ip, ''), m.port, m.secret
		FROM mtproto_proxies m
		JOIN nodes n ON n.id = m.node_id
		WHERE m.id = $1`, id).Scan(&nodeIP, &port, &secret)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	link := fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s", nodeIP, port, secret)
	writeJSON(w, map[string]any{"ok": true, "link": link})
}
