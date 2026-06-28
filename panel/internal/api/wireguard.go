package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"KorisPanel/panel/internal/wireguard"

	qrcode "github.com/skip2/go-qrcode"
)

// WgPeer represents a WireGuard peer record from the database.
type WgPeer struct {
	ID                  int64  `json:"id"`
	CustomerID          *int64 `json:"customer_id"`
	NodeID              int64  `json:"node_id"`
	NodeName            string `json:"node_name,omitempty"`
	PublicKey           string `json:"public_key"`
	PresharedKey        string `json:"preshared_key,omitempty"`
	PrivateKeyEncrypted string `json:"private_key_encrypted,omitempty"`
	AllowedIPs          string `json:"allowed_ips"`
	Endpoint            string `json:"endpoint"`
	Status              string `json:"status"`
	LastHandshakeAt     string `json:"last_handshake_at,omitempty"`
	RxBytes             int64  `json:"rx_bytes"`
	TxBytes             int64  `json:"tx_bytes"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
	Username            string `json:"username,omitempty"`
}

func (s *Server) wireguardPeers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listWireguardPeers(w, r)
	case http.MethodPost:
		s.createWireguardPeer(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listWireguardPeers(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT p.id, p.customer_id, p.node_id, p.public_key, p.allowed_ips,
		       COALESCE(p.endpoint,''), p.status, p.rx_bytes, p.tx_bytes,
		       p.created_at, p.updated_at, COALESCE(c.username,''), COALESCE(n.name,'')
		FROM wg_peers p
		LEFT JOIN customers c ON c.id = p.customer_id
		LEFT JOIN nodes n ON n.id = p.node_id`
	args := []any{}

	// Filter by customer_id if provided
	if cidStr := r.URL.Query().Get("customer_id"); cidStr != "" {
		if cid, err := strconv.ParseInt(cidStr, 10, 64); err == nil && cid > 0 {
			query += ` WHERE p.customer_id = $1`
			args = append(args, cid)
		}
	}
	query += ` ORDER BY p.id DESC LIMIT 500`

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	peers := []WgPeer{}
	for rows.Next() {
		var p WgPeer
		var customerID *int64
		var created, updated time.Time
		var nodeName string
		if err := rows.Scan(&p.ID, &customerID, &p.NodeID, &p.PublicKey, &p.AllowedIPs,
			&p.Endpoint, &p.Status, &p.RxBytes, &p.TxBytes,
			&created, &updated, &p.Username, &nodeName); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		p.CustomerID = customerID
		p.CreatedAt = created.Format(time.RFC3339)
		p.UpdatedAt = updated.Format(time.RFC3339)
		p.NodeName = nodeName
		peers = append(peers, p)
	}
	if err := rows.Err(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "peers": peers})
}

