package api

import (
	"KorisPanel/panel/internal/templates"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func (s *Server) vpnSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.readVPNSettings(r)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "settings": settings})
	case http.MethodPatch:
		var in struct {
			VPNSettings
			Apply bool `json:"apply"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}
		in.OpenVPNProtocol = strings.ToLower(strings.TrimSpace(in.OpenVPNProtocol))
		if in.OpenVPNProtocol != "udp" && in.OpenVPNProtocol != "tcp" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_protocol"})
			return
		}
		if in.OpenVPNPort <= 0 || in.OpenVPNPort > 65535 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_port"})
			return
		}
		if in.OpenVPNNetwork == "" || in.L2TPNetwork == "" || in.IKEv2Network == "" || in.DNS1 == "" || in.DNS2 == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_required_settings"})
			return
		}
		// Validate network CIDRs are private RFC1918 ranges
		if err := templates.ValidatePrivateNetwork(in.OpenVPNNetwork, false); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid openvpn_network: " + err.Error()})
			return
		}
		if err := templates.ValidatePrivateNetwork(in.L2TPNetwork, false); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid l2tp_network: " + err.Error()})
			return
		}
		if err := templates.ValidatePrivateNetwork(in.IKEv2Network, false); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid ikev2_network: " + err.Error()})
			return
		}
		// Validate port, protocol, and DNS
		if err := templates.ValidatePort(in.OpenVPNPort); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := templates.ValidateProtocol(in.OpenVPNProtocol); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := templates.ValidateDNS(in.DNS1); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid dns_1: " + err.Error()})
			return
		}
		if err := templates.ValidateDNS(in.DNS2); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid dns_2: " + err.Error()})
			return
		}
		_, err := s.DB.Exec(`INSERT INTO vpn_core_settings(id,openvpn_port,openvpn_protocol,openvpn_network,l2tp_network,ikev2_network,ipsec_psk,dns_1,dns_2)
			VALUES(1,$1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (id) DO UPDATE SET openvpn_port=EXCLUDED.openvpn_port,openvpn_protocol=EXCLUDED.openvpn_protocol,openvpn_network=EXCLUDED.openvpn_network,l2tp_network=EXCLUDED.l2tp_network,ikev2_network=EXCLUDED.ikev2_network,ipsec_psk=EXCLUDED.ipsec_psk,dns_1=EXCLUDED.dns_1,dns_2=EXCLUDED.dns_2`, in.OpenVPNPort, in.OpenVPNProtocol, in.OpenVPNNetwork, in.L2TPNetwork, in.IKEv2Network, nullString(in.IPSecPSK), in.DNS1, in.DNS2)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		applied := false
		applyError := ""
		if in.Apply {
			if err := applyOpenVPNServerConfig(in.VPNSettings); err != nil {
				applyError = err.Error()
			} else {
				applied = true
			}
		}
		settings, _ := s.readVPNSettings(r)
		actor, _, _ := s.currentAdmin(r)
		s.logAudit(actor, "vpn.settings_saved", "vpn_settings", "1", nil, map[string]any{"applied": applied}, clientIP(r))
		writeJSON(w, map[string]any{"ok": true, "settings": settings, "applied": applied, "apply_error": applyError})
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) readVPNSettings(r *http.Request) (VPNSettings, error) {
	var v VPNSettings
	var updated sql.NullTime
	err := s.DB.QueryRow(`SELECT id,openvpn_port,openvpn_protocol,openvpn_network,l2tp_network,ikev2_network,COALESCE(ipsec_psk,''),dns_1,dns_2,updated_at FROM vpn_core_settings WHERE id=1`).Scan(&v.ID, &v.OpenVPNPort, &v.OpenVPNProtocol, &v.OpenVPNNetwork, &v.L2TPNetwork, &v.IKEv2Network, &v.IPSecPSK, &v.DNS1, &v.DNS2, &updated)
	if err == sql.ErrNoRows {
		_, _ = s.DB.Exec(`INSERT INTO vpn_core_settings(id) VALUES(1) ON CONFLICT (id) DO NOTHING`)
		err = s.DB.QueryRow(`SELECT id,openvpn_port,openvpn_protocol,openvpn_network,l2tp_network,ikev2_network,COALESCE(ipsec_psk,''),dns_1,dns_2,updated_at FROM vpn_core_settings WHERE id=1`).Scan(&v.ID, &v.OpenVPNPort, &v.OpenVPNProtocol, &v.OpenVPNNetwork, &v.L2TPNetwork, &v.IKEv2Network, &v.IPSecPSK, &v.DNS1, &v.DNS2, &updated)
	}
	if err != nil {
		return v, err
	}
	if updated.Valid {
		v.UpdatedAt = updated.Time.Format(time.RFC3339)
	}
	ca := getenvFirst("PANEL_OPENVPN_CA_FILE", "/etc/openvpn/server/ca.crt", "/etc/openvpn/easy-rsa/pki/ca.crt")
	tls := getenvFirst("PANEL_OPENVPN_TLS_CRYPT_FILE", "/etc/openvpn/server/tc.key", "/etc/openvpn/server/tls-crypt.key", "/etc/openvpn/server/ta.key")
	v.CAFile = ca
	v.TLSCryptFile = tls
	_, v.CAExists = fileExists(ca)
	_, v.TLSCryptExists = fileExists(tls)
	v.RemoteHost, _, _, v.ActiveNode = s.openVPNEndpoint(r)
	v.OpenVPNServiceStatus = "unknown"
	_ = s.DB.QueryRow(`SELECT openvpn_status FROM node_status ORDER BY updated_at DESC LIMIT 1`).Scan(&v.OpenVPNServiceStatus)
	return v, nil
}

func fileExists(path string) (os.FileInfo, bool) {
	if strings.TrimSpace(path) == "" {
		return nil, false
	}
	info, err := os.Stat(path)
	return info, err == nil
}

func applyOpenVPNServerConfig(v VPNSettings) error {
	// Validate inputs
	if err := templates.ValidatePort(v.OpenVPNPort); err != nil {
		return fmt.Errorf("port validation failed: %w", err)
	}
	if err := templates.ValidateProtocol(v.OpenVPNProtocol); err != nil {
		return fmt.Errorf("protocol validation failed: %w", err)
	}
	if v.OpenVPNNetwork == "" {
		return fmt.Errorf("network validation failed: OpenVPN network is required")
	}

	conf := strings.TrimSpace(os.Getenv("PANEL_OPENVPN_SERVER_CONF"))
	if conf == "" {
		conf = "/etc/openvpn/server/server.conf"
	}

	serverNet, serverMask := cidrToOpenVPNServer(v.OpenVPNNetwork)

	vars := templates.TemplateVars{
		Port:       v.OpenVPNPort,
		Protocol:   v.OpenVPNProtocol,
		Network:    v.OpenVPNNetwork,
		ServerNet:  serverNet,
		ServerMask: serverMask,
		DNS1:       v.DNS1,
		DNS2:       v.DNS2,
	}

	engine := templates.NewEngine(os.Getenv("PANEL_TEMPLATE_DIR"))
	if err := engine.Apply("openvpn", conf, vars); err != nil {
		return err
	}

	cmd := exec.Command("systemctl", "restart", "openvpn-server@server")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restart openvpn: %w: %s", err, string(out))
	}
	return nil
}

func cidrToOpenVPNServer(cidr string) (string, string) {
	ip, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil || ip.To4() == nil {
		return "", ""
	}
	mask := ipNet.Mask
	return ip.To4().String(), fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}

// authNode authenticates a node request.
// Token is read ONLY from the X-Node-Token header. The request body is never consumed,
// allowing downstream handlers to read it for their own purposes.
func (s *Server) authNode(r *http.Request) (int64, bool) {
	token := r.Header.Get("X-Node-Token")
	if token == "" {
		return 0, false
	}
	var id int64
	var status string
	var err error
	if s.initStmts() {
		err = s.stmts.nodeAuth.QueryRow(hashToken(token)).Scan(&id, &status)
	} else {
		err = s.DB.QueryRow(`SELECT id,status FROM nodes WHERE api_token_hash=$1 LIMIT 1`, hashToken(token)).Scan(&id, &status)
	}
	if err != nil || status == "disabled" {
		return 0, false
	}
	return id, true
}
