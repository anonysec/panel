//go:build !lite

package xray

import (
	"context"
	"encoding/json"
	"fmt"
)

// Default internal listener ports for fallback targets.
const (
	DefaultVMessWSPort  = 31001
	DefaultTrojanWSPort = 31002
	DefaultHTTP2Port    = 31003
	DefaultHTTPFallback = 80
)

// FallbackConfig represents a single fallback entry in a VLESS fallback chain.
// Traffic matching the Path or ALPN criteria is forwarded to Dest.
type FallbackConfig struct {
	Dest string `json:"dest"`           // Destination address (port number or address:port)
	Path string `json:"path,omitempty"` // WebSocket path to match (e.g. "/vmess-ws")
	ALPN string `json:"alpn,omitempty"` // ALPN to match (e.g. "h2")
	Xver int    `json:"xver"`           // PROXY protocol version (0=none, 1=v1, 2=v2)
}

// MultiPortConfig defines a single-port multi-protocol setup using VLESS fallback chains.
// The main VLESS inbound listens on Port and routes traffic to other protocols via fallbacks.
type MultiPortConfig struct {
	Port         int              `json:"port"`                    // Main listening port (typically 443)
	Fallbacks    []FallbackConfig `json:"fallbacks"`               // Fallback routing entries
	TLS          *TLSConfig       `json:"tls,omitempty"`           // TLS settings (mutually exclusive with Reality)
	Reality      *RealityConfig   `json:"reality,omitempty"`       // Reality settings (mutually exclusive with TLS)
	MainProtocol string           `json:"main_protocol,omitempty"` // Main protocol, default "vless"
}

// fallbackInboundJSON represents the full Xray-compatible main inbound with fallbacks.
type fallbackInboundJSON struct {
	Listen         string                  `json:"listen"`
	Port           int                     `json:"port"`
	Protocol       string                  `json:"protocol"`
	Settings       fallbackInboundSettings `json:"settings"`
	StreamSettings json.RawMessage         `json:"streamSettings"`
	Tag            string                  `json:"tag"`
}

type fallbackInboundSettings struct {
	Clients    []any               `json:"clients"`
	Decryption string              `json:"decryption"`
	Fallbacks  []fallbackEntryJSON `json:"fallbacks"`
}

// fallbackEntryJSON is the Xray JSON representation of a single fallback entry.
type fallbackEntryJSON struct {
	Dest interface{} `json:"dest"`           // int (port) or string (addr:port)
	Path string      `json:"path,omitempty"` // WebSocket path
	ALPN string      `json:"alpn,omitempty"` // ALPN protocol
	Xver int         `json:"xver,omitempty"` // PROXY protocol version
}

// helperInboundJSON represents an internal listener for a fallback target protocol.
type helperInboundJSON struct {
	Listen         string          `json:"listen"`
	Port           int             `json:"port"`
	Protocol       string          `json:"protocol"`
	Settings       json.RawMessage `json:"settings"`
	StreamSettings json.RawMessage `json:"streamSettings"`
	Tag            string          `json:"tag"`
}

