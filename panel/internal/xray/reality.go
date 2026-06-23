//go:build !lite

package xray

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// RealityInboundParams holds all parameters needed to build a VLESS+Reality inbound.
type RealityInboundParams struct {
	Port        int      `json:"port"`
	ServerNames []string `json:"server_names"`
	PrivateKey  string   `json:"private_key"`
	PublicKey   string   `json:"public_key"`
	ShortIDs    []string `json:"short_ids"`
	Flow        string   `json:"flow"` // default: "xtls-rprx-vision"
	Dest        string   `json:"dest"` // fallback destination like "www.google.com:443"
}

// realityInboundJSON represents the full Xray-compatible JSON structure for a VLESS+Reality inbound.
type realityInboundJSON struct {
	Listen         string                 `json:"listen"`
	Port           int                    `json:"port"`
	Protocol       string                 `json:"protocol"`
	Settings       realityInboundSettings `json:"settings"`
	StreamSettings realityStreamSettings  `json:"streamSettings"`
	Tag            string                 `json:"tag"`
}

type realityInboundSettings struct {
	Clients    []interface{} `json:"clients"`
	Decryption string        `json:"decryption"`
}

type realityStreamSettings struct {
	Network         string                `json:"network"`
	Security        string                `json:"security"`
	RealitySettings realitySettingsConfig `json:"realitySettings"`
}

type realitySettingsConfig struct {
	Dest        string   `json:"dest"`
	ServerNames []string `json:"serverNames"`
	PrivateKey  string   `json:"privateKey"`
	ShortIDs    []string `json:"shortIds"`
}

// GenerateRealityKeyPair generates an X25519 key pair for Reality protocol.
// Returns base64url-encoded (no padding) private and public keys.
func GenerateRealityKeyPair() (privateKey, publicKey string, err error) {
	// Generate 32 random bytes for private key seed.
	var seed [32]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return "", "", fmt.Errorf("generate random seed: %w", err)
	}

	// Clamp the private key per X25519 spec (curve25519.ScalarBaseMult expects clamped input).
	seed[0] &= 248
	seed[31] &= 127
	seed[31] |= 64

	// Derive public key from private key.
	pub, err := curve25519.X25519(seed[:], curve25519.Basepoint)
	if err != nil {
		return "", "", fmt.Errorf("derive public key: %w", err)
	}

	// Encode as hex (Xray uses base64url for keys, but hex is also common).
	// Xray Reality actually uses raw base64url-no-padding encoding.
	privateKey = encodeBase64URL(seed[:])
	publicKey = encodeBase64URL(pub)

	return privateKey, publicKey, nil
}

// encodeBase64URL encodes bytes to base64url without padding (matching Xray's format).
func encodeBase64URL(data []byte) string {
	const base64URLChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	result := make([]byte, 0, (len(data)*4+2)/3)

	for i := 0; i < len(data); i += 3 {
		var val uint32
		remaining := len(data) - i

		switch {
		case remaining >= 3:
			val = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
			result = append(result,
				base64URLChars[val>>18&0x3F],
				base64URLChars[val>>12&0x3F],
				base64URLChars[val>>6&0x3F],
				base64URLChars[val&0x3F],
			)
		case remaining == 2:
			val = uint32(data[i])<<16 | uint32(data[i+1])<<8
			result = append(result,
				base64URLChars[val>>18&0x3F],
				base64URLChars[val>>12&0x3F],
				base64URLChars[val>>6&0x3F],
			)
		case remaining == 1:
			val = uint32(data[i]) << 16
			result = append(result,
				base64URLChars[val>>18&0x3F],
				base64URLChars[val>>12&0x3F],
			)
		}
	}

	return string(result)
}

// GenerateShortID generates a random 8-character hex short ID for Reality.
func GenerateShortID() string {
	b := make([]byte, 4) // 4 bytes = 8 hex characters
	rand.Read(b)
	return hex.EncodeToString(b)
}

