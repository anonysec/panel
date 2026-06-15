package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRender_OpenVPN(t *testing.T) {
	e := NewEngine("")
	vars := TemplateVars{
		Port:       1194,
		Protocol:   "udp",
		Network:    "10.8.0.0/24",
		ServerNet:  "10.8.0.0",
		ServerMask: "255.255.255.0",
		DNS1:       "1.1.1.1",
		DNS2:       "8.8.8.8",
	}
	out, err := e.Render("openvpn", vars)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if out == "" {
		t.Fatal("Render returned empty string")
	}
	if !strings.Contains(out, "port 1194") {
		t.Error("missing port directive")
	}
	if !strings.Contains(out, "proto udp") {
		t.Error("missing proto directive")
	}
	if !strings.Contains(out, "server 10.8.0.0 255.255.255.0") {
		t.Error("missing server directive")
	}
}

func TestRender_OpenVPN_WithIPv6(t *testing.T) {
	e := NewEngine("")
	vars := TemplateVars{
		Port:        1194,
		Protocol:    "udp",
		Network:     "10.8.0.0/24",
		NetworkIPv6: "fd00:koris::/64",
		ServerNet:   "10.8.0.0",
		ServerMask:  "255.255.255.0",
		DNS1:        "1.1.1.1",
		DNS2:        "8.8.8.8",
		DNS1v6:      "2606:4700::1",
	}
	out, err := e.Render("openvpn", vars)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(out, "server-ipv6 fd00:koris::/64") {
		t.Error("missing server-ipv6 directive")
	}
	if !strings.Contains(out, "dhcp-option DNS6 2606:4700::1") {
		t.Error("missing DNS6 option")
	}
}

func TestRender_WireGuard(t *testing.T) {
	e := NewEngine("")
	vars := TemplateVars{
		Port:    51820,
		Network: "10.11.0.1/24",
		DNS1:    "1.1.1.1",
		DNS2:    "8.8.8.8",
	}
	out, err := e.Render("wireguard", vars)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(out, "[Interface]") {
		t.Error("missing [Interface] section")
	}
	if !strings.Contains(out, "ListenPort = 51820") {
		t.Error("missing ListenPort")
	}
	if !strings.Contains(out, "Address = 10.11.0.1/24") {
		t.Error("missing Address")
	}
	if !strings.Contains(out, "DNS = 1.1.1.1, 8.8.8.8") {
		t.Error("missing DNS")
	}
}

func TestRender_StrongSwan(t *testing.T) {
	e := NewEngine("")
	vars := TemplateVars{
		Network:  "10.10.0.0/24",
		ServerIP: "203.0.113.1",
		DNS1:     "1.1.1.1",
		DNS2:     "8.8.8.8",
	}
	out, err := e.Render("strongswan", vars)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(out, "leftid=203.0.113.1") {
		t.Error("missing leftid with ServerIP")
	}
	if !strings.Contains(out, "rightsourceip=10.10.0.0/24") {
		t.Error("missing rightsourceip with Network")
	}
}

func TestRender_XL2TPD(t *testing.T) {
	e := NewEngine("")
	vars := TemplateVars{
		Network:  "10.9.0.0/24",
		ServerIP: "192.168.1.1",
	}
	out, err := e.Render("xl2tpd", vars)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(out, "ip range = 10.9.0.0/24") {
		t.Error("missing ip range")
	}
	if !strings.Contains(out, "local ip = 192.168.1.1") {
		t.Error("missing local ip")
	}
}

func TestRender_UnknownProtocol(t *testing.T) {
	e := NewEngine("")
	_, err := e.Render("unknown", TemplateVars{})
	if err == nil {
		t.Error("expected error for unknown protocol")
	}
}

func TestValidate_OpenVPN_Valid(t *testing.T) {
	e := NewEngine("")
	valid := "port 1194\nproto udp\ndev tun\nserver 10.8.0.0 255.255.255.0"
	if err := e.Validate("openvpn", valid); err != nil {
		t.Errorf("Validate returned error for valid config: %v", err)
	}
}

func TestValidate_OpenVPN_MissingDirective(t *testing.T) {
	e := NewEngine("")
	invalid := "server 10.8.0.0 255.255.255.0\n"
	if err := e.Validate("openvpn", invalid); err == nil {
		t.Error("expected error for missing port/proto/dev")
	}
}

func TestValidate_WireGuard_Valid(t *testing.T) {
	e := NewEngine("")
	valid := "[Interface]\nAddress = 10.11.0.1/24\nListenPort = 51820\n"
	if err := e.Validate("wireguard", valid); err != nil {
		t.Errorf("Validate returned error for valid wireguard config: %v", err)
	}
}

func TestValidate_WireGuard_MissingInterface(t *testing.T) {
	e := NewEngine("")
	invalid := "Address = 10.11.0.1/24\nListenPort = 51820\n"
	if err := e.Validate("wireguard", invalid); err == nil {
		t.Error("expected error for missing [Interface] section")
	}
}

func TestValidate_Empty(t *testing.T) {
	e := NewEngine("")
	if err := e.Validate("openvpn", ""); err == nil {
		t.Error("expected error for empty config")
	}
	if err := e.Validate("openvpn", "   "); err == nil {
		t.Error("expected error for whitespace-only config")
	}
}

