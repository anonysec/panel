package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handlePortalConnections handles GET /api/portal/connections.
// Returns active VPN sessions for the authenticated customer across all assigned nodes,
// plus usage/quota info for the current billing period.
func (s *Server) handlePortalConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	// Query active sessions from radacct for this customer
	rows, err := s.DB.Query(`
		SELECT
			COALESCE(r.calledstationid, 'openvpn') AS protocol,
			COALESCE(n.name, r.nasipaddress) AS node_name,
			r.framedipaddress AS assigned_ip,
			COALESCE(EXTRACT(EPOCH FROM (NOW() - r.acctstarttime))::INT, 0) AS duration,
			COALESCE(r.acctinputoctets, 0) AS rx_bytes,
			COALESCE(r.acctoutputoctets, 0) AS tx_bytes
		FROM radacct r
		LEFT JOIN nodes n ON n.public_ip = r.nasipaddress OR n.domain = r.nasipaddress
		WHERE r.username = $1 AND r.acctstoptime IS NULL
		ORDER BY r.acctstarttime DESC
	`, username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type connection struct {
		Protocol   string `json:"protocol"`
		NodeName   string `json:"node_name"`
		AssignedIP string `json:"assigned_ip"`
		Duration   int64  `json:"duration"`
		RxBytes    int64  `json:"rx_bytes"`
		TxBytes    int64  `json:"tx_bytes"`
	}

	connections := []connection{}
	for rows.Next() {
		var c connection
		if err := rows.Scan(&c.Protocol, &c.NodeName, &c.AssignedIP, &c.Duration, &c.RxBytes, &c.TxBytes); err != nil {
			continue
		}
		// Normalize protocol from calledstationid
		c.Protocol = normalizeProtocol(c.Protocol)
		connections = append(connections, c)
	}

	// Query usage quota from subscription
	var usedBytes int64
	_ = s.DB.QueryRow(`
		SELECT COALESCE(SUM(COALESCE(acctinputoctets,0) + COALESCE(acctoutputoctets,0)), 0)
		FROM radacct
		WHERE username = $1
	`, username).Scan(&usedBytes)

	// Get data limit from radcheck (Max-Data attribute)
	var limitBytes int64
	var maxDataStr string
	if err := s.DB.QueryRow(`SELECT value FROM radcheck WHERE username=$1 AND attribute='Max-Data' ORDER BY id DESC LIMIT 1`, username).Scan(&maxDataStr); err == nil {
		limitBytes, _ = strconv.ParseInt(strings.TrimSpace(maxDataStr), 10, 64)
	}

	// Get billing period from subscription
	var periodStart, periodEnd string
	var subStart, subEnd *time.Time
	err = s.DB.QueryRow(`
		SELECT started_at, expires_at
		FROM subscriptions
		WHERE customer_id = (SELECT id FROM customers WHERE username = $1 AND deleted_at IS NULL LIMIT 1)
		AND status = 'active'
		ORDER BY id DESC LIMIT 1
	`, username).Scan(&subStart, &subEnd)
	if err == nil && subStart != nil {
		periodStart = subStart.UTC().Format(time.RFC3339)
	}
	if err == nil && subEnd != nil {
		periodEnd = subEnd.UTC().Format(time.RFC3339)
	}

	writeJSON(w, map[string]any{
		"ok":          true,
		"connections": connections,
		"usage": map[string]any{
			"used_bytes":   usedBytes,
			"limit_bytes":  limitBytes,
			"period_start": periodStart,
			"period_end":   periodEnd,
		},
	})
}

// normalizeProtocol normalizes the calledstationid value to a friendly protocol name.
func normalizeProtocol(raw string) string {
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "openvpn"):
		return "openvpn"
	case strings.Contains(lower, "wireguard") || strings.Contains(lower, "wg"):
		return "wireguard"
	case strings.Contains(lower, "l2tp"):
		return "l2tp"
	case strings.Contains(lower, "ikev2") || strings.Contains(lower, "ike"):
		return "ikev2"
	case strings.Contains(lower, "ssh"):
		return "ssh"
	case strings.Contains(lower, "mtproto"):
		return "mtproto"
	case strings.Contains(lower, "xray"):
		return "xray"
	default:
		// If it looks like an IP address or empty, default to openvpn
		if raw == "" || strings.Contains(raw, ".") || strings.Contains(raw, ":") {
			return "openvpn"
		}
		return lower
	}
}