// BuildFallbackChain generates the complete Xray inbound JSON array with fallbacks.
// It produces the main VLESS inbound with a fallback chain, plus helper inbounds
// for each path-based fallback target (internal listeners on 127.0.0.1).
func BuildFallbackChain(config MultiPortConfig) (json.RawMessage, error) {
	if config.Port <= 0 {
		return nil, fmt.Errorf("port must be positive, got %d", config.Port)
	}
	if len(config.Fallbacks) == 0 {
		return nil, fmt.Errorf("at least one fallback entry is required")
	}
	if config.MainProtocol == "" {
		config.MainProtocol = ProtocolVLESS
	}

	// Build stream settings based on TLS or Reality config.
	streamSettings, err := buildFallbackStreamSettings(config)
	if err != nil {
		return nil, fmt.Errorf("build stream settings: %w", err)
	}

	// Build fallback entries for Xray JSON format.
	fallbackEntries := make([]fallbackEntryJSON, 0, len(config.Fallbacks))
	for _, fb := range config.Fallbacks {
		entry := fallbackEntryJSON{
			Dest: parseFallbackDest(fb.Dest),
			Path: fb.Path,
			ALPN: fb.ALPN,
			Xver: fb.Xver,
		}
		fallbackEntries = append(fallbackEntries, entry)
	}

	// Build the main VLESS inbound with fallbacks.
	mainInbound := fallbackInboundJSON{
		Listen:   "0.0.0.0",
		Port:     config.Port,
		Protocol: config.MainProtocol,
		Settings: fallbackInboundSettings{
			Clients:    []any{},
			Decryption: "none",
			Fallbacks:  fallbackEntries,
		},
		StreamSettings: streamSettings,
		Tag:            "main-fallback",
	}

	// Serialize the main inbound.
	mainJSON, err := json.Marshal(mainInbound)
	if err != nil {
		return nil, fmt.Errorf("marshal main inbound: %w", err)
	}

	// Build helper inbounds for path-based fallbacks.
	helpers, err := buildHelperInbounds(config.Fallbacks)
	if err != nil {
		return nil, fmt.Errorf("build helper inbounds: %w", err)
	}

	// Combine into a JSON array of all inbounds.
	allInbounds := make([]json.RawMessage, 0, 1+len(helpers))
	allInbounds = append(allInbounds, mainJSON)
	for _, h := range helpers {
		hJSON, err := json.Marshal(h)
		if err != nil {
			return nil, fmt.Errorf("marshal helper inbound: %w", err)
		}
		allInbounds = append(allInbounds, hJSON)
	}

	result, err := json.Marshal(allInbounds)
	if err != nil {
		return nil, fmt.Errorf("marshal inbounds array: %w", err)
	}

	return result, nil
}

// buildFallbackStreamSettings creates the streamSettings JSON for the main fallback inbound.
func buildFallbackStreamSettings(config MultiPortConfig) (json.RawMessage, error) {
	stream := map[string]any{
		"network": TransportTCP,
	}

	if config.Reality != nil {
		stream["security"] = "reality"
		stream["realitySettings"] = map[string]any{
			"serverNames": config.Reality.ServerNames,
			"privateKey":  config.Reality.PrivateKey,
			"shortIds":    config.Reality.ShortIDs,
		}
	} else if config.TLS != nil {
		stream["security"] = "tls"
		tlsSettings := map[string]any{}
		if config.TLS.CertPath != "" {
			tlsSettings["certificates"] = []map[string]string{
				{
					"certificateFile": config.TLS.CertPath,
					"keyFile":         config.TLS.KeyPath,
				},
			}
		}
		if config.TLS.ServerName != "" {
			tlsSettings["serverName"] = config.TLS.ServerName
		}
		if len(config.TLS.ALPN) > 0 {
			tlsSettings["alpn"] = config.TLS.ALPN
		}
		stream["tlsSettings"] = tlsSettings
	} else {
		stream["security"] = "none"
	}

	return json.Marshal(stream)
}

// buildHelperInbounds creates internal listener inbounds for path-based or ALPN-based fallback targets.
// Only fallbacks with a Path get a helper inbound (they need an internal WS listener).
func buildHelperInbounds(fallbacks []FallbackConfig) ([]helperInboundJSON, error) {
	var helpers []helperInboundJSON

	for _, fb := range fallbacks {
		if fb.Path == "" {
			continue // No helper needed for non-path fallbacks (default fallback or ALPN-only).
		}

		port := parseFallbackDestPort(fb.Dest)
		if port <= 0 {
			continue
		}

		// Determine protocol from path (common naming conventions).
		protocol, tag := inferProtocolFromPath(fb.Path)

		settings, err := buildHelperSettings(protocol)
		if err != nil {
			return nil, fmt.Errorf("build helper settings for %s: %w", tag, err)
		}

		streamSettings, err := buildHelperStreamSettings(fb.Path)
		if err != nil {
			return nil, fmt.Errorf("build helper stream settings for %s: %w", tag, err)
		}

		helpers = append(helpers, helperInboundJSON{
			Listen:         "127.0.0.1",
			Port:           port,
			Protocol:       protocol,
			Settings:       settings,
			StreamSettings: streamSettings,
			Tag:            tag,
		})
	}

	return helpers, nil
}

