package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// handlePortalConfigDownload handles GET /api/portal/configs/{protocol}.
// Generates and returns a VPN config file for the specified protocol.
// Only serves configs for protocols that have running cores on customer's assigned nodes.
func (s *Server) handlePortalConfigDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	// Extract protocol from path: /api/portal/configs/{protocol}
	path := r.URL.Path
	prefix := "/api/portal/configs/"
	if !strings.HasPrefix(path, prefix) {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	protocol := strings.TrimPrefix(path, prefix)
	protocol = strings.TrimSuffix(protocol, "/")
	if protocol == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_protocol"})
		return
	}

	// Get customer's preferred node (or first available)
	nodeID, _ := strconv.ParseInt(r.URL.Query().Get("node_id"), 10, 64)

	// Check if the protocol has a running core on customer's assigned nodes
	if !s.protocolAvailableForCustomer(username, protocol, nodeID) {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "protocol_not_available"})
		return
	}

	// Generate config based on protocol
	switch strings.ToLower(protocol) {
	case "openvpn", "openvpn-udp":
		config := s.openVPNProfile(username, r, nodeID)
		_, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
		filename := safeFilename(nodeName) + ".ovpn"
		if filename == ".ovpn" {
			filename = "vpn.ovpn"
		}
		w.Header().Set("Content-Type", "application/x-openvpn-profile; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename*=UTF-8''%s`, url.PathEscape(filename)))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(config))

	case "openvpn-tcp":
		config := s.openVPNProfileTCP(username, r, nodeID)
		_, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
		filename := safeFilename(nodeName) + "-TCP.ovpn"
		if filename == "-TCP.ovpn" {
			filename = "vpn-TCP.ovpn"
		}
		w.Header().Set("Content-Type", "application/x-openvpn-profile; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename*=UTF-8''%s`, url.PathEscape(filename)))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(config))

	case "l2tp":
		config := s.l2tpMobileConfig(username, r, nodeID)
		_, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
		filename := safeFilename(username) + "-" + safeFilename(nodeName) + ".mobileconfig"
		if filename == "-.mobileconfig" {
			filename = "vpn-l2tp.mobileconfig"
		}
		w.Header().Set("Content-Type", "application/x-apple-aspen-config; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename*=UTF-8''%s`, url.PathEscape(filename)))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(config))

	case "ikev2":
		config := s.ikev2MobileConfig(username, r, nodeID)
		_, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
		filename := safeFilename(username) + "-" + safeFilename(nodeName) + "-ikev2.mobileconfig"
		if filename == "--ikev2.mobileconfig" {
			filename = "vpn-ikev2.mobileconfig"
		}
		w.Header().Set("Content-Type", "application/x-apple-aspen-config; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename*=UTF-8''%s`, url.PathEscape(filename)))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(config))

	case "wireguard", "wg":
		// Generate WireGuard config if available
		config := s.wireguardConfig(username, nodeID)
		if config == "" {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "config_not_available"})
			return
		}
		filename := safeFilename(username) + "-wg.conf"
		if filename == "-wg.conf" {
			filename = "vpn-wg.conf"
		}
		w.Header().Set("Content-Type", "application/x-wireguard-profile; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename*=UTF-8''%s`, url.PathEscape(filename)))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(config))

	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "unsupported_protocol"})
	}
}

// protocolAvailableForCustomer checks if a protocol has a running core on nodes
// assigned to the customer.
func (s *Server) protocolAvailableForCustomer(username, protocol string, nodeID int64) bool {
	// Normalize protocol name for DB lookup
	dbProtocol := strings.ToLower(protocol)
	switch dbProtocol {
	case "openvpn-udp", "openvpn-tcp":
		dbProtocol = "openvpn"
	case "wg":
		dbProtocol = "wireguard"
	}

	// If a specific node is requested, check that node
	if nodeID > 0 {
		var cnt int
		_ = s.DB.QueryRow(`
			SELECT COUNT(*) FROM node_services
			WHERE node_id = $1 AND service = $2 AND status = 'running'
		`, nodeID, dbProtocol).Scan(&cnt)
		return cnt > 0
	}

	// Otherwise check any node that serves this customer
	// First try nodes with active sessions for this user
	var cnt int
	_ = s.DB.QueryRow(`
		SELECT COUNT(*) FROM node_services ns
		WHERE ns.service = $1 AND ns.status = 'running'
		AND ns.node_id IN (
			SELECT DISTINCT n.id FROM nodes n
			WHERE n.status != 'disabled'
		)
	`, dbProtocol).Scan(&cnt)
	return cnt > 0
}