func (s *Server) createWireguardPeer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CustomerID int64  `json:"customer_id"`
		NodeID     int64  `json:"node_id"`
		AllowedIPs string `json:"allowed_ips"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.NodeID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_id_required"})
		return
	}

	// Auto-allocate IP if allowed_ips not provided
	if in.AllowedIPs == "" {
		// Fetch the node's WireGuard network CIDR and extra_json for dual-stack
		var networkCIDR string
		var cfgExtraJSON []byte
		err := s.DB.QueryRow(`SELECT network, COALESCE(extra_json,'{}') FROM node_vpn_configs WHERE node_id=$1 AND protocol='wireguard' LIMIT 1`, in.NodeID).Scan(&networkCIDR, &cfgExtraJSON)
		if err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "wireguard_config_not_found_for_node"})
			return
		}

		// Check for IPv6 network in extra_json for dual-stack support
		var networkIPv6 string
		var cfgExtra map[string]any
		if err := json.Unmarshal(cfgExtraJSON, &cfgExtra); err == nil {
			if v, ok := cfgExtra["network_ipv6"].(string); ok && v != "" {
				networkIPv6 = v
			}
		}

		// Query active peer IPs for the node
		rows, err := s.DB.Query(`SELECT allowed_ips FROM wg_peers WHERE node_id=$1 AND status='active'`, in.NodeID)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer rows.Close()

		var usedIPs []string
		for rows.Next() {
			var allowedIPs string
			if err := rows.Scan(&allowedIPs); err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			// Extract the IP without prefix length (e.g., "10.66.66.2/24" -> "10.66.66.2")
			for _, seg := range strings.Split(allowedIPs, ",") {
				seg = strings.TrimSpace(seg)
				if ip, _, err := net.ParseCIDR(seg); err == nil {
					usedIPs = append(usedIPs, ip.String())
				} else if parsed := net.ParseIP(seg); parsed != nil {
					usedIPs = append(usedIPs, parsed.String())
				}
			}
		}
		if err := rows.Err(); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}

		// Allocate next available IPv4 address
		allocatedIP, err := wireguard.AllocateNextIP(networkCIDR, usedIPs)
		if err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "ip_pool_exhausted"})
			return
		}

		// Format with prefix length from the network CIDR
		formattedIP, err := wireguard.FormatWithPrefix(allocatedIP, networkCIDR)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "ip_format_failed"})
			return
		}

		// Dual-stack: also allocate from IPv6 pool if configured
		if networkIPv6 != "" {
			allocatedIPv6, err := wireguard.AllocateNextIP(networkIPv6, usedIPs)
			if err != nil {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "ipv6_pool_exhausted"})
				return
			}
			formattedIPv6, err := wireguard.FormatWithPrefix(allocatedIPv6, networkIPv6)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "ipv6_format_failed"})
				return
			}
			// Combine IPv4 and IPv6 as comma-separated dual-stack address
			formattedIP = formattedIP + ", " + formattedIPv6
		}

		in.AllowedIPs = formattedIP
	}

	// Validate each comma-separated CIDR segment in AllowedIPs
	for _, seg := range strings.Split(in.AllowedIPs, ",") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_allowed_ips: empty segment"})
			return
		}
		if _, _, err := net.ParseCIDR(seg); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": fmt.Sprintf("invalid_allowed_ips: %s", err.Error())})
			return
		}
	}

	// Generate WireGuard key pair and preshared key
	privateKey, publicKey, err := wireguard.GenerateKeyPair()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "keygen_failed"})
		return
	}
	presharedKey, err := wireguard.GeneratePresharedKey()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "psk_failed"})
		return
	}

	// Insert peer into database (store private key encrypted - for now store as-is)
	res, err := s.DB.Exec(`
		INSERT INTO wg_peers (customer_id, node_id, public_key, preshared_key, private_key_encrypted, allowed_ips, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'active')`,
		in.CustomerID, in.NodeID, publicKey, presharedKey, privateKey, in.AllowedIPs)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	peerID, _ := res.LastInsertId()

	// Sync user to knode via gRPC (the peer add is communicated via user sync)
	if s.UserSync != nil {
		var username string
		_ = s.DB.QueryRow(`SELECT username FROM customers WHERE id = $1`, in.CustomerID).Scan(&username)
		if username != "" {
			go func() {
				if err := s.UserSync.SyncUser(context.Background(), username); err != nil {
					log.Printf("[knode] SyncUser failed after WireGuard peer add for %q: %v", username, err)
				}
			}()
		}
	}

	writeJSON(w, map[string]any{"ok": true, "id": peerID, "public_key": publicKey})
}

func (s *Server) wireguardPeerByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/wireguard/peers/")
	if !ok {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_id"})
		return
	}

	if action == "config" {
		s.wireguardPeerConfig(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		s.deleteWireguardPeer(w, r, id)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) deleteWireguardPeer(w http.ResponseWriter, r *http.Request, id int64) {
	var nodeID int64
	var publicKey string
	var customerID int64
	err := s.DB.QueryRow(`SELECT node_id, public_key, customer_id FROM wg_peers WHERE id=$1`, id).Scan(&nodeID, &publicKey, &customerID)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "peer_not_found"})
		return
	}

	// Set peer status to revoked
	if _, err := s.DB.Exec(`UPDATE wg_peers SET status='revoked' WHERE id=$1`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Sync user to knode via gRPC (the peer removal is communicated via user sync)
	if s.UserSync != nil {
		var username string
		_ = s.DB.QueryRow(`SELECT username FROM customers WHERE id = $1`, customerID).Scan(&username)
		if username != "" {
			go func() {
				if err := s.UserSync.SyncUser(context.Background(), username); err != nil {
					log.Printf("[knode] SyncUser failed after WireGuard peer delete for %q: %v", username, err)
				}
			}()
		}
	}

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) wireguardPeerConfig(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Look up peer details including the stored private key
	var peer WgPeer
	var customerID *int64
	err := s.DB.QueryRow(`
		SELECT id, customer_id, node_id, public_key, COALESCE(preshared_key,''),
		       COALESCE(private_key_encrypted,''), allowed_ips, COALESCE(endpoint,''), status
		FROM wg_peers WHERE id=$1`, id).Scan(
		&peer.ID, &customerID, &peer.NodeID, &peer.PublicKey,
		&peer.PresharedKey, &peer.PrivateKeyEncrypted, &peer.AllowedIPs,
		&peer.Endpoint, &peer.Status)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "peer_not_found"})
		return
	}
	peer.CustomerID = customerID

	if peer.PrivateKeyEncrypted == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "private_key_not_available"})
		return
	}

	// Get the server's WireGuard public key and endpoint from node_vpn_configs
	var serverPublicKey, serverEndpoint, dns1, dns2 string
	var extraJSON []byte
	err = s.DB.QueryRow(`
		SELECT COALESCE(extra_json,'{}'), port
		FROM node_vpn_configs WHERE node_id=$1 AND protocol='wireguard'`, peer.NodeID).Scan(&extraJSON, new(int))
	if err != nil {
		// Fallback: no WireGuard config found for this node
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "wireguard_config_not_found_for_node"})
		return
	}

	// Parse extra_json for server_public_key, DNS, and gaming_optimize
	var extra map[string]any
	var gamingOptimize bool
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
		if v, ok := extra["gaming_optimize"].(bool); ok {
			gamingOptimize = v
		}
	}

	// Get the node's public IP or domain as the endpoint
	var nodeIP, nodeDomain string
	var wgPort int
	_ = s.DB.QueryRow(`SELECT COALESCE(public_ip,''), COALESCE(domain,'') FROM nodes WHERE id=$1`, peer.NodeID).Scan(&nodeIP, &nodeDomain)
	_ = s.DB.QueryRow(`SELECT port FROM node_vpn_configs WHERE node_id=$1 AND protocol='wireguard'`, peer.NodeID).Scan(&wgPort)

	// Prefer backup_domain from extra_json (failover domain), then node domain, then IP
	if backupDomain, ok := extra["backup_domain"].(string); ok && backupDomain != "" {
		serverEndpoint = fmt.Sprintf("%s:%d", backupDomain, wgPort)
	} else if nodeDomain != "" {
		serverEndpoint = fmt.Sprintf("%s:%d", nodeDomain, wgPort)
	} else if nodeIP != "" {
		serverEndpoint = fmt.Sprintf("%s:%d", nodeIP, wgPort)
	}

	// Build DNS string
	dns := dns1
	if dns2 != "" {
		dns = dns1 + ", " + dns2
	}
	if dns == "" {
		dns = "1.1.1.1, 8.8.8.8"
	}

	// Generate the config
	conf := wireguard.GenerateClientConfig(wireguard.ClientConfig{
		PrivateKey:      peer.PrivateKeyEncrypted,
		Address:         peer.AllowedIPs,
		DNS:             dns,
		ServerPublicKey: serverPublicKey,
		PresharedKey:    peer.PresharedKey,
		Endpoint:        serverEndpoint,
		GamingOptimize:  gamingOptimize,
	})

	// Return as downloadable text file
	filename := fmt.Sprintf("wg-peer-%d.conf", id)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(conf))
}

// --- Portal WireGuard Endpoints ---

// portalWireguardPeers returns the authenticated customer's WireGuard peers.
func (s *Server) portalWireguardPeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	username, _ := s.currentCustomer(r)

	// Get customer ID from username
	var customerID int64
	err := s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	rows, err := s.DB.Query(`
		SELECT p.id, p.node_id, COALESCE(n.name,''), p.status, p.allowed_ips, p.created_at
		FROM wg_peers p
		LEFT JOIN nodes n ON n.id = p.node_id
		WHERE p.customer_id = $1
		ORDER BY p.id DESC`, customerID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type PortalPeer struct {
		ID         int64  `json:"id"`
		NodeID     int64  `json:"node_id"`
		NodeName   string `json:"node_name"`
		Status     string `json:"status"`
		AllowedIPs string `json:"allowed_ips"`
		CreatedAt  string `json:"created_at"`
	}

	peers := []PortalPeer{}
	for rows.Next() {
		var p PortalPeer
		var created time.Time
		if err := rows.Scan(&p.ID, &p.NodeID, &p.NodeName, &p.Status, &p.AllowedIPs, &created); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		p.CreatedAt = created.Format(time.RFC3339)
		peers = append(peers, p)
	}
	if err := rows.Err(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "peers": peers})
}

// portalWireguardPeerByID handles portal peer sub-routes (config download, QR code).
func (s *Server) portalWireguardPeerByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/portal/wireguard/peers/")
	if !ok {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_id"})
		return
	}

	switch action {
	case "config":
		s.portalWireguardPeerConfig(w, r, id)
	case "qr":
		s.portalWireguardPeerQR(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

// portalWireguardPeerConfig serves config download for portal customers.
// Verifies the peer belongs to the authenticated customer.
func (s *Server) portalWireguardPeerConfig(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	username, _ := s.currentCustomer(r)

	// Get customer ID
	var customerID int64
	err := s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	// Fetch peer and verify ownership
	var peer WgPeer
	var peerCustomerID *int64
	err = s.DB.QueryRow(`
		SELECT id, customer_id, node_id, public_key, COALESCE(preshared_key,''),
		       COALESCE(private_key_encrypted,''), allowed_ips, COALESCE(endpoint,''), status
		FROM wg_peers WHERE id=$1`, id).Scan(
		&peer.ID, &peerCustomerID, &peer.NodeID, &peer.PublicKey,
		&peer.PresharedKey, &peer.PrivateKeyEncrypted, &peer.AllowedIPs,
		&peer.Endpoint, &peer.Status)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "peer_not_found"})
		return
	}

	// Verify peer belongs to authenticated customer
	if peerCustomerID == nil || *peerCustomerID != customerID {
		writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
		return
	}

	if peer.PrivateKeyEncrypted == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "private_key_not_available"})
		return
	}

	// Get server config for this peer's node
	var extraJSON []byte
	err = s.DB.QueryRow(`
		SELECT COALESCE(extra_json,'{}')
		FROM node_vpn_configs WHERE node_id=$1 AND protocol='wireguard'`, peer.NodeID).Scan(&extraJSON)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "wireguard_config_not_found_for_node"})
		return
	}

	var serverPublicKey, dns1, dns2, serverEndpoint string
	var gamingOptimize bool
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
		if v, ok := extra["gaming_optimize"].(bool); ok {
			gamingOptimize = v
		}
	}

	// Get node endpoint
	var nodeIP, nodeDomain string
	var wgPort int
	_ = s.DB.QueryRow(`SELECT COALESCE(public_ip,''), COALESCE(domain,'') FROM nodes WHERE id=$1`, peer.NodeID).Scan(&nodeIP, &nodeDomain)
	_ = s.DB.QueryRow(`SELECT port FROM node_vpn_configs WHERE node_id=$1 AND protocol='wireguard'`, peer.NodeID).Scan(&wgPort)

	if nodeDomain != "" {
		serverEndpoint = fmt.Sprintf("%s:%d", nodeDomain, wgPort)
	} else if nodeIP != "" {
		serverEndpoint = fmt.Sprintf("%s:%d", nodeIP, wgPort)
	}

	// Build DNS string
	dns := dns1
	if dns2 != "" {
		dns = dns1 + ", " + dns2
	}
	if dns == "" {
		dns = "1.1.1.1, 8.8.8.8"
	}

	// Generate config
	conf := wireguard.GenerateClientConfig(wireguard.ClientConfig{
		PrivateKey:      peer.PrivateKeyEncrypted,
		Address:         peer.AllowedIPs,
		DNS:             dns,
		ServerPublicKey: serverPublicKey,
		PresharedKey:    peer.PresharedKey,
		Endpoint:        serverEndpoint,
		GamingOptimize:  gamingOptimize,
	})

	// Serve as downloadable .conf file
	var nodeName string
	_ = s.DB.QueryRow(`SELECT COALESCE(name,'') FROM nodes WHERE id=$1`, peer.NodeID).Scan(&nodeName)
	if nodeName == "" {
		nodeName = fmt.Sprintf("node%d", peer.NodeID)
	}
	filename := fmt.Sprintf("KorisVPN-%s.conf", nodeName)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(conf))
}

// autoProvisionWireGuardPeer checks all WireGuard-enabled nodes and creates a peer
// for the customer on each node that doesn't already have one.
// This is called after subscription activation to auto-provision VPN access.
func (s *Server) autoProvisionWireGuardPeer(customerID int64) {
	// Find all WireGuard-enabled nodes
	rows, err := s.DB.Query(`SELECT node_id, network, COALESCE(extra_json,'{}') FROM node_vpn_configs WHERE protocol='wireguard' AND enabled=TRUE`)
	if err != nil {
		return
	}
	defer rows.Close()

	type wgNode struct {
		NodeID      int64
		NetworkCIDR string
		ExtraJSON   []byte
	}
	var nodes []wgNode
	for rows.Next() {
		var n wgNode
		if err := rows.Scan(&n.NodeID, &n.NetworkCIDR, &n.ExtraJSON); err != nil {
			continue
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return
	}

	for _, node := range nodes {
		// Check if customer already has an active peer on this node
		var existing int
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM wg_peers WHERE customer_id=$1 AND node_id=$2 AND status='active'`, customerID, node.NodeID).Scan(&existing); err != nil || existing > 0 {
			continue
		}

		// Check for IPv6 network in extra_json for dual-stack support
		var networkIPv6 string
		var cfgExtra map[string]any
		if err := json.Unmarshal(node.ExtraJSON, &cfgExtra); err == nil {
			if v, ok := cfgExtra["network_ipv6"].(string); ok && v != "" {
				networkIPv6 = v
			}
		}

		// Get used IPs on this node
		ipRows, err := s.DB.Query(`SELECT allowed_ips FROM wg_peers WHERE node_id=$1 AND status='active'`, node.NodeID)
		if err != nil {
			continue
		}
		var usedIPs []string
		for ipRows.Next() {
			var allowedIPs string
			if ipRows.Scan(&allowedIPs) == nil {
				for _, seg := range strings.Split(allowedIPs, ",") {
					seg = strings.TrimSpace(seg)
					if ip, _, err := net.ParseCIDR(seg); err == nil {
						usedIPs = append(usedIPs, ip.String())
					} else if parsed := net.ParseIP(seg); parsed != nil {
						usedIPs = append(usedIPs, parsed.String())
					}
				}
			}
		}
		ipRows.Close()

		// Allocate IP
		allocatedIP, err := wireguard.AllocateNextIP(node.NetworkCIDR, usedIPs)
		if err != nil {
			continue
		}
		formattedIP, err := wireguard.FormatWithPrefix(allocatedIP, node.NetworkCIDR)
		if err != nil {
			continue
		}

		// Dual-stack: also allocate IPv6 if configured
		if networkIPv6 != "" {
			allocatedIPv6, err := wireguard.AllocateNextIP(networkIPv6, usedIPs)
			if err == nil {
				formattedIPv6, err := wireguard.FormatWithPrefix(allocatedIPv6, networkIPv6)
				if err == nil {
					formattedIP = formattedIP + ", " + formattedIPv6
				}
			}
		}

		// Generate keys
		privateKey, publicKey, err := wireguard.GenerateKeyPair()
		if err != nil {
			continue
		}
		presharedKey, err := wireguard.GeneratePresharedKey()
		if err != nil {
			continue
		}

		// Insert peer
		_, err = s.DB.Exec(`INSERT INTO wg_peers (customer_id, node_id, public_key, preshared_key, private_key_encrypted, allowed_ips, status) VALUES ($1, $2, $3, $4, $5, $6, 'active')`,
			customerID, node.NodeID, publicKey, presharedKey, privateKey, formattedIP)
		if err != nil {
			continue
		}

		// Sync user to knode via gRPC after peer creation
		if s.UserSync != nil {
			var username string
			_ = s.DB.QueryRow(`SELECT username FROM customers WHERE id = $1`, customerID).Scan(&username)
			if username != "" {
				go func() {
					if syncErr := s.UserSync.SyncUser(context.Background(), username); syncErr != nil {
						log.Printf("[knode] SyncUser failed after WireGuard auto-provision for %q: %v", username, syncErr)
					}
				}()
			}
		}
	}
}

