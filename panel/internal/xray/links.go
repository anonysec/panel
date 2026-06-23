//go:build !lite

package xray

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// LinkParams contains all parameters needed to generate protocol share links.
type LinkParams struct {
	UUID        string `json:"uuid"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Remark      string `json:"remark"`
	Transport   string `json:"transport"`    // tcp, ws, grpc, h2
	Security    string `json:"security"`     // tls, reality, none
	ServerName  string `json:"server_name"`  // SNI
	Path        string `json:"path"`         // WebSocket path or HTTP/2 path
	ServiceName string `json:"service_name"` // gRPC service name
	Flow        string `json:"flow"`         // XTLS flow (e.g. xtls-rprx-vision)
	Fingerprint string `json:"fingerprint"`  // TLS fingerprint (e.g. chrome)
	Method      string `json:"method"`       // Shadowsocks cipher method
}

// GeneratedConfig holds a generated protocol config link and its QR-ready data.
type GeneratedConfig struct {
	Protocol string `json:"protocol"`
	Link     string `json:"link"`
	QRData   string `json:"qr_data"`
}

// GenerateVLESSLink creates a VLESS share link in standard format.
// Format: vless://UUID@host:port?type=transport&encryption=none&security=tls&sni=servername&fp=fingerprint&flow=flow#remark
func GenerateVLESSLink(params LinkParams) string {
	query := url.Values{}
	query.Set("type", params.Transport)
	query.Set("encryption", "none")

	if params.Security != "" {
		query.Set("security", params.Security)
	} else {
		query.Set("security", "none")
	}

	if params.ServerName != "" {
		query.Set("sni", params.ServerName)
	}
	if params.Fingerprint != "" {
		query.Set("fp", params.Fingerprint)
	}
	if params.Flow != "" {
		query.Set("flow", params.Flow)
	}
	if params.Path != "" {
		query.Set("path", params.Path)
	}
	if params.ServiceName != "" {
		query.Set("serviceName", params.ServiceName)
	}
	if params.Transport == TransportWS && params.ServerName != "" {
		query.Set("host", params.ServerName)
	}

	link := fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		params.UUID,
		params.Host,
		params.Port,
		query.Encode(),
		url.PathEscape(params.Remark),
	)
	return link
}

// GenerateVMessLink creates a VMess share link with base64-encoded JSON (v2 format).
// Format: vmess://base64({"v":"2","ps":"remark","add":"host","port":"port","id":"uuid",...})
func GenerateVMessLink(params LinkParams) string {
	tlsType := ""
	if params.Security == "tls" || params.Security == "reality" {
		tlsType = "tls"
	}

	host := ""
	if params.Transport == TransportWS || params.Transport == TransportH2 {
		host = params.ServerName
	}

	vmessObj := map[string]string{
		"v":    "2",
		"ps":   params.Remark,
		"add":  params.Host,
		"port": strconv.Itoa(params.Port),
		"id":   params.UUID,
		"aid":  "0",
		"scy":  "auto",
		"net":  params.Transport,
		"type": "none",
		"host": host,
		"path": params.Path,
		"tls":  tlsType,
	}

	if params.ServiceName != "" && params.Transport == TransportGRPC {
		vmessObj["path"] = params.ServiceName
	}

	jsonBytes, _ := json.Marshal(vmessObj)
	encoded := base64.StdEncoding.EncodeToString(jsonBytes)
	return "vmess://" + encoded
}

// GenerateTrojanLink creates a Trojan share link.
// Format: trojan://UUID@host:port?type=transport&security=tls&sni=servername#remark
func GenerateTrojanLink(params LinkParams) string {
	query := url.Values{}
	query.Set("type", params.Transport)

	if params.Security != "" {
		query.Set("security", params.Security)
	} else {
		query.Set("security", "tls")
	}

	if params.ServerName != "" {
		query.Set("sni", params.ServerName)
	}
	if params.Fingerprint != "" {
		query.Set("fp", params.Fingerprint)
	}
	if params.Path != "" {
		query.Set("path", params.Path)
	}
	if params.ServiceName != "" {
		query.Set("serviceName", params.ServiceName)
	}
	if params.Transport == TransportWS && params.ServerName != "" {
		query.Set("host", params.ServerName)
	}

	link := fmt.Sprintf("trojan://%s@%s:%d?%s#%s",
		params.UUID,
		params.Host,
		params.Port,
		query.Encode(),
		url.PathEscape(params.Remark),
	)
	return link
}

// GenerateShadowsocksLink creates a Shadowsocks SIP002 share link.
// Format: ss://base64(method:password)@host:port#remark
func GenerateShadowsocksLink(params LinkParams) string {
	method := params.Method
	if method == "" {
		method = "chacha20-ietf-poly1305"
	}

	// SIP002 format: base64(method:password)
	userInfo := base64.URLEncoding.EncodeToString([]byte(method + ":" + params.UUID))
	// Remove padding for compatibility
	userInfo = strings.TrimRight(userInfo, "=")

	link := fmt.Sprintf("ss://%s@%s:%d#%s",
		userInfo,
		params.Host,
		params.Port,
		url.PathEscape(params.Remark),
	)
	return link
}

// GenerateAllLinks generates config links for each requested protocol.
func GenerateAllLinks(params LinkParams, protocols []string) []GeneratedConfig {
	configs := make([]GeneratedConfig, 0, len(protocols))

	for _, proto := range protocols {
		var link string
		switch strings.ToLower(proto) {
		case ProtocolVLESS:
			link = GenerateVLESSLink(params)
		case ProtocolVMess:
			link = GenerateVMessLink(params)
		case ProtocolTrojan:
			link = GenerateTrojanLink(params)
		case ProtocolShadowsocks:
			link = GenerateShadowsocksLink(params)
		default:
			continue
		}

		configs = append(configs, GeneratedConfig{
			Protocol: proto,
			Link:     link,
			QRData:   link,
		})
	}

	return configs
}

// GenerateCustomerConfigs reads the node's xray configuration from the database
// and generates share links for all enabled inbounds for the given customer UUID.
func (s *XrayService) GenerateCustomerConfigs(ctx context.Context, customerUUID string, nodeID int64) ([]GeneratedConfig, error) {
	// Get the xray config for this node.
	cfg, err := s.GetConfig(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("get xray config for node %d: %w", nodeID, err)
	}

	if !cfg.Enabled {
		return nil, fmt.Errorf("xray is not enabled on node %d", nodeID)
	}

	// Get the node's public IP and domain for the host field.
	var publicIP string
	var domain *string
	var nodeName string

	err = s.db.QueryRowContext(ctx,
		`SELECT public_ip, domain, name FROM nodes WHERE id = ?`, nodeID,
	).Scan(&publicIP, &domain, &nodeName)
	if err != nil {
		return nil, fmt.Errorf("query node %d: %w", nodeID, err)
	}

	// Use domain if available, otherwise use public IP.
	host := publicIP
	if domain != nil && *domain != "" {
		host = *domain
	}

	var allConfigs []GeneratedConfig

	for _, inbound := range cfg.Inbounds {
		params := LinkParams{
			UUID:      customerUUID,
			Host:      host,
			Port:      inbound.Port,
			Remark:    nodeName + "-" + inbound.Protocol,
			Transport: inbound.Transport,
		}

		// Apply TLS settings from the node config.
		if cfg.TLS.ServerName != "" {
			params.Security = "tls"
			params.ServerName = cfg.TLS.ServerName
		}

		// Apply Reality settings if configured.
		if cfg.RealityConfig != nil && len(cfg.RealityConfig.ServerNames) > 0 {
			params.Security = "reality"
			params.ServerName = cfg.RealityConfig.ServerNames[0]
			params.Fingerprint = "chrome"
		}

		// Parse inbound-specific settings if present.
		if len(inbound.Settings) > 0 {
			var settings map[string]interface{}
			if json.Unmarshal(inbound.Settings, &settings) == nil {
				if path, ok := settings["path"].(string); ok {
					params.Path = path
				}
				if sn, ok := settings["service_name"].(string); ok {
					params.ServiceName = sn
				}
				if flow, ok := settings["flow"].(string); ok {
					params.Flow = flow
				}
				if fp, ok := settings["fingerprint"].(string); ok {
					params.Fingerprint = fp
				}
				if method, ok := settings["method"].(string); ok {
					params.Method = method
				}
			}
		}

		var link string
		switch inbound.Protocol {
		case ProtocolVLESS:
			link = GenerateVLESSLink(params)
		case ProtocolVMess:
			link = GenerateVMessLink(params)
		case ProtocolTrojan:
			link = GenerateTrojanLink(params)
		case ProtocolShadowsocks:
			link = GenerateShadowsocksLink(params)
		default:
			continue
		}

		allConfigs = append(allConfigs, GeneratedConfig{
			Protocol: inbound.Protocol,
			Link:     link,
			QRData:   link,
		})
	}

	return allConfigs, nil
}
