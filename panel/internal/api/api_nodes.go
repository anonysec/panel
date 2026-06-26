package api

import (
	"KorisPanel/panel/internal/auth"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

func (s *Server) nodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listNodes(w, r)
	case http.MethodPost:
		s.createNode(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) nodeByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/nodes/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodGet:
			s.getNode(w, id)
		case http.MethodPatch:
			s.updateNode(w, r, id)
		case http.MethodDelete:
			s.deleteNode(w, id)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
		return
	}
	// Cores sub-routes need special handling (support POST and DELETE)
	if action == "cores" {
		s.dispatchNodeCores(w, r, id)
		return
	}
	// antidpi supports GET, POST, DELETE — handle before POST-only check
	if action == "antidpi" {
		// Extract technique from remaining path: /api/nodes/{id}/antidpi/{technique}
		technique := ""
		rest := strings.TrimPrefix(r.URL.Path, "/api/nodes/")
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		if len(parts) >= 3 {
			technique = parts[2]
		}
		s.handleNodeAntiDPI(w, r, id, technique)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "rotate-token":
		s.rotateNodeToken(w, r, id)
	case "enable":
		s.setNodeStatus(w, id, "offline")
	case "disable":
		s.setNodeStatus(w, id, "disabled")
	case "assign-group":
		s.assignNodeToGroup(w, r, id)
	case "migrate":
		s.handleNodeMigrate(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) {
	s.markStaleNodes()

	// Check for tag filtering
	tagsParam := r.URL.Query().Get("tags")
	if tagsParam != "" {
		// Tag filtering — bypass cache, run filtered query
		tags := strings.Split(tagsParam, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		// Remove empty tags
		filtered := tags[:0]
		for _, t := range tags {
			if t != "" {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) == 0 {
			writeJSON(w, map[string]any{"ok": true, "nodes": []Node{}})
			return
		}

		// Build IN clause placeholders
		placeholders := make([]string, len(filtered))
		args := make([]any, len(filtered))
		for i, t := range filtered {
			placeholders[i] = "?"
			args[i] = t
		}

		query := `SELECT id,name,public_ip,COALESCE(domain,''),status,last_seen_at,created_at,proxy_config FROM nodes WHERE id IN (SELECT node_id FROM node_tags WHERE tag IN (` + strings.Join(placeholders, ",") + `)) ORDER BY sort_order ASC, id DESC LIMIT 500`
		rows, err := s.DB.Query(query, args...)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer rows.Close()
		out := []Node{}
		for rows.Next() {
			node, err := s.scanNode(rows)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			_ = s.fillNodeRuntime(&node)
			out = append(out, node)
		}
		writeJSON(w, map[string]any{"ok": true, "nodes": out})
		return
	}

	result, err := s.cachedQuery("nodes:list", func() (any, error) {
		rows, err := s.DB.Query(`SELECT id,name,public_ip,COALESCE(domain,''),status,last_seen_at,created_at,proxy_config FROM nodes ORDER BY sort_order ASC, id DESC LIMIT 500`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []Node{}
		for rows.Next() {
			node, err := s.scanNode(rows)
			if err != nil {
				return nil, err
			}
			_ = s.fillNodeRuntime(&node)
			out = append(out, node)
		}
		return map[string]any{"ok": true, "nodes": out}, nil
	})
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, result)
}

func (s *Server) getNode(w http.ResponseWriter, id int64) {
	s.markStaleNodes()
	node, err := s.scanNode(s.DB.QueryRow(`SELECT id,name,public_ip,COALESCE(domain,''),status,last_seen_at,created_at,proxy_config FROM nodes WHERE id=$1 LIMIT 1`, id))
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = s.fillNodeRuntime(&node)
	writeJSON(w, map[string]any{"ok": true, "node": node})
}

func (s *Server) createNode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		PublicIP string `json:"public_ip"`
		Domain   string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.PublicIP = strings.TrimSpace(in.PublicIP)
	in.Domain = strings.TrimSpace(in.Domain)
	if in.Name == "" || in.PublicIP == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_public_ip_required"})
		return
	}
	token := "kn_" + auth.RandomToken(24)
	res, err := s.DB.Exec(`INSERT INTO nodes(name,public_ip,domain,api_token_hash,status) VALUES($1,$2,$3,$4, 'offline')`, in.Name, in.PublicIP, nullString(in.Domain), hashToken(token))
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	// Create default VPN configs for all protocols on the new node
	defaultConfigs := []struct {
		protocol string
		port     int
		network  string
		extra    string
	}{
		{"openvpn", 1194, "10.8.0.0/20", `{"transport":"udp","cipher":"AES-256-GCM","tls_mode":"tls-crypt","dns1":"8.8.8.8","dns2":"8.8.4.4","comp_lzo":false,"topology":"subnet","verb":3,"keepalive":"10 120"}`},
		{"l2tp", 1701, "10.9.0.0/20", `{"ipsec_mode":"ipsec","psk":"","auth_method":"CHAP","dns1":"8.8.8.8","dns2":"8.8.4.4","lcp_echo_interval":30,"lcp_echo_failure":4}`},
		{"ikev2", 500, "10.10.0.0/20", `{"auth_type":"psk","psk":"","dns1":"8.8.8.8","dns2":"8.8.4.4","dpd_interval":30,"dpd_timeout":150,"rekey_time":"4h","ike_proposals":"aes256-sha256-modp2048","esp_proposals":"aes256-sha256"}`},
		{"ssh", 2222, "", `{"listen_address":"0.0.0.0","key_type":"ed25519","max_sessions":10,"idle_timeout":0,"shell_access":false,"accounting_enabled":true,"accounting_interval":300}`},
		{"wireguard", 51820, "10.66.0.0/20", `{"dns_1":"1.1.1.1","dns_2":"8.8.8.8","gaming_optimize":false}`},
		{"cisco_ipsec", 500, "10.11.0.0/20", `{"ike_version":"ikev1","auth_method":"xauth_psk","psk":"","dns1":"8.8.8.8","dns2":"8.8.4.4","dpd_interval":30,"dpd_timeout":150}`},
	}
	for _, dc := range defaultConfigs {
		_, _ = s.DB.Exec(`INSERT INTO node_vpn_configs(node_id, protocol, enabled, port, network, extra_json) VALUES($1, $2, 0, $3, $4, $5)`,
			id, dc.protocol, dc.port, dc.network, dc.extra)
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "node.created", "node", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name}, clientIP(r))
	if s.Cache != nil {
		s.Cache.InvalidatePrefix("nodes:")
	}
	writeJSON(w, map[string]any{"ok": true, "id": id, "token": token})
}