// buildHelperSettings creates the settings JSON for a helper inbound.
func buildHelperSettings(protocol string) (json.RawMessage, error) {
	switch protocol {
	case ProtocolVMess:
		return json.Marshal(map[string]any{
			"clients": []any{},
		})
	case ProtocolTrojan:
		return json.Marshal(map[string]any{
			"clients": []any{},
		})
	case ProtocolVLESS:
		return json.Marshal(map[string]any{
			"clients":    []any{},
			"decryption": "none",
		})
	default:
		return json.Marshal(map[string]any{
			"clients": []any{},
		})
	}
}

// buildHelperStreamSettings creates the streamSettings JSON for a helper WebSocket inbound.
func buildHelperStreamSettings(path string) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"network": TransportWS,
		"wsSettings": map[string]any{
			"path": path,
		},
	})
}

// inferProtocolFromPath guesses the protocol and tag from a WebSocket path.
func inferProtocolFromPath(path string) (protocol, tag string) {
	switch {
	case contains(path, "vmess"):
		return ProtocolVMess, "vmess-ws-in"
	case contains(path, "trojan"):
		return ProtocolTrojan, "trojan-ws-in"
	case contains(path, "vless"):
		return ProtocolVLESS, "vless-ws-in"
	case contains(path, "ss"), contains(path, "shadowsocks"):
		return ProtocolShadowsocks, "ss-ws-in"
	default:
		return ProtocolVMess, "fallback-ws-in"
	}
}

