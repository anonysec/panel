package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) getCustomerUsage(w http.ResponseWriter, id int64) {
	username, err := s.customerUsername(id)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	usage, err := s.usageForUsername(username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "usage": usage})
}

func (s *Server) portalUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	usage, err := s.usageForUsername(username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "usage": usage})
}

func (s *Server) usageForUsername(username string) (UsageSummary, error) {
	usage := UsageSummary{Sessions: []UsageSession{}}
	var lastConnected, lastDisconnected sql.NullTime
	if err := s.DB.QueryRow(`SELECT COALESCE(SUM(COALESCE(acctinputoctets,0)),0),COALESCE(SUM(COALESCE(acctoutputoctets,0)),0),COALESCE(SUM(CASE WHEN acctstoptime IS NULL THEN 1 ELSE 0 END),0),MAX(acctstarttime),MAX(acctstoptime) FROM radacct WHERE username=$1`, username).Scan(&usage.TotalInputBytes, &usage.TotalOutputBytes, &usage.ActiveSessions, &lastConnected, &lastDisconnected); err != nil {
		return usage, err
	}
	usage.TotalUsageBytes = usage.TotalInputBytes + usage.TotalOutputBytes
	usage.Online = usage.ActiveSessions > 0
	if lastConnected.Valid {
		usage.LastConnectedAt = lastConnected.Time.Format(time.RFC3339)
	}
	if lastDisconnected.Valid {
		usage.LastDisconnectedAt = lastDisconnected.Time.Format(time.RFC3339)
	}
	var maxData string
	if err := s.DB.QueryRow(`SELECT value FROM radcheck WHERE username=$1 AND attribute='Max-Data' ORDER BY id DESC LIMIT 1`, username).Scan(&maxData); err == nil {
		usage.MaxDataBytes, _ = strconv.ParseInt(strings.TrimSpace(maxData), 10, 64)
	}
	if usage.MaxDataBytes > 0 {
		remaining := usage.MaxDataBytes - usage.TotalUsageBytes
		if remaining < 0 {
			remaining = 0
		}
		usage.RemainingBytes = &remaining
	}
	rows, err := s.DB.Query(`SELECT radacctid,username,acctstarttime,acctupdatetime,acctstoptime,COALESCE(acctsessiontime,EXTRACT(EPOCH FROM (COALESCE(acctstoptime,NOW())-acctstarttime))::INT,0),COALESCE(acctinputoctets,0),COALESCE(acctoutputoctets,0),framedipaddress,callingstationid,acctterminatecause FROM radacct WHERE username=$1 ORDER BY radacctid DESC LIMIT 50`, username)
	if err != nil {
		return usage, err
	}
	defer rows.Close()
	for rows.Next() {
		var session UsageSession
		var start, update, stop sql.NullTime
		var seconds sql.NullInt64
		if err := rows.Scan(&session.ID, &session.Username, &start, &update, &stop, &seconds, &session.InputBytes, &session.OutputBytes, &session.FramedIP, &session.CallingStationID, &session.TerminateCause); err != nil {
			return usage, err
		}
		if start.Valid {
			session.StartTime = start.Time.Format(time.RFC3339)
		}
		if update.Valid {
			session.UpdateTime = update.Time.Format(time.RFC3339)
		}
		if stop.Valid {
			session.StopTime = stop.Time.Format(time.RFC3339)
		}
		if seconds.Valid {
			session.SessionSeconds = seconds.Int64
		}
		session.TotalBytes = session.InputBytes + session.OutputBytes
		session.Online = !stop.Valid
		usage.Sessions = append(usage.Sessions, session)
	}
	return usage, rows.Err()
}

