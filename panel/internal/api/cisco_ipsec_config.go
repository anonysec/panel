package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"KorisPanel/panel/internal/auth"
)

// customerCiscoIPSecConfig generates and returns an Apple .mobileconfig profile
// for Cisco IPSec (IKEv1 + XAUTH) VPN connections.
// GET /api/customer/configs/cisco-ipsec
func (s *Server) customerCiscoIPSecConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	// Get customer's preferred node (or first online node)
	var nodeID int64
	_ = s.DB.QueryRow(`SELECT COALESCE(preferred_node_id, 0) FROM customers WHERE username=$1 AND deleted_at IS NULL`, username).Scan(&nodeID)

	// Resolve node host (public IP or domain)
	host, _, _, _ := s.openVPNEndpointNode(r, nodeID)
	if host == "" {
		host = r.Host
		if strings.Contains(host, ":") {
			host = strings.Split(host, ":")[0]
		}
	}

	// Get Cisco IPSec PSK: first try per-node config, then fall back to global ipsec_psk
	psk := s.ciscoIPSecPSK(nodeID)

	// Get customer's password from radcheck
	var password string
	_ = s.DB.QueryRow(`SELECT value FROM radcheck WHERE username=$1 AND attribute IN('Cleartext-Password','User-Password') ORDER BY id DESC LIMIT 1`, username).Scan(&password)

	// Generate UUIDs for the profile
	uuidPayload := generateUUID()
	uuidProfile := generateUUID()

	// Base64 encode the PSK
	pskData := base64.StdEncoding.EncodeToString([]byte(psk))

	// Build the .mobileconfig XML
	profile := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadDescription</key>
			<string>Configures Cisco IPSec VPN</string>
			<key>PayloadDisplayName</key>
			<string>KorisPanel VPN</string>
			<key>PayloadIdentifier</key>
			<string>com.korispanel.vpn.cisco-ipsec.%s</string>
			<key>PayloadType</key>
			<string>com.apple.vpn.managed</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>UserDefinedName</key>
			<string>KorisPanel VPN</string>
			<key>VPNType</key>
			<string>IPSec</string>
			<key>IPSec</key>
			<dict>
				<key>AuthenticationMethod</key>
				<string>SharedSecret</string>
				<key>LocalIdentifierType</key>
				<string>KeyID</string>
				<key>RemoteAddress</key>
				<string>%s</string>
				<key>SharedSecret</key>
				<data>%s</data>
				<key>XAuthEnabled</key>
				<integer>1</integer>
				<key>XAuthName</key>
				<string>%s</string>
				<key>XAuthPassword</key>
				<string>%s</string>
			</dict>
		</dict>
	</array>
	<key>PayloadDisplayName</key>
	<string>KorisPanel VPN</string>
	<key>PayloadIdentifier</key>
	<string>com.korispanel.vpn.cisco-ipsec.profile.%s</string>
	<key>PayloadRemovalDisallowed</key>
	<false/>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>%s</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>`, username, uuidPayload, host, pskData, username, password, username, uuidProfile)

	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="koris-vpn.mobileconfig"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(profile))
}

// ciscoIPSecPSK returns the Cisco IPSec PSK for a given node.
// It checks the per-node config first (node_vpn_configs.extra_json -> psk),
// then falls back to the global ipsec_psk in vpn_core_settings.
func (s *Server) ciscoIPSecPSK(nodeID int64) string {
	// Try per-node Cisco IPSec config
	if nodeID > 0 {
		var extraJSON []byte
		err := s.DB.QueryRow(`SELECT extra_json FROM node_vpn_configs WHERE node_id=$1 AND protocol='cisco_ipsec' AND enabled=TRUE LIMIT 1`, nodeID).Scan(&extraJSON)
		if err == nil && len(extraJSON) > 0 {
			var extra struct {
				PSK string `json:"psk"`
			}
			if json.Unmarshal(extraJSON, &extra) == nil && extra.PSK != "" {
				return extra.PSK
			}
		}
	}

	// Fall back to global PSK
	var psk string
	_ = s.DB.QueryRow(`SELECT COALESCE(ipsec_psk,'') FROM vpn_core_settings WHERE id=1`).Scan(&psk)
	return strings.TrimSpace(psk)
}

// generateUUID creates a random UUID-like string for Apple profile payloads.
func generateUUID() string {
	return strings.ToLower(
		auth.RandomToken(4) + "-" +
			auth.RandomToken(2) + "-" +
			auth.RandomToken(2) + "-" +
			auth.RandomToken(2) + "-" +
			auth.RandomToken(6))
}
