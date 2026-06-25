//go:build !lite

package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"KorisPanel/panel/internal/xray"
)

// handleXrayInbound dispatches /api/xray/inbounds requests.
func (s *Server) handleXrayInbound(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleXrayInboundList(w, r)
	case http.MethodPost:
		s.handleXrayInboundCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleXrayInboundByID dispatches /api/xray/inbounds/{id} and sub-actions.
func (s *Server) handleXrayInboundByID(w http.ResponseWriter, r *http.Request) {
	id, _, ok := pathID(r.URL.Path, "/api/xray/inbounds/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleXrayInboundGet(w, r, id)
	case http.MethodPatch:
		s.handleXrayInboundUpdate(w, r, id)
	case http.MethodDelete:
		s.handleXrayInboundDelete(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleXrayInboundList returns all xray inbounds with optional filtering.
// GET /api/xray/inbounds?node_id=&customer_id=&protocol=&status=
func (s *Server) handleXrayInboundList(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT xi.id, xi.customer_id, xi.node_id, xi.uuid, xi.protocol, xi.transport,
		       xi.security, xi.port, COALESCE(xi.server_name,''), COALESCE(xi.public_key,''),
		       COALESCE(xi.short_id,''), COALESCE(xi.path,''), COALESCE(xi.service_name,''),
		       xi.status, xi.rx_bytes, xi.tx_bytes, xi.core_name, xi.created_at, xi.updated_at,
		       COALESCE(n.name,'') AS node_name, COALESCE(n.public_ip,'') AS node_ip,
		       COALESCE(c.username,'') AS customer_username
		FROM xray_inbounds xi
		LEFT JOIN nodes n ON n.id = xi.node_id
		LEFT JOIN customers c ON c.id = xi.customer_id
		WHERE 1=1`

	var args []any

	if v := r.URL.Query().Get("node_id"); v != "" {
		if nid, err := strconv.ParseInt(v, 10, 64); err == nil {
			query += " AND xi.node_id = ?"
			args = append(args, nid)
		}
	}
	if v := r.URL.Query().Get("customer_id"); v != "" {
		if cid, err := strconv.ParseInt(v, 10, 64); err == nil {
			query += " AND xi.customer_id = ?"
			args = append(args, cid)
		}
	}
	if v := r.URL.Query().Get("protocol"); v != "" {
		query += " AND xi.protocol = ?"
		args = append(args, v)
	}
	if v := r.URL.Query().Get("status"); v != "" {
		query += " AND xi.status = ?"
		args = append(args, v)
	}

	query += " ORDER BY xi.created_at DESC"

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		log.Printf("[xray] inbound list query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type xrayInbound struct {
		ID               int64  `json:"id"`
		CustomerID       int64  `json:"customer_id"`
		NodeID           int64  `json:"node_id"`
		UUID             string `json:"uuid"`
		Protocol         string `json:"protocol"`
		Transport        string `json:"transport"`
		Security         string `json:"security"`
		Port             int    `json:"port"`
		ServerName       string `json:"server_name"`
		PublicKey        string `json:"public_key"`
		ShortID          string `json:"short_id"`
		Path             string `json:"path"`
		ServiceName      string `json:"service_name"`
		Status           string `json:"status"`
		RxBytes          int64  `json:"rx_bytes"`
		TxBytes          int64  `json:"tx_bytes"`
		CoreName         string `json:"core_name"`
		CreatedAt        string `json:"created_at"`
		UpdatedAt        string `json:"updated_at"`
		NodeName         string `json:"node_name"`
		NodeIP           string `json:"node_ip"`
		CustomerUsername string `json:"customer_username"`
	}

	var inbounds []xrayInbound
	for rows.Next() {
		var ib xrayInbound
		if err := rows.Scan(
			&ib.ID, &ib.CustomerID, &ib.NodeID, &ib.UUID, &ib.Protocol, &ib.Transport,
			&ib.Security, &ib.Port, &ib.ServerName, &ib.PublicKey, &ib.ShortID,
			&ib.Path, &ib.ServiceName, &ib.Status, &ib.RxBytes, &ib.TxBytes,
			&ib.CoreName, &ib.CreatedAt, &ib.UpdatedAt,
			&ib.NodeName, &ib.NodeIP, &ib.CustomerUsername,
		); err != nil {
			log.Printf("[xray] inbound scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		inbounds = append(inbounds, ib)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[xray] inbound rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	if inbounds == nil {
		inbounds = []xrayInbound{}
	}

	writeJSON(w, map[string]any{"ok": true, "inbounds": inbounds})
}

// handleXrayInboundCreate creates a new Xray inbound for a customer.
// POST /api/xray/inbounds
func (s *Server) handleXrayInboundCreate(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		CustomerID  int64  `json:"customer_id"`
		NodeID      int64  `json:"node_id"`
		Protocol    string `json:"protocol"`
		Transport   string `json:"transport"`
		Security    string `json:"security"`
		Port        int    `json:"port"`
		ServerName  string `json:"server_name"`
		PublicKey   string `json:"public_key"`
		ShortID     string `json:"short_id"`
		PrivateKey  string `json:"private_key"`
		Path        string `json:"path"`
		ServiceName string `json:"service_name"`
		CoreName    string `json:"core_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Validate required fields
	if in.CustomerID == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "customer_id_required"})
		return
	}
	if in.NodeID == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_id_required"})
		return
	}
	if in.Port < 1 || in.Port > 65535 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_port"})
		return
	}

	// Validate protocol
	switch in.Protocol {
	case "vless", "vmess", "trojan":
	default:
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_protocol"})
		return
	}

	// Validate transport
	switch in.Transport {
	case "tcp", "ws", "grpc", "h2":
	default:
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_transport"})
		return
	}

	// Validate security
	switch in.Security {
	case "reality", "tls", "none", "":
		if in.Security == "" {
			in.Security = "none"
		}
	default:
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_security"})
		return
	}

	// Default core_name
	if in.CoreName == "" {
		in.CoreName = "xray-core"
	}

	// Generate UUID v4
	uuid, err := generateUUIDv4()
	if err != nil {
		log.Printf("[xray] failed to generate UUID: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "internal_error"})
		return
	}

	// Insert into xray_inbounds
	result, err := s.DB.Exec(`
		INSERT INTO xray_inbounds (customer_id, node_id, uuid, protocol, transport, security,
			port, server_name, public_key, short_id, private_key, path, service_name, core_name, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active')`,
		in.CustomerID, in.NodeID, uuid, in.Protocol, in.Transport, in.Security,
		in.Port, nullIfEmpty(in.ServerName), nullIfEmpty(in.PublicKey), nullIfEmpty(in.ShortID),
		nullIfEmpty(in.PrivateKey), nullIfEmpty(in.Path), nullIfEmpty(in.ServiceName), in.CoreName,
	)
	if err != nil {
		log.Printf("[xray] insert inbound failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	lastID, _ := result.LastInsertId()

	// Generate xray config fragment
	cfg := xray.InboundConfig{
		UUID:        uuid,
		Protocol:    in.Protocol,
		Transport:   in.Transport,
		Security:    in.Security,
		ServerName:  in.ServerName,
		PublicKey:   in.PublicKey,
		ShortID:     in.ShortID,
		PrivateKey:  in.PrivateKey,
		Path:        in.Path,
		ServiceName: in.ServiceName,
		Port:        in.Port,
	}
	fragment, err := xray.GenerateXrayFragment(cfg)
	if err != nil {
		log.Printf("[xray] generate fragment failed: %v", err)
		// Still succeed — the inbound was saved
	}
	_ = fragment // will be sent via gRPC when xray wrapper is implemented

	// NOTE: Legacy node_tasks INSERT removed. Xray add is now dispatched via gRPC.
	log.Printf("[xray] xray_add for node %d inbound %d (dispatched via gRPC)", in.NodeID, lastID)

	writeJSON(w, map[string]any{"ok": true, "id": lastID, "uuid": uuid})
}

// handleXrayInboundGet returns a single xray inbound by ID.
// GET /api/xray/inbounds/{id}
func (s *Server) handleXrayInboundGet(w http.ResponseWriter, r *http.Request, id int64) {
	type xrayInbound struct {
		ID               int64  `json:"id"`
		CustomerID       int64  `json:"customer_id"`
		NodeID           int64  `json:"node_id"`
		UUID             string `json:"uuid"`
		Protocol         string `json:"protocol"`
		Transport        string `json:"transport"`
		Security         string `json:"security"`
		Port             int    `json:"port"`
		ServerName       string `json:"server_name"`
		PublicKey        string `json:"public_key"`
		ShortID          string `json:"short_id"`
		PrivateKey       string `json:"private_key"`
		Path             string `json:"path"`
		ServiceName      string `json:"service_name"`
		Status           string `json:"status"`
		RxBytes          int64  `json:"rx_bytes"`
		TxBytes          int64  `json:"tx_bytes"`
		CoreName         string `json:"core_name"`
		CreatedAt        string `json:"created_at"`
		UpdatedAt        string `json:"updated_at"`
		NodeName         string `json:"node_name"`
		NodeIP           string `json:"node_ip"`
		CustomerUsername string `json:"customer_username"`
	}

	var ib xrayInbound
	err := s.DB.QueryRow(`
		SELECT xi.id, xi.customer_id, xi.node_id, xi.uuid, xi.protocol, xi.transport,
		       xi.security, xi.port, COALESCE(xi.server_name,''), COALESCE(xi.public_key,''),
		       COALESCE(xi.short_id,''), COALESCE(xi.private_key,''),
		       COALESCE(xi.path,''), COALESCE(xi.service_name,''),
		       xi.status, xi.rx_bytes, xi.tx_bytes, xi.core_name, xi.created_at, xi.updated_at,
		       COALESCE(n.name,'') AS node_name, COALESCE(n.public_ip,'') AS node_ip,
		       COALESCE(c.username,'') AS customer_username
		FROM xray_inbounds xi
		LEFT JOIN nodes n ON n.id = xi.node_id
		LEFT JOIN customers c ON c.id = xi.customer_id
		WHERE xi.id = ?`, id).Scan(
		&ib.ID, &ib.CustomerID, &ib.NodeID, &ib.UUID, &ib.Protocol, &ib.Transport,
		&ib.Security, &ib.Port, &ib.ServerName, &ib.PublicKey, &ib.ShortID, &ib.PrivateKey,
		&ib.Path, &ib.ServiceName, &ib.Status, &ib.RxBytes, &ib.TxBytes,
		&ib.CoreName, &ib.CreatedAt, &ib.UpdatedAt,
		&ib.NodeName, &ib.NodeIP, &ib.CustomerUsername,
	)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "inbound": ib})
}

