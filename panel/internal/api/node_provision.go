package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"KorisPanel/panel/internal/auth"
)

// nodeProvision handles /api/admin/nodes/provision
// GET: generates a fresh API token with an install command (existing behavior).
// POST: accepts SSH credentials and starts auto-provisioning via SSH.
func (s *Server) nodeProvision(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleNodeProvisionSSH(w, r)
		return
	case http.MethodGet:
		// continue below
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse optional query params
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	protocols := strings.TrimSpace(r.URL.Query().Get("protocols"))

	// Default node name if not provided
	if name == "" {
		name = "node-" + randomHex(4)
	}

	// Validate name length
	if len(name) > 64 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_too_long"})
		return
	}

	// Generate a fresh API token
	token := "kn_" + auth.RandomToken(24)
	tokenHash := hashToken(token)

	// Create the node record with status='pending'
	res, err := s.DB.Exec(
		`INSERT INTO nodes(name, public_ip, api_token_hash, status) VALUES($1, '', $2, 'offline')`,
		name, tokenHash,
	)
	if err != nil {
		log.Printf("[provision] failed to create node: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	nodeID, _ := res.LastInsertId()

	// If protocols specified, store them as tags
	if protocols != "" {
		for _, proto := range strings.Split(protocols, ",") {
			proto = strings.TrimSpace(proto)
			if proto != "" {
				_, _ = s.DB.Exec(
					`INSERT INTO node_tags (node_id, tag) VALUES ($1, $2) ON CONFLICT (node_id, tag) DO NOTHING`,
					nodeID, "protocol:"+proto,
				)
			}
		}
	}

	// Determine the panel URL from settings or request
	panelURL := s.getPanelURL(r)

	// Build install command and URL
	installCommand := fmt.Sprintf(
		`curl -sSL %s/api/node/install.sh | PANEL_URL=%s NODE_TOKEN=%s NODE_NAME=%s bash`,
		panelURL, panelURL, token, name,
	)
	installURL := fmt.Sprintf(
		"%s/api/node/install.sh?token=%s&panel_url=%s",
		panelURL, token, panelURL,
	)

	// Audit log
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "node.provisioned", "node", strconv.FormatInt(nodeID, 10), nil, map[string]any{
		"name":      name,
		"protocols": protocols,
	}, clientIP(r))

	if s.Cache != nil {
		s.Cache.InvalidatePrefix("nodes:")
	}

	log.Printf("[provision] node provisioned id=%d name=%s by=%s", nodeID, name, actor)

	writeJSON(w, map[string]any{
		"ok":              true,
		"node_id":         nodeID,
		"token":           token,
		"install_command": installCommand,
		"install_url":     installURL,
	})
}

// nodeInstallScript handles GET /api/node/install.sh
// It serves the node installation script. This is a public endpoint so nodes
// can curl it without authentication. The token is passed as an env var to bash.
func (s *Server) nodeInstallScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Optional query params (for install_url with embedded params)
	token := r.URL.Query().Get("token")
	panelURL := r.URL.Query().Get("panel_url")

	// Build the script with optional pre-filled values
	script := generateInstallScript(token, panelURL)

	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="install.sh"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(script))
}

// getPanelURL determines the panel's public URL from panel_settings or falls
// back to the request's Host header.
func (s *Server) getPanelURL(r *http.Request) string {
	// Try panel_settings first
	var domain string
	_ = s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key='panel_domain'`).Scan(&domain)
	if domain != "" {
		scheme := "https"
		return scheme + "://" + domain
	}

	// Fallback: derive from the request
	scheme := "https"
	if r.TLS == nil {
		if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
			scheme = fwd
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + r.Host
}

// randomHex generates n random bytes and returns them as a hex string.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// generateInstallScript produces a node installation bash script.
// If token and panelURL are provided (via query params), they are embedded.
// Otherwise the script prompts for them interactively.
func generateInstallScript(token, panelURL string) string {
	// Pre-fill environment variables if provided via URL
	envBlock := ""
	if token != "" {
		envBlock += fmt.Sprintf("NODE_TOKEN=${NODE_TOKEN:-%s}\n", token)
	} else {
		envBlock += `NODE_TOKEN="${NODE_TOKEN:-}"` + "\n"
	}
	if panelURL != "" {
		envBlock += fmt.Sprintf("PANEL_URL=${PANEL_URL:-%s}\n", panelURL)
	} else {
		envBlock += `PANEL_URL="${PANEL_URL:-}"` + "\n"
	}

	return `#!/usr/bin/env bash
#
# KorisPanel Node Agent Installer (Provisioning)
# Generated by the panel provisioning wizard.
#
# Usage:
#   curl -sSL https://panel.example.com/api/node/install.sh | PANEL_URL=... NODE_TOKEN=... bash
#

set -e

export TERM="${TERM:-xterm}"

red='\033[0;31m'; green='\033[0;32m'; yellow='\033[0;33m'; blue='\033[0;34m'; cyan='\033[0;36m'; bold='\033[1m'; plain='\033[0m'
info()  { echo -e "${green}[INFO]${plain} $*"; }
warn()  { echo -e "${yellow}[WARN]${plain} $*"; }
fatal() { echo -e "${red}[FATAL]${plain} $*"; exit 1; }

[[ $EUID -ne 0 ]] && fatal "Run as root: sudo bash install.sh"

# Configuration (can be overridden via environment variables)
` + envBlock + `
[[ -z "$PANEL_URL" ]] && read -rp "$(echo -e "${cyan}Panel URL: ${plain}")" PANEL_URL </dev/tty
[[ -z "$PANEL_URL" ]] && fatal "Panel URL is required."
PANEL_URL="${PANEL_URL%/}"

[[ -z "$NODE_TOKEN" ]] && read -rp "$(echo -e "${cyan}Node Token: ${plain}")" NODE_TOKEN </dev/tty
[[ -z "$NODE_TOKEN" ]] && fatal "Node Token is required."

NODE_NAME="${NODE_NAME:-$(hostname -s)}"

echo ""
echo -e "${bold}${blue}"
cat << 'EOF'
  ██╗  ██╗ ██████╗ ██████╗ ██╗███████╗
  ██║ ██╔╝██╔═══██╗██╔══██╗██║██╔════╝
  █████╔╝ ██║   ██║██████╔╝██║███████╗
  ██╔═██╗ ██║   ██║██╔══██╗██║╚════██║
  ██║  ██╗╚██████╔╝██║  ██║██║███████║
  ╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═╝╚═╝╚══════╝
                     Node Agent Installer
EOF
echo -e "${plain}"
info "Panel URL:  $PANEL_URL"
info "Node Name:  $NODE_NAME"
echo ""

# Verify panel connectivity
info "Checking panel connectivity..."
HEALTH_RESPONSE=$(curl -fsSL --max-time 10 "$PANEL_URL/api/health" 2>/dev/null) || true
if [[ -z "$HEALTH_RESPONSE" ]] || ! echo "$HEALTH_RESPONSE" | grep -qi "ok"; then
    fatal "Cannot reach panel at $PANEL_URL - verify the URL and ensure the panel is running."
fi
info "Panel is reachable."

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)        BINARY_ARCH="amd64" ;;
    aarch64|arm64) BINARY_ARCH="arm64" ;;
    armv7l)        BINARY_ARCH="armv6l" ;;
    *)             BINARY_ARCH="amd64" ;;