func (s *Server) portalNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	_, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	rows, err := s.DB.Query(`SELECT id,name,COALESCE(domain,''),public_ip,status FROM nodes WHERE status <> 'disabled' ORDER BY CASE status WHEN 'online' THEN 0 WHEN 'stale' THEN 1 ELSE 2 END, id ASC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	type NodeInfo struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Domain   string `json:"domain"`
		PublicIP string `json:"public_ip"`
		Status   string `json:"status"`
	}
	out := []NodeInfo{}
	for rows.Next() {
		var n NodeInfo
		if err := rows.Scan(&n.ID, &n.Name, &n.Domain, &n.PublicIP, &n.Status); err == nil {
			out = append(out, n)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "nodes": out})
}

func (s *Server) openVPNEndpointNode(r *http.Request, nodeID int64) (host string, port int, proto string, nodeName string) {
	port = 1194
	proto = "udp"
	_ = s.DB.QueryRow(`SELECT openvpn_port,openvpn_protocol FROM vpn_core_settings WHERE id=1`).Scan(&port, &proto)

	// Priority 0: Global VPN domain (static config — same domain for all nodes, DNS-based failover)
	var globalVPNDomain string
	_ = s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key='vpn_domain'`).Scan(&globalVPNDomain)
	globalVPNDomain = strings.TrimSpace(globalVPNDomain)
	if globalVPNDomain != "" {
		host = globalVPNDomain
	}

	if nodeID > 0 {
		// Get node name regardless
		var domain, publicIP string
		_ = s.DB.QueryRow(`SELECT name,COALESCE(domain,''),public_ip FROM nodes WHERE id=$1 AND status <> 'disabled' LIMIT 1`, nodeID).Scan(&nodeName, &domain, &publicIP)

		if host == "" {
			// Priority 1: Check for active failover domain pointing to this node
			var failoverDomain string
			if err := s.DB.QueryRow(
				`SELECT domain FROM failover_domains WHERE current_node_id = $1 AND is_active = TRUE LIMIT 1`, nodeID,
			).Scan(&failoverDomain); err == nil && strings.TrimSpace(failoverDomain) != "" {
				host = strings.TrimSpace(failoverDomain)
			}
		}

		if host == "" {
			// Priority 2 & 3: Node's domain field, then public_ip
			var domain2, publicIP2 string
			_ = s.DB.QueryRow(`SELECT COALESCE(domain,''),public_ip FROM nodes WHERE id=$1 LIMIT 1`, nodeID).Scan(&domain2, &publicIP2)
			host = strings.TrimSpace(domain2)
			if host == "" {
				host = strings.TrimSpace(publicIP2)
			}
		}
	}
	if host == "" {
		var domain, publicIP string
		_ = s.DB.QueryRow(`SELECT name,COALESCE(domain,''),public_ip FROM nodes WHERE status <> 'disabled' ORDER BY CASE status WHEN 'online' THEN 0 WHEN 'stale' THEN 1 ELSE 2 END, id ASC LIMIT 1`).Scan(&nodeName, &domain, &publicIP)
		host = strings.TrimSpace(domain)
		if host == "" {
			host = strings.TrimSpace(publicIP)
		}
	}
	if host == "" {
		host = r.Host
		if strings.Contains(host, ":") {
			host = strings.Split(host, ":")[0]
		}
	}
	if proto == "" {
		proto = "udp"
	}
	if port <= 0 {
		port = 1194
	}
	return host, port, proto, nodeName
}

