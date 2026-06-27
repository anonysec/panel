package api

import (
	"KorisPanel/panel/internal/wireguard"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) nodeVPNConfig(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/nodes/vpn-config/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_id_required"})
		return
	}
	nodeID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || nodeID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node_id"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getNodeVPNConfigs(w, nodeID)
	case http.MethodPost, http.MethodPatch:
		s.upsertNodeVPNConfig(w, r, nodeID)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getNodeVPNConfigs(w http.ResponseWriter, nodeID int64) {
	rows, err := s.DB.Query(`SELECT id, node_id, protocol, enabled, port, COALESCE(network,''), extra_json FROM node_vpn_configs WHERE node_id=$1 ORDER BY CASE protocol WHEN 'openvpn' THEN 1 WHEN 'l2tp' THEN 2 WHEN 'ikev2' THEN 3 WHEN 'ssh' THEN 4 WHEN 'wireguard' THEN 5 WHEN 'cisco_ipsec' THEN 6 ELSE 7 END`, nodeID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	configs := []NodeVPNConfig{}
	for rows.Next() {
		var c NodeVPNConfig
		var enabled bool
		var extra []byte
		if err := rows.Scan(&c.ID, &c.NodeID, &c.Protocol, &enabled, &c.Port, &c.Network, &extra); err == nil {
			c.Enabled = enabled
			c.Extra = extra
			configs = append(configs, c)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "configs": configs})
}

func (s *Server) upsertNodeVPNConfig(w http.ResponseWriter, r *http.Request, nodeID int64) {
	var in struct {
		Protocol string          `json:"protocol"`
		Enabled  bool            `json:"enabled"`
		Port     int             `json:"port"`
		Network  string          `json:"network"`
		Extra    json.RawMessage `json:"extra_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	if in.Protocol != "openvpn" && in.Protocol != "l2tp" && in.Protocol != "ikev2" && in.Protocol != "ssh" && in.Protocol != "wireguard" && in.Protocol != "cisco_ipsec" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_protocol"})
		return
	}

	// WireGuard-specific validation: stricter port range and network CIDR
	if in.Protocol == "wireguard" {
		if err := wireguard.ValidatePort(in.Port); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_port"})
			return
		}
		if strings.TrimSpace(in.Network) != "" {
			if err := wireguard.ValidateNetworkCIDR(strings.TrimSpace(in.Network)); err != nil {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_network_cidr"})
				return
			}
		}
	}

	if in.Port <= 0 || in.Port > 65535 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_port"})
		return
	}

	enabledInt := 0
	if in.Enabled {
		enabledInt = 1
	}
	extraStr := ""
	if len(in.Extra) > 0 {
		extraStr = string(in.Extra)
		// Validate outbound config if present
		var extraMap map[string]any
		if err := json.Unmarshal(in.Extra, &extraMap); err == nil {
			if outbound, ok := extraMap["outbound"].(map[string]any); ok {
				if enabled, _ := outbound["enabled"].(bool); enabled {
					oType, _ := outbound["type"].(string)
					validTypes := map[string]bool{"vless": true, "vmess": true, "trojan": true, "shadowsocks": true, "socks5": true}
					if oType != "" && !validTypes[oType] {
						writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_outbound_type"})
						return
					}
				}
			}
		}
	}

	_, err := s.DB.Exec(`INSERT INTO node_vpn_configs(node_id, protocol, enabled, port, network, extra_json)
		VALUES($1, $2, $3, $4, $5, $6)
		ON CONFLICT (node_id, protocol) DO UPDATE SET enabled=EXCLUDED.enabled, port=EXCLUDED.port, network=EXCLUDED.network, extra_json=EXCLUDED.extra_json`,
		nodeID, in.Protocol, enabledInt, in.Port, strings.TrimSpace(in.Network), nullString(extraStr))
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "node.vpn_config_updated", "node", strconv.FormatInt(nodeID, 10), nil, map[string]any{"protocol": in.Protocol, "port": in.Port, "enabled": in.Enabled}, clientIP(r))

	// Auto-start/stop service on the node when toggled via gRPC
	serviceMap := map[string]string{
		"openvpn":     "openvpn-server@server",
		"l2tp":        "xl2tpd",
		"ikev2":       "strongswan-starter",
		"ssh":         "ssh",
		"wireguard":   "wg-quick@wg0",
		"cisco_ipsec": "strongswan",
	}
	if svcName, ok := serviceMap[in.Protocol]; ok {
		if in.Enabled {
			if s.CoreMgr != nil {
				// Use gRPC EnableCore for the protocol with 10s timeout (R10.2)
				extraConfig, _ := json.Marshal(map[string]any{
					"port":    in.Port,
					"network": strings.TrimSpace(in.Network),
					"service": svcName,
				})
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				if err := s.CoreMgr.EnableCore(ctx, nodeID, in.Protocol, in.Port, extraConfig); err != nil {
					log.Printf("[knode] EnableCore gRPC failed for node %d protocol %s: %v", nodeID, in.Protocol, err)
					// Report error to admin UI, leave core in previous state (R10.3)
					writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
					return
				}
			} else {
				log.Printf("[knode] gRPC pool not configured, cannot enable core %s on node %d", in.Protocol, nodeID)
			}
		} else {
			if s.CoreMgr != nil {
				// Use gRPC DisableCore for the protocol with 10s timeout (R10.2)
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				if err := s.CoreMgr.DisableCore(ctx, nodeID, in.Protocol); err != nil {
					log.Printf("[knode] DisableCore gRPC failed for node %d protocol %s: %v", nodeID, in.Protocol, err)
					// Report error to admin UI, leave core in previous state (R10.3)
					writeJSONCode(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
					return
				}
			} else {
				log.Printf("[knode] gRPC pool not configured, cannot disable core %s on node %d", in.Protocol, nodeID)
			}
		}
	}

	writeJSON(w, map[string]any{"ok": true})
}

// ========== Certificates ==========

type VPNCertificate struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	NodeID    *int64 `json:"node_id,omitempty"`
	Content   string `json:"content"`
	IsDefault bool   `json:"is_default"`
	CreatedAt string `json:"created_at"`
}
