package wireguard

import (
	"fmt"
	"strings"
)

// ClientConfig holds the information needed to generate a WireGuard client configuration file.
type ClientConfig struct {
	PrivateKey      string
	Address         string
	DNS             string
	ServerPublicKey string
	PresharedKey    string
	Endpoint        string
	GamingOptimize  bool
	MTU             int
}

// GenerateClientConfig produces a complete WireGuard .conf file string
// for a client peer, given the client's private key, address (allowed_ips),
// DNS servers, the server's public key, preshared key, and server endpoint.
//
// When GamingOptimize is true, MTU is set to 1280 and PersistentKeepalive to 15.
// When GamingOptimize is false, default PersistentKeepalive is 25 and MTU is
// only included if explicitly set (MTU > 0).
func GenerateClientConfig(cfg ClientConfig) string {
	var b strings.Builder

	b.WriteString("[Interface]\n")
	b.WriteString(fmt.Sprintf("PrivateKey = %s\n", cfg.PrivateKey))
	b.WriteString(fmt.Sprintf("Address = %s\n", cfg.Address))
	b.WriteString(fmt.Sprintf("DNS = %s\n", cfg.DNS))

	// Determine MTU and keepalive based on gaming optimize
	mtu := cfg.MTU
	keepalive := 25
	if cfg.GamingOptimize {
		mtu = 1280
		keepalive = 15
	}

	if mtu > 0 {
		b.WriteString(fmt.Sprintf("MTU = %d\n", mtu))
	}

	b.WriteString("\n[Peer]\n")
	b.WriteString(fmt.Sprintf("PublicKey = %s\n", cfg.ServerPublicKey))
	b.WriteString(fmt.Sprintf("PresharedKey = %s\n", cfg.PresharedKey))
	b.WriteString("AllowedIPs = 0.0.0.0/0, ::/0\n")
	b.WriteString(fmt.Sprintf("Endpoint = %s\n", cfg.Endpoint))
	b.WriteString(fmt.Sprintf("PersistentKeepalive = %d\n", keepalive))

	return b.String()
}
