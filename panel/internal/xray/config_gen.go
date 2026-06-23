//go:build !lite

package xray

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// InboundConfig represents a user's xray/sing-box inbound configuration
// for config fragment generation and share link creation.
type InboundConfig struct {
	UUID        string `json:"uuid"`
	Protocol    string `json:"protocol"`               // "vless", "vmess", "trojan"
	Transport   string `json:"transport"`              // "tcp", "ws", "grpc", "h2"
	Security    string `json:"security"`               // "reality", "tls", "none"
	ServerName  string `json:"server_name,omitempty"`  // SNI / Reality server name
	PublicKey   string `json:"public_key,omitempty"`   // Reality public key
	ShortID     string `json:"short_id,omitempty"`     // Reality short ID
	PrivateKey  string `json:"private_key,omitempty"`  // Reality private key
	CertPath    string `json:"cert_path,omitempty"`    // TLS certificate path
	KeyPath     string `json:"key_path,omitempty"`     // TLS key path
	Path        string `json:"path,omitempty"`         // WS/H2 path
	ServiceName string `json:"service_name,omitempty"` // gRPC service name
	Port        int    `json:"port"`
}

// GenerateXrayFragment produces a JSON config fragment for xray-core
// representing a single inbound listener based on the given InboundConfig.
func GenerateXrayFragment(cfg InboundConfig) ([]byte, error) {
	// Build settings based on protocol
	var settings map[string]any
	switch cfg.Protocol {
	case ProtocolVLESS:
		settings = map[string]any{
			"clients": []map[string]any{
				{
					"id":    cfg.UUID,
					"flow":  flowForConfig(cfg),
					"level": 0,
				},
			},
			"decryption": "none",
		}
	case ProtocolVMess:
		settings = map[string]any{
			"clients": []map[string]any{
				{
					"id":      cfg.UUID,
					"alterId": 0,
					"level":   0,
				},
			},
		}
	case ProtocolTrojan:
		settings = map[string]any{
			"clients": []map[string]any{
				{
					"password": cfg.UUID,
					"level":    0,
				},
			},
		}
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", cfg.Protocol)
	}

	// Build streamSettings
	streamSettings := buildXrayStreamSettings(cfg)

	inbound := map[string]any{
		"listen":         "0.0.0.0",
		"port":           cfg.Port,
		"protocol":       cfg.Protocol,
		"settings":       settings,
		"streamSettings": streamSettings,
		"tag":            fmt.Sprintf("%s-%s-%d", cfg.Protocol, cfg.UUID[:8], cfg.Port),
	}

	fragment := map[string]any{
		"inbounds": []any{inbound},
	}

	return json.MarshalIndent(fragment, "", "  ")
}

// GenerateSingBoxFragment produces a JSON config fragment for sing-box
// representing a single inbound listener based on the given InboundConfig.
func GenerateSingBoxFragment(cfg InboundConfig) ([]byte, error) {
	inbound := map[string]any{
		"type":   singboxProtocolType(cfg.Protocol),
		"tag":    fmt.Sprintf("%s-in-%d", cfg.Protocol, cfg.Port),
		"listen": "::",
		"listen_fields": map[string]any{
			"listen_port": cfg.Port,
		},
	}

	// Flatten listen_port to top level (sing-box format)
	delete(inbound, "listen_fields")
	inbound["listen_port"] = cfg.Port

	// Add users based on protocol
	switch cfg.Protocol {
	case ProtocolVLESS:
		users := []map[string]any{
			{"uuid": cfg.UUID, "flow": flowForSingBox(cfg)},
		}
		inbound["users"] = users
	case ProtocolVMess:
		users := []map[string]any{
			{"uuid": cfg.UUID, "alterId": 0},
		}
		inbound["users"] = users
	case ProtocolTrojan:
		users := []map[string]any{
			{"password": cfg.UUID},
		}
		inbound["users"] = users
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", cfg.Protocol)
	}

	// Add transport
	if cfg.Transport != TransportTCP {
		inbound["transport"] = buildSingBoxTransport(cfg)
	}

	// Add TLS / Reality
	if cfg.Security == "tls" {
		inbound["tls"] = map[string]any{
			"enabled":          true,
			"server_name":      cfg.ServerName,
			"certificate_path": cfg.CertPath,
			"key_path":         cfg.KeyPath,
		}
	} else if cfg.Security == "reality" {
		inbound["tls"] = map[string]any{
			"enabled":     true,
			"server_name": cfg.ServerName,
			"reality": map[string]any{
				"enabled":     true,
				"private_key": cfg.PrivateKey,
				"short_id":    []string{cfg.ShortID},
				"handshake": map[string]any{
					"server":      cfg.ServerName,
					"server_port": 443,
				},
			},
		}
	}

	fragment := map[string]any{
		"inbounds": []any{inbound},
	}

	return json.MarshalIndent(fragment, "", "  ")
}