func TestValidate_UnresolvedTemplateDirectives(t *testing.T) {
	e := NewEngine("")
	withTemplate := "port 1194\nproto udp\ndev tun\nserver {{.Network}}"
	if err := e.Validate("openvpn", withTemplate); err == nil {
		t.Error("expected error for unresolved template directives")
	}
}

func TestDiff(t *testing.T) {
	e := NewEngine("")
	current := "port 1194\nproto udp"
	proposed := "port 443\nproto tcp"
	diff := e.Diff(current, proposed)
	if diff == "" {
		t.Error("expected non-empty diff")
	}
	if !strings.Contains(diff, "- port 1194") || !strings.Contains(diff, "+ port 443") {
		t.Error("diff should show changed port lines")
	}
	if !strings.Contains(diff, "- proto udp") || !strings.Contains(diff, "+ proto tcp") {
		t.Error("diff should show changed proto lines")
	}
}

func TestDiff_Identical(t *testing.T) {
	e := NewEngine("")
	config := "port 1194\nproto udp\ndev tun"
	diff := e.Diff(config, config)
	if diff != "" {
		t.Errorf("expected empty diff for identical configs, got: %q", diff)
	}
}

func TestApply_ValidConfig(t *testing.T) {
	e := NewEngine("")
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "server.conf")

	vars := TemplateVars{
		Port:       1194,
		Protocol:   "udp",
		Network:    "10.8.0.0/24",
		ServerNet:  "10.8.0.0",
		ServerMask: "255.255.255.0",
		DNS1:       "1.1.1.1",
		DNS2:       "8.8.8.8",
	}

	if err := e.Apply("openvpn", confPath, vars); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	content, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(content), "port 1194") {
		t.Error("written config missing port directive")
	}
}

func TestApply_CreatesBackup(t *testing.T) {
	e := NewEngine("")
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "server.conf")

	// Write an initial config
	originalContent := "port 1194\nproto udp\ndev tun\n"
	if err := os.WriteFile(confPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Override timeNowUnix for predictable backup names
	oldTimeNow := timeNowUnix
	timeNowUnix = func() int64 { return 1234567890 }
	defer func() { timeNowUnix = oldTimeNow }()

	vars := TemplateVars{
		Port:       443,
		Protocol:   "tcp",
		Network:    "10.8.0.0/24",
		ServerNet:  "10.8.0.0",
		ServerMask: "255.255.255.0",
		DNS1:       "1.1.1.1",
		DNS2:       "8.8.8.8",
	}

	if err := e.Apply("openvpn", confPath, vars); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Check backup exists
	backupPath := confPath + ".bak.1234567890"
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Backup file not found at %s: %v", backupPath, err)
	}
	if string(backupContent) != originalContent {
		t.Error("backup content does not match original")
	}

	// Check new config is different
	newContent, _ := os.ReadFile(confPath)
	if strings.Contains(string(newContent), "port 1194") {
		t.Error("config should have been updated to new port")
	}
	if !strings.Contains(string(newContent), "port 443") {
		t.Error("config should contain new port 443")
	}
}

func TestApply_InvalidNetwork(t *testing.T) {
	e := NewEngine("")
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "server.conf")

	vars := TemplateVars{
		Port:       1194,
		Protocol:   "udp",
		Network:    "8.8.8.0/24", // Public IP - should fail
		ServerNet:  "8.8.8.0",
		ServerMask: "255.255.255.0",
		DNS1:       "1.1.1.1",
		DNS2:       "8.8.8.8",
	}

	if err := e.Apply("openvpn", confPath, vars); err == nil {
		t.Error("expected error for public IP network")
	}

	// Config file should NOT exist since validation failed
	if _, err := os.Stat(confPath); err == nil {
		t.Error("config file should not be written when validation fails")
	}
}

func TestApply_InvalidIPv6Network(t *testing.T) {
	e := NewEngine("")
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "server.conf")

	vars := TemplateVars{
		Port:        1194,
		Protocol:    "udp",
		Network:     "10.8.0.0/24",
		NetworkIPv6: "2001:db8::/32", // Not ULA - should fail
		ServerNet:   "10.8.0.0",
		ServerMask:  "255.255.255.0",
		DNS1:        "1.1.1.1",
		DNS2:        "8.8.8.8",
	}

	if err := e.Apply("openvpn", confPath, vars); err == nil {
		t.Error("expected error for non-ULA IPv6 network")
	}
}

func TestNewEngine_FromFilesystem(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a custom template
	customTmpl := `port {{.Port}}
proto {{.Protocol}}
dev tun
custom-directive true
`
	tmplPath := filepath.Join(tmpDir, "openvpn.conf.tmpl")
	if err := os.WriteFile(tmplPath, []byte(customTmpl), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	e := NewEngine(tmpDir)
	vars := TemplateVars{
		Port:     1194,
		Protocol: "udp",
	}
	out, err := e.Render("openvpn", vars)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !strings.Contains(out, "custom-directive true") {
		t.Error("should use filesystem template with custom directive")
	}
}
