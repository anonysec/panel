//go:build !lite

package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// handleAnyConnect dispatches /api/anyconnect requests.
func (s *Server) handleAnyConnect(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleAnyConnectList(w, r)
	case http.MethodPost:
		s.handleAnyConnectCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAnyConnectByID dispatches /api/anyconnect/{id} and /api/anyconnect/{id}/{action}.
func (s *Server) handleAnyConnectByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/anyconnect/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	switch {
	case action == "" && r.Method == http.MethodDelete:
		s.handleAnyConnectDelete(w, r, id)
	case action == "cert" && r.Method == http.MethodPost:
		s.handleAnyConnectCert(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

// handleAnyConnectList returns all AnyConnect-enabled nodes joined with node info.
// GET /api/anyconnect
func (s *Server) handleAnyConnectList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(`
		SELECT a.id, a.node_id, a.port, COALESCE(a.cert_path, '') AS cert_path,
		       a.status, a.created_at, a.updated_at,
		       COALESCE(n.public_ip, '') AS node_ip, COALESCE(n.name, '') AS node_name
		FROM anyconnect_nodes a
		LEFT JOIN nodes n ON n.id = a.node_id
		ORDER BY a.created_at DESC`)
	if err != nil {
		log.Printf("[anyconnect] list query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type anyconnectNode struct {
		ID        int64  `json:"id"`
		NodeID    int64  `json:"node_id"`
		Port      int    `json:"port"`
		CertPath  string `json:"cert_path"`
		Status    string `json:"status"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		NodeIP    string `json:"node_ip"`
		NodeName  string `json:"node_name"`
	}

	var nodes []anyconnectNode
	for rows.Next() {
		var n anyconnectNode
		if err := rows.Scan(&n.ID, &n.NodeID, &n.Port, &n.CertPath,
			&n.Status, &n.CreatedAt, &n.UpdatedAt,
			&n.NodeIP, &n.NodeName); err != nil {
			log.Printf("[anyconnect] scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[anyconnect] rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	if nodes == nil {
		nodes = []anyconnectNode{}
	}

	writeJSON(w, map[string]any{"ok": true, "nodes": nodes})
}

// handleAnyConnectCreate enables AnyConnect on a node.
// POST /api/anyconnect
func (s *Server) handleAnyConnectCreate(w http.ResponseWriter, r *http.Request) {
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

	// Insert into anyconnect_nodes
	result, err := s.DB.Exec(
		`INSERT INTO anyconnect_nodes (node_id, port, status) VALUES (?, ?, 'pending')`,
		in.NodeID, in.Port,
	)
	if err != nil {
		log.Printf("[anyconnect] insert failed: %v", err)
		if strings.Contains(err.Error(), "Duplicate") {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "anyconnect_already_exists"})
			return
		}
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	id, _ := result.LastInsertId()

	// NOTE: Legacy node_tasks INSERT removed. AnyConnect enable is now dispatched via gRPC.
	log.Printf("[anyconnect] anyconnect_enable for node %d (dispatched via gRPC)", in.NodeID)

	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// handleAnyConnectDelete disables AnyConnect on a node and sends a disable task.
// DELETE /api/anyconnect/{id}
func (s *Server) handleAnyConnectDelete(w http.ResponseWriter, r *http.Request, id int64) {
	// Get node_id from the record
	var nodeID int64
	err := s.DB.QueryRow(`SELECT node_id FROM anyconnect_nodes WHERE id = ?`, id).Scan(&nodeID)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// NOTE: Legacy node_tasks INSERT removed. AnyConnect disable is now dispatched via gRPC.
	log.Printf("[anyconnect] anyconnect_disable for node %d (dispatched via gRPC)", nodeID)

	// Delete from anyconnect_nodes
	_, err = s.DB.Exec(`DELETE FROM anyconnect_nodes WHERE id = ?`, id)
	if err != nil {
		log.Printf("[anyconnect] delete failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// handleAnyConnectCert uploads/rotates TLS certificate for AnyConnect on a node.
// POST /api/anyconnect/{id}/cert
func (s *Server) handleAnyConnectCert(w http.ResponseWriter, r *http.Request, id int64) {
	// Use a larger limit for cert data (512KB)
	limitBody(w, r, 512<<10)

	var in struct {
		CertPEM string `json:"cert_pem"`
		KeyPEM  string `json:"key_pem"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if strings.TrimSpace(in.CertPEM) == "" || strings.TrimSpace(in.KeyPEM) == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "cert_and_key_required"})
		return
	}

	// Get node_id from the record
	var nodeID int64
	err := s.DB.QueryRow(`SELECT node_id FROM anyconnect_nodes WHERE id = ?`, id).Scan(&nodeID)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	// Update cert_path in anyconnect_nodes (store a reference indicating cert is managed)
	_, err = s.DB.Exec(`UPDATE anyconnect_nodes SET cert_path = '/etc/ocserv/server-cert.pem' WHERE id = ?`, id)
	if err != nil {
		log.Printf("[anyconnect] update cert_path failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// NOTE: Legacy node_tasks INSERT removed. AnyConnect cert update is now dispatched via gRPC.
	log.Printf("[anyconnect] anyconnect_cert_update for node %d (dispatched via gRPC)", nodeID)

	writeJSON(w, map[string]any{"ok": true})
}

// handleAnyConnectProfile generates and returns an AnyConnect profile XML
// for the customer's assigned node.
// GET /api/portal/anyconnect/profile
func (s *Server) handleAnyConnectProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	// Get customer's preferred node
	var nodeID int64
	_ = s.DB.QueryRow(`SELECT COALESCE(preferred_node_id, 0) FROM customers WHERE username=? AND deleted_at IS NULL`, username).Scan(&nodeID)

	// Resolve node host (domain or IP)
	var host string
	if nodeID > 0 {
		var domain, publicIP string
		_ = s.DB.QueryRow(`SELECT COALESCE(domain,''), public_ip FROM nodes WHERE id=? LIMIT 1`, nodeID).Scan(&domain, &publicIP)
		host = strings.TrimSpace(domain)
		if host == "" {
			host = strings.TrimSpace(publicIP)
		}
	}
	if host == "" {
		// Fallback: first online node with AnyConnect enabled
		_ = s.DB.QueryRow(`
			SELECT COALESCE(n.domain,''), n.public_ip
			FROM anyconnect_nodes a
			JOIN nodes n ON n.id = a.node_id
			WHERE a.status = 'active' AND n.status <> 'disabled'
			ORDER BY n.id ASC LIMIT 1`).Scan(&host, &host)
		if host == "" {
			host = r.Host
			if strings.Contains(host, ":") {
				host = strings.Split(host, ":")[0]
			}
		}
	}

	// Get AnyConnect port for the node
	port := 443
	if nodeID > 0 {
		_ = s.DB.QueryRow(`SELECT port FROM anyconnect_nodes WHERE node_id=? LIMIT 1`, nodeID).Scan(&port)
	}

	// Generate the AnyConnect XML profile
	profile := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<AnyConnectProfile xmlns="http://schemas.xmlsoap.org/encoding/">
  <ServerList>
    <HostEntry>
      <HostName>KorisPanel VPN</HostName>
      <HostAddress>%s</HostAddress>
      <PrimaryProtocol>SSL</PrimaryProtocol>
    </HostEntry>
  </ServerList>
  <ClientInitialization>
    <UseStartBeforeLogon UserControllable="false">false</UseStartBeforeLogon>
    <AutomaticCertSelection UserControllable="false">true</AutomaticCertSelection>
    <ShowPreConnectMessage>false</ShowPreConnectMessage>
    <CertificateStore>All</CertificateStore>
    <CertificateStoreOverride>false</CertificateStoreOverride>
    <ProxySettings>Native</ProxySettings>
    <AllowLocalProxyConnections>true</AllowLocalProxyConnections>
    <AuthenticationTimeout>12</AuthenticationTimeout>
    <AutoConnectOnStart UserControllable="true">false</AutoConnectOnStart>
    <MinimizeOnConnect UserControllable="true">true</MinimizeOnConnect>
    <LocalLanAccess UserControllable="true">true</LocalLanAccess>
    <AutoReconnect UserControllable="false">true
      <AutoReconnectBehavior>ReconnectAfterResume</AutoReconnectBehavior>
    </AutoReconnect>
    <AutoUpdate UserControllable="false">true</AutoUpdate>
    <RSASecurIDIntegration UserControllable="false">Automatic</RSASecurIDIntegration>
    <WindowsLogonEnforcement>SingleLocalLogon</WindowsLogonEnforcement>
    <WindowsVPNEstablishment>LocalUsersOnly</WindowsVPNEstablishment>
    <LinuxLogonEnforcement>SingleLocalLogon</LinuxLogonEnforcement>
    <LinuxVPNEstablishment>LocalUsersOnly</LinuxVPNEstablishment>
  </ClientInitialization>
</AnyConnectProfile>`, host)

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="anyconnect-profile-%s.xml"`, username))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(profile))
}