// autoRevokeWireGuardPeers revokes all active WireGuard peers for a customer
// and triggers user sync to propagate the change via gRPC.
func (s *Server) autoRevokeWireGuardPeers(customerID int64) {
	rows, err := s.DB.Query(`SELECT id FROM wg_peers WHERE customer_id=$1 AND status='active'`, customerID)
	if err != nil {
		return
	}
	defer rows.Close()

	var peerIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			continue
		}
		peerIDs = append(peerIDs, id)
	}
	if err := rows.Err(); err != nil {
		return
	}

	for _, id := range peerIDs {
		_, _ = s.DB.Exec(`UPDATE wg_peers SET status='revoked' WHERE id=$1`, id)
	}

	// Sync user to knode via gRPC (disabled state will be communicated)
	if s.UserSync != nil && len(peerIDs) > 0 {
		var username string
		_ = s.DB.QueryRow(`SELECT username FROM customers WHERE id = $1`, customerID).Scan(&username)
		if username != "" {
			go func() {
				if err := s.UserSync.SyncUser(context.Background(), username); err != nil {
					log.Printf("[knode] SyncUser failed after WireGuard revoke for %q: %v", username, err)
				}
			}()
		}
	}
}

// AutoRevokeWireGuardPeersByDB is a standalone function for use by the background worker.
// It revokes all active WireGuard peers for a given customer ID.
// Note: User sync is triggered by the caller (background worker) after revocation.
func AutoRevokeWireGuardPeersByDB(db *sql.DB, customerID int64) {
	rows, err := db.Query(`SELECT id FROM wg_peers WHERE customer_id=$1 AND status='active'`, customerID)
	if err != nil {
		return
	}
	defer rows.Close()

	var peerIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			continue
		}
		peerIDs = append(peerIDs, id)
	}
	if err := rows.Err(); err != nil {
		return
	}

	for _, id := range peerIDs {
		_, _ = db.Exec(`UPDATE wg_peers SET status='revoked' WHERE id=$1`, id)
	}
}

