//go:build !lite

package xray

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateXrayFragment_VLESS_Reality(t *testing.T) {
	cfg := InboundConfig{
		UUID:       "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Protocol:   "vless",
		Transport:  "tcp",
		Security:   "reality",
		ServerName: "www.google.com",
		PublicKey:  "testpublickey123",
		ShortID:    "abcd1234",
		PrivateKey: "testprivatekey123",
		Port:       443,
	}

	data, err := GenerateXrayFragment(cfg)
	if err != nil {
		t.Fatalf("GenerateXrayFragment error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	inbounds, ok := result["inbounds"].([]any)
	if !ok || len(inbounds) == 0 {
		t.Fatal("expected inbounds array with at least one entry")
	}

	inbound := inbounds[0].(map[string]any)
	if inbound["protocol"] != "vless" {
		t.Errorf("expected protocol vless, got %v", inbound["protocol"])
	}
	if int(inbound["port"].(float64)) != 443 {
		t.Errorf("expected port 443, got %v", inbound["port"])
	}

	stream := inbound["streamSettings"].(map[string]any)
	if stream["security"] != "reality" {
		t.Errorf("expected security reality, got %v", stream["security"])
	}
	if stream["network"] != "tcp" {
		t.Errorf("expected network tcp, got %v", stream["network"])
	}

	realitySettings := stream["realitySettings"].(map[string]any)
	if realitySettings["privateKey"] != "testprivatekey123" {
		t.Errorf("expected privateKey testprivatekey123, got %v", realitySettings["privateKey"])
	}
}

func TestGenerateXrayFragment_VMess_WS_TLS(t *testing.T) {
	cfg := InboundConfig{
		UUID:       "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Protocol:   "vmess",
		Transport:  "ws",
		Security:   "tls",
		ServerName: "example.com",
		CertPath:   "/etc/ssl/cert.pem",
		KeyPath:    "/etc/ssl/key.pem",
		Path:       "/ws",
		Port:       8443,
	}

	data, err := GenerateXrayFragment(cfg)
	if err != nil {
		t.Fatalf("GenerateXrayFragment error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	inbounds := result["inbounds"].([]any)
	inbound := inbounds[0].(map[string]any)
	if inbound["protocol"] != "vmess" {
		t.Errorf("expected protocol vmess, got %v", inbound["protocol"])
	}

	stream := inbound["streamSettings"].(map[string]any)
	if stream["security"] != "tls" {
		t.Errorf("expected security tls, got %v", stream["security"])
	}
	if stream["network"] != "ws" {
		t.Errorf("expected network ws, got %v", stream["network"])
	}

	wsSettings := stream["wsSettings"].(map[string]any)
	if wsSettings["path"] != "/ws" {
		t.Errorf("expected path /ws, got %v", wsSettings["path"])
	}
}

func TestGenerateXrayFragment_Trojan_GRPC(t *testing.T) {
	cfg := InboundConfig{
		UUID:        "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Protocol:    "trojan",
		Transport:   "grpc",
		Security:    "tls",
		ServerName:  "grpc.example.com",
		CertPath:    "/etc/ssl/cert.pem",
		KeyPath:     "/etc/ssl/key.pem",
		ServiceName: "mygrpc",
		Port:        2053,
	}

	data, err := GenerateXrayFragment(cfg)
	if err != nil {
		t.Fatalf("GenerateXrayFragment error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	inbounds := result["inbounds"].([]any)
	inbound := inbounds[0].(map[string]any)
	if inbound["protocol"] != "trojan" {
		t.Errorf("expected protocol trojan, got %v", inbound["protocol"])
	}

	settings := inbound["settings"].(map[string]any)
	clients := settings["clients"].([]any)
	client := clients[0].(map[string]any)
	if client["password"] != cfg.UUID {
		t.Errorf("expected password %s, got %v", cfg.UUID, client["password"])
	}

	stream := inbound["streamSettings"].(map[string]any)
	grpcSettings := stream["grpcSettings"].(map[string]any)
	if grpcSettings["serviceName"] != "mygrpc" {
		t.Errorf("expected serviceName mygrpc, got %v", grpcSettings["serviceName"])
	}
}

func TestGenerateXrayFragment_UnsupportedProtocol(t *testing.T) {
	cfg := InboundConfig{
		UUID:     "test-uuid",
		Protocol: "unknown",
		Port:     443,
	}

	_, err := GenerateXrayFragment(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported protocol")
	}
}

func TestGenerateSingBoxFragment_VLESS_Reality(t *testing.T) {
	cfg := InboundConfig{
		UUID:       "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Protocol:   "vless",
		Transport:  "tcp",
		Security:   "reality",
		ServerName: "www.google.com",
		PublicKey:  "testpubkey",
		ShortID:    "abcd",
		PrivateKey: "testprivkey",
		Port:       443,
	}

	data, err := GenerateSingBoxFragment(cfg)
	if err != nil {
		t.Fatalf("GenerateSingBoxFragment error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	inbounds := result["inbounds"].([]any)
	inbound := inbounds[0].(map[string]any)
	if inbound["type"] != "vless" {
		t.Errorf("expected type vless, got %v", inbound["type"])
	}
	if int(inbound["listen_port"].(float64)) != 443 {
		t.Errorf("expected listen_port 443, got %v", inbound["listen_port"])
	}

	tls := inbound["tls"].(map[string]any)
	if tls["enabled"] != true {
		t.Error("expected tls enabled")
	}
	reality := tls["reality"].(map[string]any)
	if reality["private_key"] != "testprivkey" {
		t.Errorf("expected private_key testprivkey, got %v", reality["private_key"])
	}
}

func TestGenerateSingBoxFragment_VMess_WS(t *testing.T) {
	cfg := InboundConfig{
		UUID:       "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Protocol:   "vmess",
		Transport:  "ws",
		Security:   "tls",
		ServerName: "example.com",
		CertPath:   "/cert.pem",
		KeyPath:    "/key.pem",
		Path:       "/vmess-ws",
		Port:       8080,
	}

	data, err := GenerateSingBoxFragment(cfg)
	if err != nil {
		t.Fatalf("GenerateSingBoxFragment error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	inbounds := result["inbounds"].([]any)
	inbound := inbounds[0].(map[string]any)
	if inbound["type"] != "vmess" {
		t.Errorf("expected type vmess, got %v", inbound["type"])
	}

	transport := inbound["transport"].(map[string]any)
	if transport["type"] != "ws" {
		t.Errorf("expected transport type ws, got %v", transport["type"])
	}
	if transport["path"] != "/vmess-ws" {
		t.Errorf("expected path /vmess-ws, got %v", transport["path"])
	}
}

func TestGenerateShareLink_VLESS(t *testing.T) {
	cfg := InboundConfig{
		UUID:       "test-uuid-1234",
		Protocol:   "vless",
		Transport:  "tcp",
		Security:   "reality",
		ServerName: "www.google.com",
		PublicKey:  "mypubkey",
		ShortID:    "ab12",
		Port:       443,
	}

	link := GenerateShareLink(cfg, "1.2.3.4", "MyServer")
	if !strings.HasPrefix(link, "vless://") {
		t.Errorf("expected vless:// prefix, got %s", link)
	}
	if !strings.Contains(link, "test-uuid-1234@1.2.3.4:443") {
		t.Errorf("expected UUID@host:port in link, got %s", link)
	}
	if !strings.Contains(link, "security=reality") {
		t.Errorf("expected security=reality in link, got %s", link)
	}
	if !strings.Contains(link, "pbk=mypubkey") {
		t.Errorf("expected pbk=mypubkey in link, got %s", link)
	}
	if !strings.Contains(link, "sid=ab12") {
		t.Errorf("expected sid=ab12 in link, got %s", link)
	}
	if !strings.Contains(link, "#MyServer") {
		t.Errorf("expected #MyServer remark in link, got %s", link)
	}
}

func TestGenerateShareLink_VMess(t *testing.T) {
	cfg := InboundConfig{
		UUID:       "vmess-uuid-5678",
		Protocol:   "vmess",
		Transport:  "ws",
		Security:   "tls",
		ServerName: "ws.example.com",
		Path:       "/chat",
		Port:       8443,
	}

	link := GenerateShareLink(cfg, "5.6.7.8", "WS Server")
	if !strings.HasPrefix(link, "vmess://") {
		t.Errorf("expected vmess:// prefix, got %s", link)
	}

	// Decode the base64 payload
	encoded := strings.TrimPrefix(link, "vmess://")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("failed to decode vmess base64: %v", err)
	}

	var vmessObj map[string]string
	if err := json.Unmarshal(decoded, &vmessObj); err != nil {
		t.Fatalf("failed to parse vmess JSON: %v", err)
	}

	if vmessObj["id"] != "vmess-uuid-5678" {
		t.Errorf("expected id vmess-uuid-5678, got %s", vmessObj["id"])
	}
	if vmessObj["add"] != "5.6.7.8" {
		t.Errorf("expected add 5.6.7.8, got %s", vmessObj["add"])
	}
	if vmessObj["port"] != "8443" {
		t.Errorf("expected port 8443, got %s", vmessObj["port"])
	}
	if vmessObj["net"] != "ws" {
		t.Errorf("expected net ws, got %s", vmessObj["net"])
	}
	if vmessObj["tls"] != "tls" {
		t.Errorf("expected tls tls, got %s", vmessObj["tls"])
	}
	if vmessObj["path"] != "/chat" {
		t.Errorf("expected path /chat, got %s", vmessObj["path"])
	}
	if vmessObj["ps"] != "WS Server" {
		t.Errorf("expected ps 'WS Server', got %s", vmessObj["ps"])
	}
}

func TestGenerateShareLink_Trojan(t *testing.T) {
	cfg := InboundConfig{
		UUID:       "trojan-pass-9012",
		Protocol:   "trojan",
		Transport:  "ws",
		Security:   "tls",
		ServerName: "trojan.example.com",
		Path:       "/trojan-ws",
		Port:       443,
	}

	link := GenerateShareLink(cfg, "9.8.7.6", "Trojan WS")
	if !strings.HasPrefix(link, "trojan://") {
		t.Errorf("expected trojan:// prefix, got %s", link)
	}
	if !strings.Contains(link, "trojan-pass-9012@9.8.7.6:443") {
		t.Errorf("expected UUID@host:port in link, got %s", link)
	}
	if !strings.Contains(link, "security=tls") {
		t.Errorf("expected security=tls in link, got %s", link)
	}
	if !strings.Contains(link, "sni=trojan.example.com") {
		t.Errorf("expected sni in link, got %s", link)
	}
}

func TestGenerateShareLink_UnknownProtocol(t *testing.T) {
	cfg := InboundConfig{
		UUID:     "test",
		Protocol: "unknown",
		Port:     443,
	}
	link := GenerateShareLink(cfg, "1.2.3.4", "test")
	if link != "" {
		t.Errorf("expected empty string for unknown protocol, got %s", link)
	}
}

func TestGenerateSubscription(t *testing.T) {
	links := []string{
		"vless://uuid1@host1:443?type=tcp#server1",
		"vmess://base64data",
		"trojan://pass@host2:8443?type=ws#server2",
	}

	sub := GenerateSubscription(links)

	decoded, err := base64.StdEncoding.DecodeString(sub)
	if err != nil {
		t.Fatalf("failed to decode subscription: %v", err)
	}

	expected := strings.Join(links, "\n")
	if string(decoded) != expected {
		t.Errorf("decoded subscription mismatch.\nExpected: %s\nGot: %s", expected, string(decoded))
	}
}

func TestGenerateSubscription_Empty(t *testing.T) {
	sub := GenerateSubscription([]string{})
	decoded, err := base64.StdEncoding.DecodeString(sub)
	if err != nil {
		t.Fatalf("failed to decode empty subscription: %v", err)
	}
	if string(decoded) != "" {
		t.Errorf("expected empty string, got %s", string(decoded))
	}
}

func TestGenerateSubscription_Single(t *testing.T) {
	links := []string{"vless://uuid@host:443#test"}
	sub := GenerateSubscription(links)
	decoded, err := base64.StdEncoding.DecodeString(sub)
	if err != nil {
		t.Fatalf("failed to decode subscription: %v", err)
	}
	if string(decoded) != "vless://uuid@host:443#test" {
		t.Errorf("expected single link, got %s", string(decoded))
	}
}