func (s *Server) portalProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	nodeID, _ := strconv.ParseInt(r.URL.Query().Get("node_id"), 10, 64)
	host, port, _, nodeName := s.openVPNEndpointNode(r, nodeID)
	var psk string
	_ = s.DB.QueryRow(`SELECT COALESCE(ipsec_psk,'') FROM vpn_core_settings WHERE id=1`).Scan(&psk)
	psk = strings.TrimSpace(psk)
	passwordlessAvailable := s.canUsePasswordless(username)

	// Resolve auth_mode for this customer: check radcheck Auth-Mode attribute first,
	// then fall back to node's OpenVPN extra_json config, default to "userpass".
	authMode := s.resolveAuthMode(username, nodeID)

	// Build filenames:
	// - OpenVPN with auth: generic config, use node name only (e.g. "🇩🇪Germany.ovpn")
	// - Passwordless / mobileconfig: per-user, use "username-nodename" (e.g. "john-🇩🇪Germany.ovpn")
	nodeBase := safeFilename(nodeName)
	if nodeBase == "" {
		nodeBase = "vpn"
	}
	userBase := safeFilename(username)
	genericFilenameUDP := nodeBase + ".ovpn"
	genericFilenameTCP := nodeBase + "-TCP.ovpn"
	perUserOvpn := userBase + "-" + nodeBase + ".ovpn"
	perUserL2TP := userBase + "-" + nodeBase + ".mobileconfig"
	perUserIKEv2 := userBase + "-" + nodeBase + "-ikev2.mobileconfig"

	// Check if Cisco IPSec is enabled for this node
	var ciscoEnabled bool
	if nodeID > 0 {
		var cnt int
		_ = s.DB.QueryRow(`SELECT COUNT(*) FROM node_vpn_configs WHERE node_id=$1 AND protocol='cisco_ipsec' AND enabled=TRUE`, nodeID).Scan(&cnt)
		ciscoEnabled = cnt > 0
	} else {
		// If no specific node, check if any node has it enabled
		var cnt int
		_ = s.DB.QueryRow(`SELECT COUNT(*) FROM node_vpn_configs WHERE protocol='cisco_ipsec' AND enabled=TRUE LIMIT 1`).Scan(&cnt)
		ciscoEnabled = cnt > 0
	}

	// Get user's preferred node
	var preferredNodeID int64
	_ = s.DB.QueryRow(`SELECT COALESCE(preferred_node_id, 0) FROM customers WHERE username=$1 AND deleted_at IS NULL`, username).Scan(&preferredNodeID)

	writeJSON(w, map[string]any{
		"ok":                     true,
		"passwordless_available": passwordlessAvailable,
		"preferred_node_id":      preferredNodeID,
		"profiles": []map[string]any{
			{
				"type":                  "openvpn-udp",
				"name":                  "OpenVPN UDP — " + nodeName,
				"filename":              genericFilenameUDP,
				"filename_passwordless": perUserOvpn,
				"available":             host != "",
				"remote":                host,
				"port":                  port,
				"protocol":              "udp",
				"node":                  nodeName,
				"download":              fmt.Sprintf("/api/portal/profiles/openvpn.ovpn?node_id=%d", nodeID),
				"description":           "Fast, best for gaming. Direct connection with failover.",
				"auth_mode":             authMode,
			},
			{
				"type":        "openvpn-tcp",
				"name":        "OpenVPN TCP — " + nodeName,
				"filename":    genericFilenameTCP,
				"available":   host != "",
				"remote":      host,
				"port":        443,
				"protocol":    "tcp",
				"node":        nodeName,
				"download":    fmt.Sprintf("/api/portal/profiles/openvpn-tcp.ovpn?node_id=%d", nodeID),
				"description": "Stable, supports node selection. Works behind firewalls.",
				"auth_mode":   authMode,
			},
			{
				"type":      "l2tp",
				"name":      "L2TP/IPSec — " + nodeName,
				"filename":  perUserL2TP,
				"available": host != "" && psk != "",
				"remote":    host,
				"port":      1701,
				"protocol":  "l2tp",
				"node":      nodeName,
				"download":  fmt.Sprintf("/api/portal/profiles/l2tp.mobileconfig?node_id=%d", nodeID),
				"auth_mode": authMode,
			},
			{
				"type":      "ikev2",
				"name":      "IKEv2 — " + nodeName,
				"filename":  perUserIKEv2,
				"available": host != "",
				"remote":    host,
				"port":      500,
				"protocol":  "ikev2",
				"node":      nodeName,
				"download":  fmt.Sprintf("/api/portal/profiles/ikev2.mobileconfig?node_id=%d", nodeID),
				"auth_mode": authMode,
			},
			{
				"type":        "cisco-ipsec",
				"name":        "Cisco IPSec — " + nodeName,
				"filename":    userBase + "-" + nodeBase + "-cisco.mobileconfig",
				"available":   ciscoEnabled && host != "",
				"remote":      host,
				"port":        500,
				"protocol":    "ipsec",
				"node":        nodeName,
				"download":    "/api/customer/configs/cisco-ipsec",
				"description": "IKEv1 + XAUTH. For iOS, macOS, and Android (strongSwan).",
				"auth_mode":   authMode,
			},
		},
	})
}

// resolveAuthMode determines the OpenVPN authentication mode for a customer.
// It checks (in order):
//  1. radcheck attribute "Auth-Mode" for the customer's username
//  2. The node's OpenVPN extra_json config (node_vpn_configs)
//  3. Defaults to "userpass"
func (s *Server) resolveAuthMode(username string, nodeID int64) string {
	// 1. Check radcheck for per-user Auth-Mode attribute
	var mode string
	err := s.DB.QueryRow(
		`SELECT value FROM radcheck WHERE username=$1 AND attribute='Auth-Mode' ORDER BY id DESC LIMIT 1`,
		username,
	).Scan(&mode)
	if err == nil {
		mode = strings.TrimSpace(mode)
		if mode == "certificate" || mode == "userpass" {
			return mode
		}
	}

	// 2. Check the node's OpenVPN config extra_json
	if nodeID > 0 {
		var nodeMode sql.NullString
		_ = s.DB.QueryRow(
			`SELECT extra_json->>'auth_mode' FROM node_vpn_configs WHERE node_id=$1 AND protocol='openvpn' AND enabled=TRUE LIMIT 1`,
			nodeID,
		).Scan(&nodeMode)
		if nodeMode.Valid {
			m := strings.TrimSpace(nodeMode.String)
			if m == "certificate" || m == "userpass" {
				return m
			}
		}
	}

	// 3. Default
	return "userpass"
}