// portalWireguardPeerQR serves a QR code PNG of the WireGuard config for portal customers.
// Verifies the peer belongs to the authenticated customer (403 if not).
func (s *Server) portalWireguardPeerQR(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	username, _ := s.currentCustomer(r)

	// Get customer ID
	var customerID int64
	err := s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	// Fetch peer and verify ownership
	var peer WgPeer
	var peerCustomerID *int64
	err = s.DB.QueryRow(`
		SELECT id, customer_id, node_id, public_key, COALESCE(preshared_key,''),
		       COALESCE(private_key_encrypted,''), allowed_ips, COALESCE(endpoint,''), status
		FROM wg_peers WHERE id=$1`, id).Scan(
		&peer.ID, &peerCustomerID, &peer.NodeID, &peer.PublicKey,
		&peer.PresharedKey, &peer.PrivateKeyEncrypted, &peer.AllowedIPs,
		&peer.Endpoint, &peer.Status)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "peer_not_found"})
		return
	}

	// Verify peer belongs to authenticated customer
	if peerCustomerID == nil || *peerCustomerID != customerID {
		writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
		return
	}

	if peer.PrivateKeyEncrypted == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "private_key_not_available"})
		return
	}

	// Get server config for this peer's node
	var extraJSON []byte
	err = s.DB.QueryRow(`
		SELECT COALESCE(extra_json,'{}')
		FROM node_vpn_configs WHERE node_id=$1 AND protocol='wireguard'`, peer.NodeID).Scan(&extraJSON)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "wireguard_config_not_found_for_node"})
		return
	}

	var serverPublicKey, dns1, dns2, serverEndpoint string
	var gamingOptimize bool
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
		if v, ok := extra["gaming_optimize"].(bool); ok {
			gamingOptimize = v
		}
	}

	// Get node endpoint
	var nodeIP, nodeDomain string
	var wgPort int
	_ = s.DB.QueryRow(`SELECT COALESCE(public_ip,''), COALESCE(domain,'') FROM nodes WHERE id=$1`, peer.NodeID).Scan(&nodeIP, &nodeDomain)
	_ = s.DB.QueryRow(`SELECT port FROM node_vpn_configs WHERE node_id=$1 AND protocol='wireguard'`, peer.NodeID).Scan(&wgPort)

	if nodeDomain != "" {
		serverEndpoint = fmt.Sprintf("%s:%d", nodeDomain, wgPort)
	} else if nodeIP != "" {
		serverEndpoint = fmt.Sprintf("%s:%d", nodeIP, wgPort)
	}

	// Build DNS string
	dns := dns1
	if dns2 != "" {
		dns = dns1 + ", " + dns2
	}
	if dns == "" {
		dns = "1.1.1.1, 8.8.8.8"
	}

	// Generate config string
	conf := wireguard.GenerateClientConfig(wireguard.ClientConfig{
		PrivateKey:      peer.PrivateKeyEncrypted,
		Address:         peer.AllowedIPs,
		DNS:             dns,
		ServerPublicKey: serverPublicKey,
		PresharedKey:    peer.PresharedKey,
		Endpoint:        serverEndpoint,
		GamingOptimize:  gamingOptimize,
	})

	// Encode config as QR code PNG (256x256, medium error correction)
	png, err := qrcode.Encode(conf, qrcode.Medium, 256)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "qr_generation_failed"})
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}