func (s *Server) updateNode(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		Name             string  `json:"name"`
		PublicIP         string  `json:"public_ip"`
		Domain           string  `json:"domain"`
		BandwidthQuotaGB *int64  `json:"bandwidth_quota_gb,omitempty"`
		ProxyEnabled     *bool   `json:"proxy_enabled,omitempty"`
		ProxyType        *string `json:"proxy_type,omitempty"`
		ProxyAddress     *string `json:"proxy_address,omitempty"`
		ProxyUsername    *string `json:"proxy_username,omitempty"`
		ProxyPassword    *string `json:"proxy_password,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.PublicIP = strings.TrimSpace(in.PublicIP)
	in.Domain = strings.TrimSpace(in.Domain)
	if in.Name == "" || in.PublicIP == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_public_ip_required"})
		return
	}

	// Build proxy_config JSON if any proxy fields are provided
	var proxyConfigJSON *string
	if in.ProxyEnabled != nil || in.ProxyType != nil || in.ProxyAddress != nil {
		pc := map[string]any{}
		if in.ProxyEnabled != nil {
			pc["enabled"] = *in.ProxyEnabled
		}
		if in.ProxyType != nil {
			pc["type"] = *in.ProxyType
		}
		if in.ProxyAddress != nil {
			pc["address"] = *in.ProxyAddress
		}
		if in.ProxyUsername != nil {
			pc["username"] = *in.ProxyUsername
		}
		if in.ProxyPassword != nil {
			pc["password"] = *in.ProxyPassword
		}
		b, _ := json.Marshal(pc)
		s := string(b)
		proxyConfigJSON = &s
	}

	if proxyConfigJSON != nil {
		if _, err := s.DB.Exec(`UPDATE nodes SET name=$1,public_ip=$2,domain=$3,proxy_config=$4 WHERE id=$5`, in.Name, in.PublicIP, nullString(in.Domain), *proxyConfigJSON, id); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	} else {
		if _, err := s.DB.Exec(`UPDATE nodes SET name=$1,public_ip=$2,domain=$3 WHERE id=$4`, in.Name, in.PublicIP, nullString(in.Domain), id); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}

	// Update bandwidth_quota_gb if provided
	if in.BandwidthQuotaGB != nil {
		var quotaVal any
		if *in.BandwidthQuotaGB <= 0 {
			quotaVal = nil // 0 or negative means remove quota
		} else {
			quotaVal = *in.BandwidthQuotaGB
		}
		if _, err := s.DB.Exec(`UPDATE nodes SET bandwidth_quota_gb = $1 WHERE id = $2`, quotaVal, id); err != nil {
			log.Printf("[bandwidth] failed to update quota for node %d: %v", id, err)
		}
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "node.updated", "node", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name}, clientIP(r))

	// Push config update to the node agent so NODE_NAME stays in sync
	// NOTE: Legacy node_tasks removed. Config updates are now handled via gRPC.
	if s.GRPCPool != nil {
		log.Printf("[node] config update for node %d would be pushed via gRPC", id)
	}

	if s.Cache != nil {
		s.Cache.InvalidatePrefix("nodes:")
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) rotateNodeToken(w http.ResponseWriter, r *http.Request, id int64) {
	token := "kn_" + auth.RandomToken(24)
	if _, err := s.DB.Exec(`UPDATE nodes SET api_token_hash=$1 WHERE id=$2`, hashToken(token), id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Push new token to the node agent before returning
	// NOTE: Legacy node_tasks removed. Token updates are now handled via gRPC.
	if s.GRPCPool != nil {
		log.Printf("[node] token rotation for node %d would be pushed via gRPC", id)
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "node.token_rotated", "node", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "token": token})
}

func (s *Server) setNodeStatus(w http.ResponseWriter, id int64, status string) {
	if _, err := s.DB.Exec(`UPDATE nodes SET status=$1 WHERE id=$2`, status, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// When disabling a node, disconnect all active sessions on it and revoke WireGuard peers
	if status == "disabled" {
		// Get node's NAS IP for RADIUS disconnect
		var nasIP string
		_ = s.DB.QueryRow(`SELECT public_ip FROM nodes WHERE id=$1`, id).Scan(&nasIP)
		if nasIP == "" {
			nasIP = "127.0.0.1"
		}

		// Disconnect all active RADIUS sessions originating from this node
		rows, err := s.DB.Query(`SELECT radacctid, username, acctsessionid FROM radacct WHERE acctstoptime IS NULL AND nasipaddress=$1`, nasIP)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var radID int64
				var username, sessionID string
				if rows.Scan(&radID, &username, &sessionID) == nil {
					// Close the session in radacct
					_, _ = s.DB.Exec(`UPDATE radacct SET acctstoptime=NOW(), acctterminatecause='Admin-Node-Disabled' WHERE radacctid=$1`, radID)
					// Send CoA disconnect (best effort)
					go func(u, sid, ip string) {
						attrs := fmt.Sprintf("User-Name=%s,Acct-Session-Id=%s", u, sid)
						cmd := exec.Command("radclient", "-x", ip+":3799", "disconnect", "testing123")
						cmd.Stdin = strings.NewReader(attrs)
						_ = cmd.Run()
					}(username, sessionID, nasIP)
				}
			}
		}

		// Revoke WireGuard peers on this node
		_, _ = s.DB.Exec(`UPDATE wg_peers SET status='revoked' WHERE node_id=$1 AND status='active'`, id)

		log.Printf("[node] disabled node %d, disconnected sessions and revoked WG peers", id)
	}

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) deleteNode(w http.ResponseWriter, id int64) {
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	// Clean up all related tables within a transaction (explicit queries, no concatenation)
	if _, err := tx.Exec(`DELETE FROM node_vpn_configs WHERE node_id=$1`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Sprintf("failed to clean node_vpn_configs: %v", err)})
		return
	}
	// node_tasks table will be dropped in migration 071; skip cleanup for now
	if _, err := tx.Exec(`DELETE FROM node_status WHERE node_id=$1`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Sprintf("failed to clean node_status: %v", err)})
		return
	}
	if _, err := tx.Exec(`DELETE FROM node_services WHERE node_id=$1`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Sprintf("failed to clean node_services: %v", err)})
		return
	}
	if _, err := tx.Exec(`DELETE FROM node_usage_snapshots WHERE node_id=$1`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Sprintf("failed to clean node_usage_snapshots: %v", err)})
		return
	}
	if _, err := tx.Exec(`DELETE FROM node_diagnostics WHERE node_id=$1`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Sprintf("failed to clean node_diagnostics: %v", err)})
		return
	}

	if _, err := tx.Exec(`DELETE FROM nodes WHERE id=$1`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if s.Cache != nil {
		s.Cache.InvalidatePrefix("nodes:")
	}
	writeJSON(w, map[string]any{"ok": true})
}

type nodeScanner interface{ Scan(dest ...any) error }