esac

# Download the node agent binary from the panel
info "Downloading node agent binary..."
DOWNLOAD_URL="$PANEL_URL/api/node/agent/download"
BINARY_PATH="/usr/local/bin/knode"

HTTP_CODE=$(curl -sSL -o "$BINARY_PATH" -w "%{http_code}" \
    -H "X-Node-Token: $NODE_TOKEN" \
    "$DOWNLOAD_URL" 2>/dev/null) || true

if [[ "$HTTP_CODE" != "200" ]]; then
    warn "Could not download binary from panel (HTTP $HTTP_CODE)."
    warn "The node agent binary may need to be installed manually."
    warn "See: https://github.com/anonysec/panel for instructions."
fi

if [[ -f "$BINARY_PATH" ]]; then
    chmod +x "$BINARY_PATH"
    info "Node agent binary installed at $BINARY_PATH"
else
    warn "Binary not found at $BINARY_PATH — skipping binary setup."
fi

# Write configuration
info "Writing configuration..."
mkdir -p /etc/knode
cat > /etc/knode/node.env <<ENV
PANEL_URL='${PANEL_URL}'
NODE_TOKEN='${NODE_TOKEN}'
NODE_NAME='${NODE_NAME}'
NODE_INTERVAL=10
LOG_LEVEL=info
NODE_AUTO_UPDATE=true
ENV
chmod 600 /etc/knode/node.env
info "Configuration written to /etc/knode/node.env"

# Create systemd service
info "Creating systemd service..."
cat > /etc/systemd/system/knode.service <<'SERVICE'
[Unit]
Description=KorisPanel Node Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/knode
EnvironmentFile=/etc/knode/node.env
Restart=always
RestartSec=5
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
SERVICE

# Enable and start the service
systemctl daemon-reload
systemctl enable knode >/dev/null 2>&1
systemctl restart knode
sleep 2

# Verify service status
AGENT_STATUS=$(systemctl is-active knode 2>/dev/null || echo "inactive")
if [[ "$AGENT_STATUS" == "active" ]]; then
    info "Node agent service is running."
else
    warn "Node agent service is not active (status: $AGENT_STATUS)."
    warn "Check logs: journalctl -u knode -n 50"
fi

# Verify panel registration
info "Verifying panel registration..."
REG_RESPONSE=$(curl -fsSL --max-time 5 -H "X-Node-Token: $NODE_TOKEN" "$PANEL_URL/api/node/agent/version" 2>/dev/null) || true
if [[ -n "$REG_RESPONSE" ]] && echo "$REG_RESPONSE" | grep -qi "ok"; then
    info "Panel registration verified."
else
    warn "Could not verify registration. The node should appear in the panel shortly."
fi

echo ""
echo -e "${bold}${green}═══════════════════════════════════════════════${plain}"
echo -e "${bold}${green}     Node Agent Installed!${plain}"
echo -e "${bold}${green}═══════════════════════════════════════════════${plain}"
echo -e "  ${cyan}Node:${plain}    ${NODE_NAME}"
echo -e "  ${cyan}Panel:${plain}   ${PANEL_URL}"
echo -e "  ${cyan}Agent:${plain}   ${AGENT_STATUS}"
echo -e "${bold}${green}───────────────────────────────────────────────${plain}"
echo -e "  ${cyan}Logs:${plain}    journalctl -u knode -f"
echo -e "  ${cyan}Status:${plain}  systemctl status knode"
echo -e "  ${cyan}Restart:${plain} systemctl restart knode"
echo -e "${bold}${green}═══════════════════════════════════════════════${plain}"
echo ""
`
}