// GenerateShareLink produces a protocol share link (vless://, vmess://, trojan://)
// for the given InboundConfig, host, and remark.
func GenerateShareLink(cfg InboundConfig, host string, remark string) string {
	switch cfg.Protocol {
	case ProtocolVLESS:
		return generateVLESSShareLink(cfg, host, remark)
	case ProtocolVMess:
		return generateVMessShareLink(cfg, host, remark)
	case ProtocolTrojan:
		return generateTrojanShareLink(cfg, host, remark)
	default:
		return ""
	}
}

// GenerateSubscription joins links with newlines and base64 encodes the result.
func GenerateSubscription(links []string) string {
	joined := strings.Join(links, "\n")
	return base64.StdEncoding.EncodeToString([]byte(joined))
}

// --- internal helpers ---

func flowForConfig(cfg InboundConfig) string {
	if cfg.Protocol == ProtocolVLESS && cfg.Security == "reality" && cfg.Transport == TransportTCP {
		return "xtls-rprx-vision"
	}
	return ""
}

func flowForSingBox(cfg InboundConfig) string {
	if cfg.Protocol == ProtocolVLESS && cfg.Security == "reality" && cfg.Transport == TransportTCP {
		return "xtls-rprx-vision"
	}
	return ""
}

func singboxProtocolType(protocol string) string {
	switch protocol {
	case ProtocolVLESS:
		return "vless"
	case ProtocolVMess:
		return "vmess"
	case ProtocolTrojan:
		return "trojan"
	default:
		return protocol
	}
}

func buildXrayStreamSettings(cfg InboundConfig) map[string]any {
	stream := map[string]any{
		"network": xrayNetworkName(cfg.Transport),
	}

	// Transport-specific settings
	switch cfg.Transport {
	case TransportWS:
		stream["wsSettings"] = map[string]any{
			"path": cfg.Path,
			"headers": map[string]any{
				"Host": cfg.ServerName,
			},
		}
	case TransportGRPC:
		stream["grpcSettings"] = map[string]any{
			"serviceName": cfg.ServiceName,
		}
	case TransportH2:
		hosts := []string{}
		if cfg.ServerName != "" {
			hosts = []string{cfg.ServerName}
		}
		stream["httpSettings"] = map[string]any{
			"path": cfg.Path,
			"host": hosts,
		}
	case TransportTCP:
		stream["tcpSettings"] = map[string]any{}
	}

	// Security settings
	switch cfg.Security {
	case "tls":
		stream["security"] = "tls"
		stream["tlsSettings"] = map[string]any{
			"serverName": cfg.ServerName,
			"certificates": []map[string]any{
				{
					"certificateFile": cfg.CertPath,
					"keyFile":         cfg.KeyPath,
				},
			},
		}
	case "reality":
		stream["security"] = "reality"
		stream["realitySettings"] = map[string]any{
			"dest":        cfg.ServerName + ":443",
			"serverNames": []string{cfg.ServerName},
			"privateKey":  cfg.PrivateKey,
			"shortIds":    []string{cfg.ShortID},
		}
	default:
		stream["security"] = "none"
	}

	return stream
}

