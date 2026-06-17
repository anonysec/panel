package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// OutboundConfig represents the outbound proxy configuration stored in extra_json.
type OutboundConfig struct {
	Enabled bool   `json:"enabled"`
	Type    string `json:"type"`    // vless, vmess, trojan, shadowsocks, socks5
	Address string `json:"address"` // server:port
	UUID    string `json:"uuid"`    // UUID or password
	TLS     bool   `json:"tls"`
	Path    string `json:"path"` // WebSocket path
	SNI     string `json:"sni"`  // SNI hostname
}

// parseOutboundConfig extracts outbound config from the extra_json blob.
// Returns nil if outbound is not configured or not enabled.
func parseOutboundConfig(extraJSON json.RawMessage) *OutboundConfig {
	if len(extraJSON) == 0 {
		return nil
	}
	var extra map[string]json.RawMessage
	if err := json.Unmarshal(extraJSON, &extra); err != nil {
		return nil
	}
	raw, ok := extra["outbound"]
	if !ok || len(raw) == 0 {
		return nil
	}
	var cfg OutboundConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil
	}
	if !cfg.Enabled || cfg.Address == "" {
		return nil
	}
	return &cfg
}

// applyOutboundOpenVPN generates OpenVPN directives for routing through outbound proxy.
// It writes a socks-proxy directive that points to a local SOCKS5 bridge.
func applyOutboundOpenVPN(cfg *OutboundConfig, configDir string) ([]string, error) {
	if cfg == nil {
		return nil, nil
	}

	directives := []string{
		"# Outbound proxy configuration (auto-generated)",
	}

	switch cfg.Type {
	case "socks5":
		// Direct SOCKS5 proxy support in OpenVPN
		parts := strings.SplitN(cfg.Address, ":", 2)
		host := parts[0]
		port := "1080"
		if len(parts) == 2 {
			port = parts[1]
		}
		directives = append(directives, fmt.Sprintf("socks-proxy %s %s", host, port))
		if cfg.UUID != "" {
			// Write auth file for socks-proxy
			authFile := filepath.Join(configDir, "outbound-socks-auth.txt")
			authContent := fmt.Sprintf("%s\n%s\n", cfg.UUID, "")
			if err := os.WriteFile(authFile, []byte(authContent), 0600); err != nil {
				return nil, fmt.Errorf("failed to write socks auth file: %w", err)
			}
			directives = append(directives, fmt.Sprintf("socks-proxy-retry\n<socks-proxy> %s %s %s", host, port, authFile))
		}
	case "vless", "vmess", "trojan", "shadowsocks":
		// For these protocols, we configure a local SOCKS5 bridge that the outbound
		// proxy client (xray/sing-box) provides. The bridge listens on 127.0.0.1:10800.
		directives = append(directives,
			"socks-proxy 127.0.0.1 10800",
			"socks-proxy-retry",
		)
		// Write outbound proxy config for the bridge
		if err := writeOutboundBridgeConfig(cfg, configDir); err != nil {
			return nil, err
		}
	}

	return directives, nil
}

// applyOutboundIKEv2 generates a leftupdown script for strongswan that routes
// through the outbound proxy.
func applyOutboundIKEv2(cfg *OutboundConfig, configDir string) error {
	if cfg == nil {
		return nil
	}

	// Create a routing script that sets up traffic to flow through the outbound proxy
	scriptPath := filepath.Join(configDir, "outbound-updown.sh")
	proxyHost := strings.SplitN(cfg.Address, ":", 2)[0]

	script := fmt.Sprintf(`#!/bin/bash
# Outbound routing script for IKEv2/strongSwan (auto-generated)
# Routes VPN traffic through the outbound proxy server

PROXY_HOST="%s"

case "$PLUTO_VERB" in
  up-client)
    # Add route to proxy server via default gateway
    DEFAULT_GW=$(ip route show default | awk '{print $3}' | head -1)
    DEFAULT_IF=$(ip route show default | awk '{print $5}' | head -1)
    if [ -n "$DEFAULT_GW" ] && [ -n "$DEFAULT_IF" ]; then
      ip route add "$PROXY_HOST/32" via "$DEFAULT_GW" dev "$DEFAULT_IF" 2>/dev/null || true
    fi
    ;;
  down-client)
    ip route del "$PROXY_HOST/32" 2>/dev/null || true
    ;;
esac
`, proxyHost)

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write IKEv2 outbound script: %w", err)
	}

	// Write bridge config for non-socks5 types
	if cfg.Type != "socks5" {
		if err := writeOutboundBridgeConfig(cfg, configDir); err != nil {
			return err
		}
	}

	return nil
}

