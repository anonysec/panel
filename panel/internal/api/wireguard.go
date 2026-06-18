package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"KorisPanel/panel/internal/wireguard"
)

// WgPeer represents a WireGuard peer record from the database.
type WgPeer struct {
	ID                  int64  `json:"id"`
	CustomerID          *int64 `json:"customer_id"`
	NodeID              int64  `json:"node_id"`
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
	rows, err := s.DB.Query(`
		SELECT p.id, p.customer_id, p.node_id, p.public_key, p.allowed_ips,
		       COALESCE(p.endpoint,''), p.status, p.rx_bytes, p.tx_bytes,
		       p.created_at, p.updated_at, COALESCE(c.username,'')
		FROM wg_peers p
		LEFT JOIN customers c ON c.id = p.customer_id
		ORDER BY p.id DESC
		LIMIT 500`)
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
		if err := rows.Scan(&p.ID, &customerID, &p.NodeID, &p.PublicKey, &p.AllowedIPs,
			&p.Endpoint, &p.Status, &p.RxBytes, &p.TxBytes,
			&created, &updated, &p.Username); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		p.CustomerID = customerID
		p.CreatedAt = created.Format(time.RFC3339)
		p.UpdatedAt = updated.Format(time.RFC3339)
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
		err := s.DB.QueryRow(`SELECT network, COALESCE(extra_json,'{}') FROM node_vpn_configs WHERE node_id=? AND protocol='wireguard' LIMIT 1`, in.NodeID).Scan(&networkCIDR, &cfgExtraJSON)
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
		rows, err := s.DB.Query(`SELECT allowed_ips FROM wg_peers WHERE node_id=? AND status='active'`, in.NodeID)
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
		VALUES (?, ?, ?, ?, ?, ?, 'active')`,
		in.CustomerID, in.NodeID, publicKey, presharedKey, privateKey, in.AllowedIPs)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	peerID, _ := res.LastInsertId()

	// Create node task for adding the peer on the node
	payload, _ := json.Marshal(map[string]any{
		"public_key":    publicKey,
		"preshared_key": presharedKey,
		"allowed_ips":   in.AllowedIPs,
	})
	actor, _, _ := s.currentAdmin(r)
	_, _ = s.DB.Exec(`INSERT INTO node_tasks(node_id, action, payload_json, status, created_by) VALUES(?,?,?,?,?)`,
		in.NodeID, "wireguard.add_peer", string(payload), "pending", actor)

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
	err := s.DB.QueryRow(`SELECT node_id, public_key FROM wg_peers WHERE id=?`, id).Scan(&nodeID, &publicKey)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "peer_not_found"})
		return
	}

	// Set peer status to revoked
	if _, err := s.DB.Exec(`UPDATE wg_peers SET status='revoked' WHERE id=?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Create node task to remove the peer
	payload, _ := json.Marshal(map[string]any{
		"public_key": publicKey,
	})
	actor, _, _ := s.currentAdmin(r)
	_, _ = s.DB.Exec(`INSERT INTO node_tasks(node_id, action, payload_json, status, created_by) VALUES(?,?,?,?,?)`,
		nodeID, "wireguard.remove_peer", string(payload), "pending", actor)

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
		FROM wg_peers WHERE id=?`, id).Scan(
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
		FROM node_vpn_configs WHERE node_id=? AND protocol='wireguard'`, peer.NodeID).Scan(&extraJSON, new(int))
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
	_ = s.DB.QueryRow(`SELECT COALESCE(public_ip,''), COALESCE(domain,'') FROM nodes WHERE id=?`, peer.NodeID).Scan(&nodeIP, &nodeDomain)
	_ = s.DB.QueryRow(`SELECT port FROM node_vpn_configs WHERE node_id=? AND protocol='wireguard'`, peer.NodeID).Scan(&wgPort)

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


