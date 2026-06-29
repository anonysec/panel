package api

import (
	"KorisPanel/panel/internal/auth"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func (s *Server) portalProfileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/openvpn-tcp.ovpn"):
		nodeID, _ := strconv.ParseInt(r.URL.Query().Get("node_id"), 10, 64)
		profile := s.openVPNProfileTCP(username, r, nodeID)
		_, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
		nodeBase := safeFilename(nodeName)
		if nodeBase == "" {
			nodeBase = "vpn"
		}
		filename := nodeBase + "-TCP.ovpn"
		w.Header().Set("Content-Type", "application/x-openvpn-profile; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+url.PathEscape(filename))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(profile))
	case strings.HasSuffix(path, "/openvpn.ovpn"):
		nodeID, _ := strconv.ParseInt(r.URL.Query().Get("node_id"), 10, 64)
		passwordless := r.URL.Query().Get("passwordless") == "true"
		var profile string
		if passwordless && s.canUsePasswordless(username) {
			profile = s.openVPNProfilePasswordless(username, r, nodeID)
		} else {
			profile = s.openVPNProfile(username, r, nodeID)
		}
		_, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
		nodeBase := safeFilename(nodeName)
		if nodeBase == "" {
			nodeBase = "vpn"
		}
		// Passwordless configs are per-user; standard OpenVPN is generic (node name only)
		var filename string
		if passwordless {
			filename = safeFilename(username) + "-" + nodeBase + ".ovpn"
		} else {
			filename = nodeBase + ".ovpn"
		}
		w.Header().Set("Content-Type", "application/x-openvpn-profile; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+url.PathEscape(filename))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(profile))
	case strings.HasSuffix(path, "/l2tp.mobileconfig"):
		nodeID, _ := strconv.ParseInt(r.URL.Query().Get("node_id"), 10, 64)
		profile := s.l2tpMobileConfig(username, r, nodeID)
		_, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
		nodeBase := safeFilename(nodeName)
		if nodeBase == "" {
			nodeBase = "vpn"
		}
		// mobileconfig embeds username — always per-user
		filename := safeFilename(username) + "-" + nodeBase + ".mobileconfig"
		w.Header().Set("Content-Type", "application/x-apple-aspen-config; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+url.PathEscape(filename))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(profile))
	case strings.HasSuffix(path, "/ikev2.mobileconfig"):
		nodeID, _ := strconv.ParseInt(r.URL.Query().Get("node_id"), 10, 64)
		profile := s.ikev2MobileConfig(username, r, nodeID)
		_, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
		nodeBase := safeFilename(nodeName)
		if nodeBase == "" {
			nodeBase = "vpn"
		}
		// mobileconfig embeds username — always per-user
		filename := safeFilename(username) + "-" + nodeBase + "-ikev2.mobileconfig"
		w.Header().Set("Content-Type", "application/x-apple-aspen-config; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+url.PathEscape(filename))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(profile))
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) openVPNEndpoint(r *http.Request) (host string, port int, proto string, nodeName string) {
	port = 1194
	proto = "udp"
	_ = s.DB.QueryRow(`SELECT openvpn_port,openvpn_protocol FROM vpn_core_settings WHERE id=1`).Scan(&port, &proto)
	var address string
	_ = s.DB.QueryRow(`SELECT name, address FROM knode_connections WHERE enabled=TRUE ORDER BY CASE status WHEN 'online' THEN 0 WHEN 'stale' THEN 1 ELSE 2 END, id ASC LIMIT 1`).Scan(&nodeName, &address)
	host = strings.TrimSpace(address)
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

func (s *Server) openVPNProfile(username string, r *http.Request, nodeID int64) string {
	return s.openVPNProfileWithAuth(username, r, nodeID, true)
}

func (s *Server) openVPNProfilePasswordless(username string, r *http.Request, nodeID int64) string {
	return s.openVPNProfileWithAuth(username, r, nodeID, false)
}

// openVPNProfileTCP generates a TCP-based OpenVPN config on port 443.
// Uses the user's preferred node as primary, with backup nodes as fallback.
func (s *Server) openVPNProfileTCP(username string, r *http.Request, nodeID int64) string {
	host, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
	if nodeName == "" {
		nodeName = host
	}
	caBlock := inlineOpenVPNBlock("ca", getenvFirst("PANEL_OPENVPN_CA_FILE", "/etc/openvpn/server/ca.crt", "/etc/openvpn/easy-rsa/pki/ca.crt"))
	tlsCryptBlock := inlineOpenVPNBlock("tls-crypt", getenvFirst("PANEL_OPENVPN_TLS_CRYPT_FILE", "/etc/openvpn/server/tc.key", "/etc/openvpn/server/tls-crypt.key", "/etc/openvpn/server/ta.key"))

	// Get TCP port from node config or default to 8443
	tcpPort := 8443
	if nodeID > 0 {
		_ = s.DB.QueryRow(`SELECT port FROM node_vpn_configs WHERE node_id=$1 AND protocol='openvpn-tcp' AND enabled=TRUE LIMIT 1`, nodeID).Scan(&tcpPort)
	}

	// Build remote lines for TCP: preferred node first, then backups
	remoteLines := fmt.Sprintf("remote %s %d tcp", host, tcpPort)

	// Get user's preferred node — put it first if different from default
	var preferredNodeID int64
	_ = s.DB.QueryRow(`SELECT COALESCE(preferred_node_id, 0) FROM customers WHERE username=$1 AND deleted_at IS NULL`, username).Scan(&preferredNodeID)
	if preferredNodeID > 0 && preferredNodeID != nodeID {
		var prefIP string
		if s.DB.QueryRow(`SELECT address FROM knode_connections WHERE id=$1 AND enabled=TRUE`, preferredNodeID).Scan(&prefIP) == nil {
			prefHost := strings.TrimSpace(prefIP)
			if prefHost != "" && prefHost != host {
				// Preferred node goes first
				remoteLines = fmt.Sprintf("remote %s %d tcp\nremote %s %d tcp", prefHost, tcpPort, host, tcpPort)
			}
		}
	}

	// Add other active nodes as backup
	rows, err := s.DB.Query(`
		SELECT n.address
		FROM knode_connections n
		JOIN node_vpn_configs c ON c.node_id = n.id AND c.protocol = 'openvpn' AND c.enabled = TRUE
		WHERE n.enabled = TRUE AND n.id <> $1 AND n.id <> $2
		ORDER BY n.id`, nodeID, preferredNodeID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ip string
			if rows.Scan(&ip) == nil {
				backupHost := strings.TrimSpace(ip)
				if backupHost != "" && backupHost != host {
					remoteLines += fmt.Sprintf("\nremote %s %d tcp", backupHost, tcpPort)
				}
			}
		}
	}

	return fmt.Sprintf(`# KorisPanel OpenVPN TCP Profile
# User: %s
# Node: %s
# Generated: %s
# TCP mode — supports node selection via portal
client
dev tun
%s
resolv-retry infinite
nobind
persist-key
persist-tun
remote-cert-tls server
setenv CLIENT_CERT 0
auth-user-pass
auth-nocache
auth SHA256
data-ciphers AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305
data-ciphers-fallback AES-256-GCM
verb 3
pull
%s%s`, username, nodeName, time.Now().UTC().Format(time.RFC3339), remoteLines, caBlock, tlsCryptBlock)
}

// canUsePasswordless checks if a customer is allowed to generate passwordless configs.
// Requires: global setting enabled AND customer's plan allows passwordless.
func (s *Server) canUsePasswordless(username string) bool {
	// Check global setting
	var enabled string
	_ = s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key='passwordless_configs_enabled'`).Scan(&enabled)
	if enabled != "true" {
		return false
	}
	// Check per-plan setting
	var allowPasswordless int
	err := s.DB.QueryRow(`SELECT COALESCE(p.allow_passwordless, 0) FROM customers c JOIN plans p ON p.id = c.plan_id WHERE c.username = $1 AND c.deleted_at IS NULL LIMIT 1`, username).Scan(&allowPasswordless)
	if err != nil {
		return false
	}
	return allowPasswordless == 1
}

func (s *Server) openVPNProfileWithAuth(username string, r *http.Request, nodeID int64, withAuth bool) string {
	host, port, proto, nodeName := s.openVPNEndpointNode(r, nodeID)
	if nodeName == "" {
		nodeName = host
	}
	caBlock := inlineOpenVPNBlock("ca", getenvFirst("PANEL_OPENVPN_CA_FILE", "/etc/openvpn/server/ca.crt", "/etc/openvpn/easy-rsa/pki/ca.crt"))
	tlsCryptBlock := inlineOpenVPNBlock("tls-crypt", getenvFirst("PANEL_OPENVPN_TLS_CRYPT_FILE", "/etc/openvpn/server/tc.key", "/etc/openvpn/server/tls-crypt.key", "/etc/openvpn/server/ta.key"))

	authLine := "auth-user-pass\n"
	authComment := "# Login with your VPN username/password when OpenVPN asks for credentials."
	if !withAuth {
		authLine = ""
		authComment = "# Passwordless mode — no credentials required."
	}

	// Build remote lines: primary + backup nodes for failover
	remoteLines := fmt.Sprintf("remote %s %d %s", host, port, proto)

	// Add backup remotes: all other active nodes with OpenVPN enabled
	rows, err := s.DB.Query(`
		SELECT n.address
		FROM knode_connections n
		JOIN node_vpn_configs c ON c.node_id = n.id AND c.protocol = 'openvpn' AND c.enabled = TRUE
		WHERE n.enabled = TRUE AND n.id <> $1
		ORDER BY n.id`, nodeID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ip string
			if rows.Scan(&ip) == nil {
				backupHost := strings.TrimSpace(ip)
				if backupHost != "" && backupHost != host {
					remoteLines += fmt.Sprintf("\nremote %s %d %s", backupHost, port, proto)
				}
			}
		}
	}

	// Add remote-random only if explicitly configured (disabled by default)
	// Load balancing is handled by smart proxy, not client-side randomization

	return fmt.Sprintf(`# KorisPanel generated OpenVPN profile
# User: %s
# Node: %s
# Generated: %s
%s
client
dev tun
%s
resolv-retry infinite
nobind
persist-key
persist-tun
remote-cert-tls server
setenv CLIENT_CERT 0
%sauth-nocache
auth SHA256
data-ciphers AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305
data-ciphers-fallback AES-256-GCM
explicit-exit-notify 1
verb 3
pull
%s%s`, username, nodeName, time.Now().UTC().Format(time.RFC3339), authComment, remoteLines, authLine, caBlock, tlsCryptBlock)
}

func getenvFirst(envName string, paths ...string) string {
	if v := strings.TrimSpace(os.Getenv(envName)); v != "" {
		return v
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func inlineOpenVPNBlock(name, filePath string) string {
	if filePath == "" {
		return ""
	}
	b, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(b))
	if content == "" {
		return ""
	}
	return fmt.Sprintf("\n<%s>\n%s\n</%s>\n", name, content, name)
}

func safeFilename(s string) string {
	return strings.NewReplacer("/", "_", "\\", "_", " ", "_", "\x00", "_").Replace(s)
}

func (s *Server) l2tpMobileConfig(username string, r *http.Request, nodeID int64) string {
	host, _, _, _ := s.openVPNEndpointNode(r, nodeID)
	if host == "" {
		host = r.Host
	}
	var psk string
	_ = s.DB.QueryRow(`SELECT COALESCE(ipsec_psk,'') FROM vpn_core_settings WHERE id=1`).Scan(&psk)
	psk = strings.TrimSpace(psk)
	uuidPayload := strings.ToLower(auth.RandomToken(8) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(12))
	uuidProfile := strings.ToLower(auth.RandomToken(8) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(12))
	pskData := base64.StdEncoding.EncodeToString([]byte(psk))
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadDescription</key>
			<string>Configures L2TP VPN</string>
			<key>PayloadDisplayName</key>
			<string>Koris L2TP</string>
			<key>PayloadIdentifier</key>
			<string>koris.vpn.l2tp.%s</string>
			<key>PayloadType</key>
			<string>com.apple.vpn.managed</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>UserDefinedName</key>
			<string>Koris L2TP</string>
			<key>VPNType</key>
			<string>L2TP</string>
			<key>IPv4</key>
			<dict>
				<key>OverridePrimary</key>
				<integer>1</integer>
			</dict>
			<key>PPP</key>
			<dict>
				<key>AuthName</key>
				<string>%s</string>
				<key>CommRemoteAddress</key>
				<string>%s</string>
				<key>OnDemandEnabled</key>
				<integer>0</integer>
			</dict>
			<key>IPSec</key>
			<dict>
				<key>AuthenticationMethod</key>
				<string>SharedSecret</string>
				<key>SharedSecret</key>
				<data>%s</data>
			</dict>
		</dict>
	</array>
	<key>PayloadDisplayName</key>
	<string>Koris L2TP</string>
	<key>PayloadIdentifier</key>
	<string>koris.vpn.l2tp.profile.%s</string>
	<key>PayloadRemovalDisallowed</key>
	<false/>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>%s</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>`, username, uuidPayload, username, host, pskData, username, uuidProfile)
}