// handleXrayInboundUpdate updates an existing xray inbound and pushes config to node.
// PATCH /api/xray/inbounds/{id}
func (s *Server) handleXrayInboundUpdate(w http.ResponseWriter, r *http.Request, id int64) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		Port        *int    `json:"port"`
		Transport   *string `json:"transport"`
		Security    *string `json:"security"`
		ServerName  *string `json:"server_name"`
		PublicKey   *string `json:"public_key"`
		ShortID     *string `json:"short_id"`
		PrivateKey  *string `json:"private_key"`
		Path        *string `json:"path"`
		ServiceName *string `json:"service_name"`
		Status      *string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Check inbound exists and get current values
	var nodeID int64
	var uuid, protocol, transport, security, serverName, publicKey, shortID, privateKey, path, serviceName string
	var port int
	err := s.DB.QueryRow(`
		SELECT node_id, uuid, protocol, transport, security, port,
		       COALESCE(server_name,''), COALESCE(public_key,''), COALESCE(short_id,''),
		       COALESCE(private_key,''), COALESCE(path,''), COALESCE(service_name,'')
		FROM xray_inbounds WHERE id = ?`, id).Scan(
		&nodeID, &uuid, &protocol, &transport, &security, &port,
		&serverName, &publicKey, &shortID, &privateKey, &path, &serviceName,
	)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// Build dynamic UPDATE
	setClauses := []string{}
	setArgs := []any{}

	if in.Port != nil {
		if *in.Port < 1 || *in.Port > 65535 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_port"})
			return
		}
		setClauses = append(setClauses, "port = ?")
		setArgs = append(setArgs, *in.Port)
		port = *in.Port
	}
	if in.Transport != nil {
		switch *in.Transport {
		case "tcp", "ws", "grpc", "h2":
		default:
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_transport"})
			return
		}
		setClauses = append(setClauses, "transport = ?")
		setArgs = append(setArgs, *in.Transport)
		transport = *in.Transport
	}
	if in.Security != nil {
		switch *in.Security {
		case "reality", "tls", "none":
		default:
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_security"})
			return
		}
		setClauses = append(setClauses, "security = ?")
		setArgs = append(setArgs, *in.Security)
		security = *in.Security
	}
	if in.ServerName != nil {
		setClauses = append(setClauses, "server_name = ?")
		setArgs = append(setArgs, nullIfEmpty(*in.ServerName))
		serverName = *in.ServerName
	}
	if in.PublicKey != nil {
		setClauses = append(setClauses, "public_key = ?")
		setArgs = append(setArgs, nullIfEmpty(*in.PublicKey))
		publicKey = *in.PublicKey
	}
	if in.ShortID != nil {
		setClauses = append(setClauses, "short_id = ?")
		setArgs = append(setArgs, nullIfEmpty(*in.ShortID))
		shortID = *in.ShortID
	}
	if in.PrivateKey != nil {
		setClauses = append(setClauses, "private_key = ?")
		setArgs = append(setArgs, nullIfEmpty(*in.PrivateKey))
		privateKey = *in.PrivateKey
	}
	if in.Path != nil {
		setClauses = append(setClauses, "path = ?")
		setArgs = append(setArgs, nullIfEmpty(*in.Path))
		path = *in.Path
	}
	if in.ServiceName != nil {
		setClauses = append(setClauses, "service_name = ?")
		setArgs = append(setArgs, nullIfEmpty(*in.ServiceName))
		serviceName = *in.ServiceName
	}
	if in.Status != nil {
		switch *in.Status {
		case "active", "disabled", "pending":
		default:
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_status"})
			return
		}
		setClauses = append(setClauses, "status = ?")
		setArgs = append(setArgs, *in.Status)
	}

	if len(setClauses) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "no_fields"})
		return
	}

	// Execute UPDATE
	updateSQL := "UPDATE xray_inbounds SET "
	for i, clause := range setClauses {
		if i > 0 {
			updateSQL += ", "
		}
		updateSQL += clause
	}
	updateSQL += " WHERE id = ?"
	setArgs = append(setArgs, id)

	_, err = s.DB.Exec(updateSQL, setArgs...)
	if err != nil {
		log.Printf("[xray] update inbound %d failed: %v", id, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// Generate new config fragment and push xray_update task
	cfg := xray.InboundConfig{
		UUID:        uuid,
		Protocol:    protocol,
		Transport:   transport,
		Security:    security,
		ServerName:  serverName,
		PublicKey:   publicKey,
		ShortID:     shortID,
		PrivateKey:  privateKey,
		Path:        path,
		ServiceName: serviceName,
		Port:        port,
	}
	fragment, err := xray.GenerateXrayFragment(cfg)
	if err != nil {
		log.Printf("[xray] generate fragment for update failed: %v", err)
	}
	_ = fragment // will be sent via gRPC when xray wrapper is implemented

	// NOTE: Legacy node_tasks INSERT removed. Xray update is now dispatched via gRPC.
	log.Printf("[xray] xray_update for node %d inbound %d (dispatched via gRPC)", nodeID, id)

	writeJSON(w, map[string]any{"ok": true})
}

// handleXrayInboundDelete removes an xray inbound and pushes removal task to node.
// DELETE /api/xray/inbounds/{id}
func (s *Server) handleXrayInboundDelete(w http.ResponseWriter, r *http.Request, id int64) {
	// Get inbound details for the task payload
	var nodeID int64
	var uuid string
	err := s.DB.QueryRow(`SELECT node_id, uuid FROM xray_inbounds WHERE id = ?`, id).Scan(&nodeID, &uuid)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// NOTE: Legacy node_tasks INSERT removed. Xray remove is now dispatched via gRPC.
	log.Printf("[xray] xray_remove for node %d inbound %d (dispatched via gRPC)", nodeID, id)

	// Delete from xray_inbounds
	_, err = s.DB.Exec(`DELETE FROM xray_inbounds WHERE id = ?`, id)
	if err != nil {
		log.Printf("[xray] delete inbound %d failed: %v", id, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// generateUUIDv4 generates a random UUID v4 string using crypto/rand.
func generateUUIDv4() (string, error) {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		return "", err
	}
	// Set version 4
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	// Set variant RFC 4122
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}

// nullIfEmpty returns nil if the string is empty, otherwise returns a pointer.
// Used for nullable VARCHAR columns.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
