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
	rows, err := s.DB.Query(`SELECT id, name, address, status FROM knode_connections WHERE enabled=TRUE ORDER BY CASE status WHEN 'online' THEN 0 WHEN 'stale' THEN 1 ELSE 2 END, id ASC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	type NodeInfo struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		Address string `json:"address"`
		Status  string `json:"status"`
	}
	out := []NodeInfo{}
	for rows.Next() {
		var n NodeInfo
		if err := rows.Scan(&n.ID, &n.Name, &n.Address, &n.Status); err == nil {
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
		// Get node name and address from knode_connections
		var address string
		_ = s.DB.QueryRow(`SELECT name, address FROM knode_connections WHERE id=$1 AND enabled=TRUE LIMIT 1`, nodeID).Scan(&nodeName, &address)

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
			// Priority 2: Node's address (IP)
			host = strings.TrimSpace(address)
		}
	}
	if host == "" {
		// Fallback: pick any online enabled node from knode_connections
		var address string
		_ = s.DB.QueryRow(`SELECT name, address FROM knode_connections WHERE enabled=TRUE ORDER BY CASE status WHEN 'online' THEN 0 WHEN 'stale' THEN 1 ELSE 2 END, id ASC LIMIT 1`).Scan(&nodeName, &address)
		host = strings.TrimSpace(address)
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
	passwordlessAvailable := s.canUsePasswordless(username)

	// Resolve auth_mode for this customer: check radcheck Auth-Mode attribute first,
	// then fall back to node's OpenVPN extra_json config, default to "userpass".
	authMode := s.resolveAuthMode(username, nodeID)

	// Build filenames
	nodeBase := safeFilename(nodeName)
	if nodeBase == "" {
		nodeBase = "vpn"
	}
	userBase := safeFilename(username)
	perUserOvpn := userBase + "-" + nodeBase + ".ovpn"
	perUserL2TP := userBase + "-" + nodeBase + ".mobileconfig"
	perUserIKEv2 := userBase + "-" + nodeBase + "-ikev2.mobileconfig"

	// Query which protocols are actually enabled on the target node
	enabledProtocols := map[string]bool{}
	var protocolQuery string
	var protocolArgs []any
	if nodeID > 0 {
		protocolQuery = `SELECT protocol FROM node_vpn_configs WHERE node_id=$1 AND enabled=TRUE`
		protocolArgs = []any{nodeID}
	} else {
		// No specific node — find the resolved node's ID from knode_connections
		var resolvedNodeID int64
		_ = s.DB.QueryRow(`SELECT id FROM knode_connections WHERE address=$1 AND enabled=TRUE LIMIT 1`, host).Scan(&resolvedNodeID)
		if resolvedNodeID > 0 {
			protocolQuery = `SELECT protocol FROM node_vpn_configs WHERE node_id=$1 AND enabled=TRUE`
			protocolArgs = []any{resolvedNodeID}
		} else {
			// Fallback: use first enabled node
			protocolQuery = `SELECT DISTINCT protocol FROM node_vpn_configs WHERE enabled=TRUE`
		}
	}
	if protocolQuery != "" {
		rows, err := s.DB.Query(protocolQuery, protocolArgs...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var p string
				if rows.Scan(&p) == nil {
					enabledProtocols[p] = true
				}
			}
		}
	}

	// Get user's preferred node
	var preferredNodeID int64
	_ = s.DB.QueryRow(`SELECT COALESCE(preferred_node_id, 0) FROM customers WHERE username=$1 AND deleted_at IS NULL`, username).Scan(&preferredNodeID)

	// Build dynamic profiles list based on enabled protocols
	profiles := []map[string]any{}

	// OpenVPN UDP
	if enabledProtocols["openvpn"] {
		profiles = append(profiles, map[string]any{
			"type":                  "openvpn-udp",
			"name":                  "OpenVPN UDP — " + nodeName,
			"filename":              nodeBase + ".ovpn",
			"filename_passwordless": perUserOvpn,
			"available":             true,
			"remote":                host,
			"port":                  port,
			"protocol":              "udp",
			"node":                  nodeName,
			"download":              fmt.Sprintf("/api/portal/profiles/openvpn.ovpn?node_id=%d", nodeID),
			"description":           "Fast, best for gaming. Direct connection with failover.",
			"auth_mode":             authMode,
		})

		// Passwordless OpenVPN UDP (when auth_mode is certificate)
		if authMode == "certificate" || passwordlessAvailable {
			profiles = append(profiles, map[string]any{
				"type":        "openvpn-udp-passwordless",
				"name":        "OpenVPN UDP (Passwordless) — " + nodeName,
				"filename":    perUserOvpn,
				"available":   true,
				"remote":      host,
				"port":        port,
				"protocol":    "udp",
				"node":        nodeName,
				"download":    fmt.Sprintf("/api/portal/profiles/openvpn-passwordless.ovpn?node_id=%d", nodeID),
				"description": "Certificate-based, no password needed.",
				"auth_mode":   "certificate",
			})
		}
	}

	// OpenVPN TCP — only if explicitly enabled as a separate protocol
	if enabledProtocols["openvpn-tcp"] {
		profiles = append(profiles, map[string]any{
			"type":        "openvpn-tcp",
			"name":        "OpenVPN TCP — " + nodeName,
			"filename":    nodeBase + "-TCP.ovpn",
			"available":   true,
			"remote":      host,
			"port":        443,
			"protocol":    "tcp",
			"node":        nodeName,
			"download":    fmt.Sprintf("/api/portal/profiles/openvpn-tcp.ovpn?node_id=%d", nodeID),
			"description": "Stable, supports node selection. Works behind firewalls.",
			"auth_mode":   authMode,
		})
	}

	// L2TP/IPSec
	if enabledProtocols["l2tp"] {
		profiles = append(profiles, map[string]any{
			"type":      "l2tp",
			"name":      "L2TP/IPSec — " + nodeName,
			"filename":  perUserL2TP,
			"available": true,
			"remote":    host,
			"port":      1701,
			"protocol":  "l2tp",
			"node":      nodeName,
			"download":  fmt.Sprintf("/api/portal/profiles/l2tp.mobileconfig?node_id=%d", nodeID),
			"auth_mode": authMode,
		})
	}

	// IKEv2
	if enabledProtocols["ikev2"] {
		profiles = append(profiles, map[string]any{
			"type":      "ikev2",
			"name":      "IKEv2 — " + nodeName,
			"filename":  perUserIKEv2,
			"available": true,
			"remote":    host,
			"port":      500,
			"protocol":  "ikev2",
			"node":      nodeName,
			"download":  fmt.Sprintf("/api/portal/profiles/ikev2.mobileconfig?node_id=%d", nodeID),
			"auth_mode": authMode,
		})
	}

	// SSH Tunnel
	if enabledProtocols["ssh"] {
		profiles = append(profiles, map[string]any{
			"type":        "ssh",
			"name":        "SSH Tunnel — " + nodeName,
			"filename":    userBase + "-" + nodeBase + "-ssh.json",
			"available":   true,
			"remote":      host,
			"port":        22,
			"protocol":    "ssh",
			"node":        nodeName,
			"download":    fmt.Sprintf("/api/portal/profiles/ssh.json?node_id=%d", nodeID),
			"description": "SSH-based tunnel. Works in restricted networks.",
			"auth_mode":   authMode,
		})
	}

	// WireGuard
	if enabledProtocols["wireguard"] {
		profiles = append(profiles, map[string]any{
			"type":        "wireguard",
			"name":        "WireGuard — " + nodeName,
			"filename":    userBase + "-" + nodeBase + "-wg.conf",
			"available":   true,
			"remote":      host,
			"port":        51820,
			"protocol":    "wireguard",
			"node":        nodeName,
			"download":    fmt.Sprintf("/api/portal/profiles/wireguard.conf?node_id=%d", nodeID),
			"description": "Modern, fast, lightweight VPN protocol.",
			"auth_mode":   authMode,
		})
	}

	// MTProto Proxy
	if enabledProtocols["mtproto"] {
		profiles = append(profiles, map[string]any{
			"type":        "mtproto",
			"name":        "MTProto Proxy — " + nodeName,
			"filename":    userBase + "-" + nodeBase + "-mtproto.json",
			"available":   true,
			"remote":      host,
			"port":        443,
			"protocol":    "mtproto",
			"node":        nodeName,
			"download":    fmt.Sprintf("/api/portal/profiles/mtproto.json?node_id=%d", nodeID),
			"description": "Telegram-optimized proxy protocol.",
			"auth_mode":   authMode,
		})
	}

	// Cisco IPSec
	if enabledProtocols["cisco_ipsec"] {
		profiles = append(profiles, map[string]any{
			"type":        "cisco-ipsec",
			"name":        "Cisco IPSec — " + nodeName,
			"filename":    userBase + "-" + nodeBase + "-cisco.mobileconfig",
			"available":   true,
			"remote":      host,
			"port":        500,
			"protocol":    "ipsec",
			"node":        nodeName,
			"download":    "/api/customer/configs/cisco-ipsec",
			"description": "IKEv1 + XAUTH. For iOS, macOS, and Android (strongSwan).",
			"auth_mode":   authMode,
		})
	}

	writeJSON(w, map[string]any{
		"ok":                     true,
		"passwordless_available": passwordlessAvailable || authMode == "certificate",
		"preferred_node_id":      preferredNodeID,
		"profiles":               profiles,
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