// BuildRealityInbound constructs a VLESS+Reality inbound config ready for Xray.
// The returned Inbound has its Settings field populated with the full JSON structure.
func BuildRealityInbound(params RealityInboundParams) (Inbound, error) {
	if params.Port <= 0 {
		return Inbound{}, fmt.Errorf("port must be positive, got %d", params.Port)
	}
	if len(params.ServerNames) == 0 {
		return Inbound{}, fmt.Errorf("at least one server name is required")
	}
	if params.PrivateKey == "" {
		return Inbound{}, fmt.Errorf("private key is required")
	}
	if params.Dest == "" {
		// Default to first server name with port 443.
		params.Dest = params.ServerNames[0] + ":443"
	}
	if params.Flow == "" {
		params.Flow = "xtls-rprx-vision"
	}
	if len(params.ShortIDs) == 0 {
		return Inbound{}, fmt.Errorf("at least one short ID is required")
	}

	// Build the full Xray-compatible JSON structure.
	inboundJSON := realityInboundJSON{
		Listen:   "0.0.0.0",
		Port:     params.Port,
		Protocol: ProtocolVLESS,
		Settings: realityInboundSettings{
			Clients:    []interface{}{},
			Decryption: "none",
		},
		StreamSettings: realityStreamSettings{
			Network:  TransportTCP,
			Security: "reality",
			RealitySettings: realitySettingsConfig{
				Dest:        params.Dest,
				ServerNames: params.ServerNames,
				PrivateKey:  params.PrivateKey,
				ShortIDs:    params.ShortIDs,
			},
		},
		Tag: "vless-reality",
	}

	settingsJSON, err := json.Marshal(inboundJSON)
	if err != nil {
		return Inbound{}, fmt.Errorf("marshal reality inbound: %w", err)
	}

	return Inbound{
		Protocol:  ProtocolVLESS,
		Port:      params.Port,
		Transport: TransportTCP,
		Tag:       "vless-reality",
		Settings:  settingsJSON,
	}, nil
}

// SetupReality configures VLESS+Reality on a node. It generates keys and short IDs
// if not provided, saves the Reality config to the DB, and adds the inbound to
// the node's Xray configuration.
func (s *XrayService) SetupReality(ctx context.Context, nodeID int64, params RealityInboundParams) error {
	// Generate key pair if not provided.
	if params.PrivateKey == "" || params.PublicKey == "" {
		privKey, pubKey, err := GenerateRealityKeyPair()
		if err != nil {
			return fmt.Errorf("generate reality key pair: %w", err)
		}
		params.PrivateKey = privKey
		params.PublicKey = pubKey
	}

	// Generate short IDs if not provided (default: 3 random short IDs).
	if len(params.ShortIDs) == 0 {
		params.ShortIDs = make([]string, 3)
		for i := range params.ShortIDs {
			params.ShortIDs[i] = GenerateShortID()
		}
	}

	// Default flow.
	if params.Flow == "" {
		params.Flow = "xtls-rprx-vision"
	}

	// Default port.
	if params.Port <= 0 {
		params.Port = 443
	}

	// Build the inbound config.
	inbound, err := BuildRealityInbound(params)
	if err != nil {
		return fmt.Errorf("build reality inbound: %w", err)
	}

	// Get existing config or create a new one.
	cfg, err := s.GetConfig(ctx, nodeID)
	if err != nil {
		// Config doesn't exist yet — create a new one.
		cfg = &XrayConfig{
			NodeID:  nodeID,
			Enabled: true,
		}
	}

	// Update or add the VLESS+Reality inbound (replace existing if tag matches).
	found := false
	for i, ib := range cfg.Inbounds {
		if ib.Tag == "vless-reality" {
			cfg.Inbounds[i] = inbound
			found = true
			break
		}
	}
	if !found {
		cfg.Inbounds = append(cfg.Inbounds, inbound)
	}

	// Set the Reality config (public info for share links).
	cfg.RealityConfig = &RealityConfig{
		ServerNames: params.ServerNames,
		PrivateKey:  params.PrivateKey,
		PublicKey:   params.PublicKey,
		ShortIDs:    params.ShortIDs,
	}

	// Save to database.
	if err := s.SaveConfig(ctx, cfg); err != nil {
		return fmt.Errorf("save reality config: %w", err)
	}

	s.notify(fmt.Sprintf("setup reality on node %d with server names %v", nodeID, params.ServerNames))
	return nil
}

// GetRealityPublicKey returns the public key for a node's Reality config.
// This is the key shared with clients (included in share links via the `pbk` parameter).
func (s *XrayService) GetRealityPublicKey(ctx context.Context, nodeID int64) (string, error) {
	var realityJSON *string

	err := s.db.QueryRowContext(ctx,
		`SELECT reality_config_json FROM xray_configs WHERE node_id = ?`, nodeID,
	).Scan(&realityJSON)
	if err != nil {
		return "", fmt.Errorf("query reality config for node %d: %w", nodeID, err)
	}

	if realityJSON == nil || *realityJSON == "" {
		return "", fmt.Errorf("reality not configured for node %d", nodeID)
	}

	var rc RealityConfig
	if err := json.Unmarshal([]byte(*realityJSON), &rc); err != nil {
		return "", fmt.Errorf("unmarshal reality config: %w", err)
	}

	if rc.PublicKey == "" {
		return "", fmt.Errorf("reality public key is empty for node %d", nodeID)
	}

	return rc.PublicKey, nil
}