func buildSingBoxTransport(cfg InboundConfig) map[string]any {
	switch cfg.Transport {
	case TransportWS:
		t := map[string]any{
			"type": "ws",
			"path": cfg.Path,
		}
		if cfg.ServerName != "" {
			t["headers"] = map[string]any{
				"Host": cfg.ServerName,
			}
		}
		return t
	case TransportGRPC:
		return map[string]any{
			"type":         "grpc",
			"service_name": cfg.ServiceName,
		}
	case TransportH2:
		t := map[string]any{
			"type": "http",
			"path": cfg.Path,
		}
		if cfg.ServerName != "" {
			t["host"] = []string{cfg.ServerName}
		}
		return t
	default:
		return map[string]any{"type": "tcp"}
	}
}

func xrayNetworkName(transport string) string {
	switch transport {
	case TransportWS:
		return "ws"
	case TransportGRPC:
		return "grpc"
	case TransportH2:
		return "http"
	case TransportTCP:
		return "tcp"
	default:
		return transport
	}
}

func generateVLESSShareLink(cfg InboundConfig, host string, remark string) string {
	query := url.Values{}
	query.Set("type", cfg.Transport)
	query.Set("encryption", "none")
	query.Set("security", cfg.Security)

	if cfg.ServerName != "" {
		query.Set("sni", cfg.ServerName)
	}
	if cfg.Security == "reality" {
		if cfg.PublicKey != "" {
			query.Set("pbk", cfg.PublicKey)
		}
		if cfg.ShortID != "" {
			query.Set("sid", cfg.ShortID)
		}
		query.Set("fp", "chrome")
		if cfg.Transport == TransportTCP {
			query.Set("flow", "xtls-rprx-vision")
		}
	}
	if cfg.Path != "" {
		query.Set("path", cfg.Path)
	}
	if cfg.ServiceName != "" {
		query.Set("serviceName", cfg.ServiceName)
	}

	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		cfg.UUID, host, cfg.Port, query.Encode(), url.PathEscape(remark))
}

func generateVMessShareLink(cfg InboundConfig, host string, remark string) string {
	tlsType := ""
	if cfg.Security == "tls" || cfg.Security == "reality" {
		tlsType = "tls"
	}

	vmessHost := ""
	if cfg.Transport == TransportWS || cfg.Transport == TransportH2 {
		vmessHost = cfg.ServerName
	}

	path := cfg.Path
	if cfg.Transport == TransportGRPC && cfg.ServiceName != "" {
		path = cfg.ServiceName
	}

	vmessObj := map[string]string{
		"v":    "2",
		"ps":   remark,
		"add":  host,
		"port": strconv.Itoa(cfg.Port),
		"id":   cfg.UUID,
		"aid":  "0",
		"scy":  "auto",
		"net":  cfg.Transport,
		"type": "none",
		"host": vmessHost,
		"path": path,
		"tls":  tlsType,
		"sni":  cfg.ServerName,
	}

	jsonBytes, _ := json.Marshal(vmessObj)
	encoded := base64.StdEncoding.EncodeToString(jsonBytes)
	return "vmess://" + encoded
}

func generateTrojanShareLink(cfg InboundConfig, host string, remark string) string {
	query := url.Values{}
	query.Set("type", cfg.Transport)

	security := cfg.Security
	if security == "" || security == "none" {
		security = "tls"
	}
	query.Set("security", security)

	if cfg.ServerName != "" {
		query.Set("sni", cfg.ServerName)
	}
	if cfg.Path != "" {
		query.Set("path", cfg.Path)
	}
	if cfg.ServiceName != "" {
		query.Set("serviceName", cfg.ServiceName)
	}

	return fmt.Sprintf("trojan://%s@%s:%d?%s#%s",
		cfg.UUID, host, cfg.Port, query.Encode(), url.PathEscape(remark))
}