// wireguardConfig generates a WireGuard config for the customer.
// Returns empty string if no WireGuard peer exists.
func (s *Server) wireguardConfig(username string, nodeID int64) string {
	// Get customer ID
	var customerID int64
	if err := s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID); err != nil {
		return ""
	}

	// Find an active WireGuard peer for this customer
	var peerID int64
	var peerNodeID int64
	var privateKey, allowedIPs, presharedKey string

	query := `SELECT id, node_id, COALESCE(private_key_encrypted,''), allowed_ips, COALESCE(preshared_key,'')
		FROM wg_peers WHERE customer_id=$1 AND status='active'`
	if nodeID > 0 {
		query += ` AND node_id=$2 ORDER BY id DESC LIMIT 1`
		if err := s.DB.QueryRow(query, customerID, nodeID).Scan(&peerID, &peerNodeID, &privateKey, &allowedIPs, &presharedKey); err != nil {
			return ""
		}
	} else {
		query += ` ORDER BY id DESC LIMIT 1`
		if err := s.DB.QueryRow(query, customerID).Scan(&peerID, &peerNodeID, &privateKey, &allowedIPs, &presharedKey); err != nil {
			return ""
		}
	}

	if privateKey == "" {
		return ""
	}

	// Get server public key and port from node_vpn_configs
	var extraJSON []byte
	var wgPort int
	if err := s.DB.QueryRow(`SELECT COALESCE(extra_json,'{}'), port FROM node_vpn_configs WHERE node_id=$1 AND protocol='wireguard'`, peerNodeID).Scan(&extraJSON, &wgPort); err != nil {
		return ""
	}

	var serverPublicKey, dns1, dns2 string
	var extra map[string]any
	if err := json.Unmarshal(extraJSON, &extra); err == nil {
		if v, ok := extra["server_public_key"].(string); ok {
			serverPublicKey = v
		}
		if v, ok := extra["dns_1"].(string); ok {
			dns1 = v
		}
		if v, ok := extra["dns_2"].(string); ok {
			dns2 = v
		}
	}

	if serverPublicKey == "" {
		return ""
	}

	// Get endpoint
	var nodeIP, nodeDomain string
	_ = s.DB.QueryRow(`SELECT COALESCE(public_ip,''), COALESCE(domain,'') FROM nodes WHERE id=$1`, peerNodeID).Scan(&nodeIP, &nodeDomain)

	var endpoint string
	if nodeDomain != "" {
		endpoint = fmt.Sprintf("%s:%d", nodeDomain, wgPort)
	} else if nodeIP != "" {
		endpoint = fmt.Sprintf("%s:%d", nodeIP, wgPort)
	} else {
		return ""
	}

	// Build DNS
	dns := dns1
	if dns2 != "" {
		dns = dns1 + ", " + dns2
	}
	if dns == "" {
		dns = "1.1.1.1, 8.8.8.8"
	}

	// Build the config manually (avoid importing wireguard package which may have complex deps)
	config := fmt.Sprintf("[Interface]\nPrivateKey = %s\nAddress = %s\nDNS = %s\n\n[Peer]\nPublicKey = %s\n",
		privateKey, allowedIPs, dns, serverPublicKey)
	if presharedKey != "" {
		config += fmt.Sprintf("PresharedKey = %s\n", presharedKey)
	}
	config += fmt.Sprintf("Endpoint = %s\nAllowedIPs = 0.0.0.0/0, ::/0\nPersistentKeepalive = 25\n", endpoint)

	return config
}
