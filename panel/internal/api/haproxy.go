package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// ========== HAProxy Management ==========

// haproxyConfig generates and applies HAProxy config for TCP load balancing.
// POST /api/admin/haproxy/apply — regenerates config from active nodes
// GET /api/admin/haproxy/status — returns current HAProxy status
func (s *Server) haproxyApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Get all active nodes with OpenVPN TCP enabled
	rows, err := s.DB.Query(`
		SELECT n.id, n.name, COALESCE(n.domain,''), n.public_ip, c.port
		FROM nodes n
		JOIN node_vpn_configs c ON c.node_id = n.id AND c.protocol = 'openvpn' AND c.enabled = 1
		WHERE n.status <> 'disabled'
		ORDER BY n.id`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	var backends []NodeBackend
	for rows.Next() {
		var nb NodeBackend
		var domain, publicIP string
		if rows.Scan(&nb.ID, &nb.Name, &domain, &publicIP, &nb.Port) == nil {
			nb.Host = strings.TrimSpace(domain)
			if nb.Host == "" {
				nb.Host = strings.TrimSpace(publicIP)
			}
			if nb.Host != "" {
				backends = append(backends, nb)
			}
		}
	}

	if len(backends) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "no_active_nodes"})
		return
	}

	// Generate HAProxy config
	config := generateHAProxyConfig(backends)

	// Write config
	configPath := "/etc/haproxy/haproxy.cfg"
	if err := os.MkdirAll("/etc/haproxy", 0755); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "mkdir: " + err.Error()})
		return
	}
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "write: " + err.Error()})
		return
	}

	// Test config
	testCmd := exec.Command("haproxy", "-c", "-f", configPath)
	if out, err := testCmd.CombinedOutput(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "config_invalid: " + string(out)})
		return
	}

	// Reload HAProxy
	if err := exec.Command("systemctl", "reload", "haproxy").Run(); err != nil {
		// Try restart if reload fails
		exec.Command("systemctl", "restart", "haproxy").Run()
	}

	log.Printf("[haproxy] config applied with %d backends", len(backends))
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "haproxy.applied", "system", "haproxy", nil, map[string]any{"backends": len(backends)}, clientIP(r))

	writeJSON(w, map[string]any{"ok": true, "backends": len(backends), "config_path": configPath})
}

func (s *Server) haproxyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	active := "inactive"
	if out, err := exec.Command("systemctl", "is-active", "haproxy").Output(); err == nil {
		active = strings.TrimSpace(string(out))
	}

	configExists := false
	if _, err := os.Stat("/etc/haproxy/haproxy.cfg"); err == nil {
		configExists = true
	}

	writeJSON(w, map[string]any{
		"ok":            true,
		"status":        active,
		"config_exists": configExists,
	})
}

func generateHAProxyConfig(backends []NodeBackend) string {
	var b strings.Builder

	b.WriteString(`# KorisPanel HAProxy Config (auto-generated)
# TCP load balancer for OpenVPN connections
# Regenerate via: POST /api/admin/haproxy/apply

global
    log /dev/log local0
    maxconn 4096
    daemon

defaults
    log     global
    mode    tcp
    option  tcplog
    option  dontlognull
    timeout connect 10s
    timeout client  300s
    timeout server  300s
    retries 3

# ─── OpenVPN TCP (port 443) ───────────────────────────────────
frontend openvpn_tcp
    bind *:443
    default_backend openvpn_nodes

backend openvpn_nodes
    balance roundrobin
    option tcp-check
`)

	for _, nb := range backends {
		safeName := strings.ReplaceAll(strings.ReplaceAll(nb.Name, " ", "_"), ".", "_")
		b.WriteString(fmt.Sprintf("    server %s %s:%d check inter 30s fall 3 rise 2\n",
			safeName, nb.Host, nb.Port))
	}

	b.WriteString(`
# ─── Stats (optional, admin only) ────────────────────────────
frontend stats
    bind 127.0.0.1:8404
    mode http
    stats enable
    stats uri /stats
    stats refresh 10s
    stats admin if TRUE
`)

	return b.String()
}

type NodeBackend struct {
	ID   int64
	Name string
	Host string
	Port int
}