// contains checks if s contains substr (case-insensitive simple check).
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := range substr {
			sc := s[i+j]
			tc := substr[j]
			// Simple ASCII lowercase comparison.
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if tc >= 'A' && tc <= 'Z' {
				tc += 32
			}
			if sc != tc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// parseFallbackDest converts a destination string to the appropriate type.
// If it's a pure number, it returns an int (port). Otherwise returns the string.
func parseFallbackDest(dest string) interface{} {
	port := 0
	for _, c := range dest {
		if c < '0' || c > '9' {
			return dest // Not a pure number, return as string.
		}
		port = port*10 + int(c-'0')
	}
	if port > 0 {
		return port
	}
	return dest
}

// parseFallbackDestPort extracts the port number from a destination string.
// Returns the port as int, or 0 if the dest is not a numeric port.
func parseFallbackDestPort(dest string) int {
	port := 0
	for _, c := range dest {
		if c < '0' || c > '9' {
			return 0
		}
		port = port*10 + int(c-'0')
	}
	return port
}

// SetupMultiPort configures a multi-protocol single-port setup for a node.
// It saves the MultiPortConfig to the node's Xray configuration and generates
// the appropriate inbound entries.
func (s *XrayService) SetupMultiPort(ctx context.Context, nodeID int64, config MultiPortConfig) error {
	if config.Port <= 0 {
		return fmt.Errorf("port must be positive, got %d", config.Port)
	}
	if len(config.Fallbacks) == 0 {
		return fmt.Errorf("at least one fallback entry is required")
	}
	if config.MainProtocol == "" {
		config.MainProtocol = ProtocolVLESS
	}

	// Build the fallback chain JSON.
	inboundsJSON, err := BuildFallbackChain(config)
	if err != nil {
		return fmt.Errorf("build fallback chain: %w", err)
	}

	// Get or create the node's Xray config.
	cfg, err := s.GetConfig(ctx, nodeID)
	if err != nil {
		// Config doesn't exist — create a new one.
		cfg = &XrayConfig{
			NodeID:  nodeID,
			Enabled: true,
		}
	}

	// Parse the generated inbounds and update the config.
	var inbounds []json.RawMessage
	if err := json.Unmarshal(inboundsJSON, &inbounds); err != nil {
		return fmt.Errorf("parse generated inbounds: %w", err)
	}

	// Convert raw JSON inbounds to Inbound structs for storage.
	newInbounds := make([]Inbound, 0, len(inbounds))
	for _, raw := range inbounds {
		var parsed struct {
			Port     int    `json:"port"`
			Protocol string `json:"protocol"`
			Tag      string `json:"tag"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return fmt.Errorf("parse inbound entry: %w", err)
		}

		transport := TransportTCP
		// Check if it's a WS helper inbound.
		var streamCheck struct {
			StreamSettings struct {
				Network string `json:"network"`
			} `json:"streamSettings"`
		}
		if json.Unmarshal(raw, &streamCheck) == nil && streamCheck.StreamSettings.Network != "" {
			transport = streamCheck.StreamSettings.Network
		}

		newInbounds = append(newInbounds, Inbound{
			Protocol:  parsed.Protocol,
			Port:      parsed.Port,
			Transport: transport,
			Tag:       parsed.Tag,
			Settings:  raw,
		})
	}

	// Replace existing fallback-related inbounds (remove old ones with matching tags).
	filtered := make([]Inbound, 0, len(cfg.Inbounds))
	for _, ib := range cfg.Inbounds {
		if ib.Tag == "main-fallback" || hasSuffix(ib.Tag, "-ws-in") {
			continue // Remove old fallback inbounds.
		}
		filtered = append(filtered, ib)
	}
	cfg.Inbounds = append(filtered, newInbounds...)

	// Apply TLS or Reality config to the node config.
	if config.TLS != nil {
		cfg.TLS = *config.TLS
	}
	if config.Reality != nil {
		cfg.RealityConfig = config.Reality
	}

	// Save to database.
	if err := s.SaveConfig(ctx, cfg); err != nil {
		return fmt.Errorf("save multi-port config: %w", err)
	}

	s.notify(fmt.Sprintf("setup multi-port fallback on node %d (port %d, %d fallbacks)",
		nodeID, config.Port, len(config.Fallbacks)))
	return nil
}

// GenerateDefaultFallbackConfig creates a sensible default multi-port configuration
// for common protocol combinations. VLESS listens on the main port with fallbacks
// to VMess-WS and Trojan-WS on internal ports.
func GenerateDefaultFallbackConfig(protocols []string) MultiPortConfig {
	config := MultiPortConfig{
		Port:         443,
		MainProtocol: ProtocolVLESS,
		Fallbacks:    make([]FallbackConfig, 0, len(protocols)+1),
	}

	nextPort := DefaultVMessWSPort

	for _, proto := range protocols {
		switch proto {
		case ProtocolVMess:
			config.Fallbacks = append(config.Fallbacks, FallbackConfig{
				Dest: fmt.Sprintf("%d", nextPort),
				Path: "/vmess-ws",
				Xver: 1,
			})
			nextPort++
		case ProtocolTrojan:
			config.Fallbacks = append(config.Fallbacks, FallbackConfig{
				Dest: fmt.Sprintf("%d", nextPort),
				Path: "/trojan-ws",
				Xver: 1,
			})
			nextPort++
		case ProtocolShadowsocks:
			config.Fallbacks = append(config.Fallbacks, FallbackConfig{
				Dest: fmt.Sprintf("%d", nextPort),
				Path: "/ss-ws",
				Xver: 1,
			})
			nextPort++
		case ProtocolVLESS:
			// VLESS is the main protocol — skip adding as a fallback target.
			continue
		}
	}

	// Always add a default fallback (catch-all) that routes to HTTP port 80.
	config.Fallbacks = append(config.Fallbacks, FallbackConfig{
		Dest: fmt.Sprintf("%d", DefaultHTTPFallback),
		Xver: 0,
	})

	return config
}

// hasSuffix checks if s ends with suffix.
func hasSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