// applyOutboundSSH generates a ProxyCommand directive for SSH tunneling through
// the outbound proxy.
func applyOutboundSSH(cfg *OutboundConfig, configDir string) (string, error) {
	if cfg == nil {
		return "", nil
	}

	switch cfg.Type {
	case "socks5":
		// Use netcat through SOCKS5 proxy
		parts := strings.SplitN(cfg.Address, ":", 2)
		host := parts[0]
		port := "1080"
		if len(parts) == 2 {
			port = parts[1]
		}
		return fmt.Sprintf("ProxyCommand /usr/bin/nc -X 5 -x %s:%s %%h %%p", host, port), nil
	case "vless", "vmess", "trojan", "shadowsocks":
		// Route through local SOCKS5 bridge (xray/sing-box)
		if err := writeOutboundBridgeConfig(cfg, configDir); err != nil {
			return "", err
		}
		return "ProxyCommand /usr/bin/nc -X 5 -x 127.0.0.1:10800 %h %p", nil
	}

	return "", nil
}

// writeOutboundBridgeConfig writes a JSON configuration file for the local
// outbound proxy bridge (compatible with xray-core/sing-box format).
// The bridge provides a local SOCKS5 endpoint at 127.0.0.1:10800 that
// forwards traffic through the configured outbound protocol.
func writeOutboundBridgeConfig(cfg *OutboundConfig, configDir string) error {
	bridgeConfig := map[string]any{
		"inbounds": []map[string]any{
			{
				"type":   "socks",
				"listen": "127.0.0.1",
				"port":   10800,
			},
		},
		"outbounds": []map[string]any{
			buildOutboundEntry(cfg),
		},
	}

	data, err := json.MarshalIndent(bridgeConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bridge config: %w", err)
	}

	configPath := filepath.Join(configDir, "outbound-bridge.json")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write bridge config: %w", err)
	}

	return nil
}

// buildOutboundEntry creates the outbound protocol entry for the bridge config.
func buildOutboundEntry(cfg *OutboundConfig) map[string]any {
	parts := strings.SplitN(cfg.Address, ":", 2)
	host := parts[0]
	port := 443
	if len(parts) == 2 {
		if p, err := strconv.Atoi(parts[1]); err == nil {
			port = p
		}
	}

	entry := map[string]any{
		"type":   cfg.Type,
		"server": host,
		"port":   port,
	}

	switch cfg.Type {
	case "vless":
		entry["uuid"] = cfg.UUID
		entry["flow"] = ""
		if cfg.TLS {
			entry["tls"] = map[string]any{"enabled": true, "server_name": cfg.SNI}
		}
		if cfg.Path != "" {
			entry["transport"] = map[string]any{"type": "ws", "path": cfg.Path}
		}
	case "vmess":
		entry["uuid"] = cfg.UUID
		entry["security"] = "auto"
		if cfg.TLS {
			entry["tls"] = map[string]any{"enabled": true, "server_name": cfg.SNI}
		}
		if cfg.Path != "" {
			entry["transport"] = map[string]any{"type": "ws", "path": cfg.Path}
		}
	case "trojan":
		entry["password"] = cfg.UUID
		if cfg.TLS {
			entry["tls"] = map[string]any{"enabled": true, "server_name": cfg.SNI}
		}
		if cfg.Path != "" {
			entry["transport"] = map[string]any{"type": "ws", "path": cfg.Path}
		}
	case "shadowsocks":
		entry["password"] = cfg.UUID
		entry["method"] = "aes-256-gcm"
	case "socks5":
		if cfg.UUID != "" {
			entry["username"] = cfg.UUID
			entry["password"] = ""
		}
	}

	return entry
}
