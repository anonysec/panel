#!/usr/bin/env bash
#
# KorisPanel Node Agent Installer
# Usage: bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/node-install.sh)
# Or:    PANEL_URL=https://... NODE_TOKEN=xxx bash <(curl -Ls ...)
#

set -e

export TERM="${TERM:-xterm}"

red='\033[0;31m'; green='\033[0;32m'; yellow='\033[0;33m'; blue='\033[0;34m'; cyan='\033[0;36m'; bold='\033[1m'; plain='\033[0m'
info()  { echo -e "${green}[INFO]${plain} $*"; }
warn()  { echo -e "${yellow}[WARN]${plain} $*"; }
fatal() { echo -e "${red}[FATAL]${plain} $*"; exit 1; }

[[ $EUID -ne 0 ]] && fatal "Run as root: sudo bash node-install.sh"
[[ -f /etc/os-release ]] && source /etc/os-release || fatal "Cannot detect OS."
OS=$ID

REPO="anonysec/panel"
BRANCH="main"
INSTALL_DIR="/opt/KorisPanel"

clear 2>/dev/null || true
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
echo -e "  ${cyan}OS:${plain} ${PRETTY_NAME:-$OS} ($(uname -m))"
echo ""

# Config
[[ -z "${PANEL_URL:-}" ]] && read -rp "$(echo -e "${cyan}Panel URL: ${plain}")" PANEL_URL </dev/tty
[[ -z "$PANEL_URL" ]] && fatal "Panel URL required."
PANEL_URL="${PANEL_URL%/}"

# Verify panel is reachable before continuing
info "Checking panel connectivity..."
HEALTH_RESPONSE=$(curl -fsSL --max-time 10 "$PANEL_URL/api/health" 2>/dev/null) || true
if [[ -z "$HEALTH_RESPONSE" ]] || ! echo "$HEALTH_RESPONSE" | grep -qi "ok"; then
    fatal "Cannot reach panel at $PANEL_URL - verify the URL and ensure the panel is running."
fi
info "Panel is reachable."

[[ -z "${NODE_TOKEN:-}" ]] && read -rp "$(echo -e "${cyan}Node Token: ${plain}")" NODE_TOKEN </dev/tty
[[ -z "$NODE_TOKEN" ]] && fatal "Node Token required."

DEFAULT_NAME="$(hostname -s)"
[[ -z "${NODE_NAME:-}" ]] && read -rp "$(echo -e "${cyan}Node Name [${DEFAULT_NAME}]: ${plain}")" NODE_NAME </dev/tty
NODE_NAME="${NODE_NAME:-$DEFAULT_NAME}"

echo ""
info "Installing dependencies..."
export DEBIAN_FRONTEND=noninteractive
case "$OS" in
    ubuntu|debian)
        apt-get update -qq >/dev/null 2>&1
        apt-get install -y -qq git curl openssl iproute2 \
            openvpn easy-rsa strongswan xl2tpd wireguard-tools >/dev/null 2>&1
        ;;
    centos|almalinux|rocky|rhel|fedora)
        dnf install -y -q git curl openssl iproute \
            openvpn easy-rsa strongswan xl2tpd wireguard-tools >/dev/null 2>&1
        ;;
    *) fatal "Unsupported OS: $OS" ;;
esac

# Ensure Go >= 1.21 is available
GO_REQUIRED_MAJOR=1
GO_REQUIRED_MINOR=21
install_go() {
    local ARCH
    case "$(uname -m)" in
        x86_64)        ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        armv7l)        ARCH="armv6l" ;;
        *)             ARCH="amd64" ;;
    esac
    local GO_VERSION="1.22.5"
    local GO_URL="https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz"
    local GO_TARBALL="/tmp/go${GO_VERSION}.linux-${ARCH}.tar.gz"
    info "Downloading Go ${GO_VERSION} for ${ARCH}..."
    curl -fsSL -o "$GO_TARBALL" "$GO_URL"

    # Verify SHA256 checksum if sha256sum is available (best-effort)
    if command -v sha256sum &>/dev/null; then
        local EXPECTED_HASH
        EXPECTED_HASH=$(curl -fsSL "https://go.dev/dl/?mode=json&include=all" 2>/dev/null \
            | grep -A 5 "go${GO_VERSION}.linux-${ARCH}" \
            | grep -oP '"sha256":\s*"\K[a-f0-9]+' || true)
        if [[ -n "$EXPECTED_HASH" ]]; then
            local ACTUAL_HASH
            ACTUAL_HASH=$(sha256sum "$GO_TARBALL" | awk '{print $1}')
            if [[ "$ACTUAL_HASH" != "$EXPECTED_HASH" ]]; then
                rm -f "$GO_TARBALL"
                fatal "Go binary checksum mismatch! Expected: $EXPECTED_HASH, Got: $ACTUAL_HASH"
            fi
            info "Go binary checksum verified."
        else
            warn "Could not fetch Go checksum for verification. Proceeding with install."
        fi
    fi

    rm -rf /usr/local/go
    tar -C /usr/local -xzf "$GO_TARBALL"
    rm -f "$GO_TARBALL"
    export PATH="/usr/local/go/bin:$PATH"
}

if command -v go &>/dev/null; then
    GO_VER=$(go version | grep -oP '\d+\.\d+' | head -1)
    GO_MAJOR=$(echo "$GO_VER" | cut -d. -f1)
    GO_MINOR=$(echo "$GO_VER" | cut -d. -f2)
    if [[ "$GO_MAJOR" -lt "$GO_REQUIRED_MAJOR" ]] || \
       { [[ "$GO_MAJOR" -eq "$GO_REQUIRED_MAJOR" ]] && [[ "$GO_MINOR" -lt "$GO_REQUIRED_MINOR" ]]; }; then
        warn "Go ${GO_VER} found but >= ${GO_REQUIRED_MAJOR}.${GO_REQUIRED_MINOR} required."
        install_go
    else
        info "Go ${GO_VER} found (meets requirement)."
    fi
else
    install_go
fi
# Ensure Go is on PATH for this session
export PATH="/usr/local/go/bin:$PATH"

info "Downloading source..."
if [[ -d "$INSTALL_DIR/.git" ]]; then
    cd "$INSTALL_DIR" && git fetch origin "$BRANCH" --depth=1 >/dev/null 2>&1 && git reset --hard "origin/$BRANCH" >/dev/null 2>&1
else
    rm -rf "$INSTALL_DIR"
    git clone --depth=1 -b "$BRANCH" "https://github.com/${REPO}.git" "$INSTALL_DIR" >/dev/null 2>&1
fi
cd "$INSTALL_DIR"
VERSION="$(cat VERSION 2>/dev/null || echo dev)"

info "Building node agent..."
go mod tidy >/dev/null 2>&1
go build -ldflags="-s -w" -o /usr/local/bin/panel-node ./node/cmd/node
chmod +x /usr/local/bin/panel-node

# Detect node public IP
NODE_IP=$(curl -fsS4 --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')

# RADIUS_SECRET: Used for RADIUS authentication between this node's VPN services
# and the panel's FreeRADIUS server. The panel auto-configures this secret for the
# node when it first connects. It must remain consistent across restarts.
RADIUS_SECRET="$(openssl rand -hex 16)"

# Config
mkdir -p /etc/panel-node
cat > /etc/panel-node/node.env <<ENV
PANEL_URL='${PANEL_URL}'
NODE_TOKEN='${NODE_TOKEN}'
NODE_NAME='${NODE_NAME}'
KORIS_RADIUS_SECRET='${RADIUS_SECRET}'
KORIS_NAS_IP='${NODE_IP}'
ENV
chmod 600 /etc/panel-node/node.env

# VPN scripts
if [[ -d "$INSTALL_DIR/scripts/openvpn" ]]; then
    mkdir -p /etc/openvpn/server
    cp -f "$INSTALL_DIR/scripts/openvpn/"*.sh /etc/openvpn/server/ 2>/dev/null || true
    chmod +x /etc/openvpn/server/*.sh 2>/dev/null || true
fi

# Log rotation and directories
mkdir -p /var/log/openvpn/
chmod 0750 /var/log/openvpn/
chown root:root /var/log/openvpn/

# ═══════════════════════════════════════════════════════════════════════
# OpenVPN Server Setup (UDP + TCP dual-stack)
# ═══════════════════════════════════════════════════════════════════════
info "Setting up OpenVPN server instances..."

# Install radclient (used by koris-radius-auth.sh)
case "$OS" in
    ubuntu|debian)
        apt-get install -y -qq freeradius-utils >/dev/null 2>&1 || true
        ;;
    centos|almalinux|rocky|rhel|fedora)
        dnf install -y -q freeradius-utils >/dev/null 2>&1 || true
        ;;
esac

# Initialize Easy-RSA PKI (only if not already done)
if [[ ! -f /etc/openvpn/server/ca.crt ]]; then
    info "Initializing PKI certificates..."
    EASYRSA_DIR="/etc/openvpn/easy-rsa"
    mkdir -p "$EASYRSA_DIR"
    cp -r /usr/share/easy-rsa/* "$EASYRSA_DIR/" 2>/dev/null || true
    cd "$EASYRSA_DIR"
    ./easyrsa --batch init-pki
    EASYRSA_REQ_CN="KorisVPN-CA" ./easyrsa --batch build-ca nopass
    ./easyrsa --batch build-server-full server nopass
    ./easyrsa --batch gen-dh
    openvpn --genkey secret /etc/openvpn/server/tc.key
    cp pki/ca.crt pki/issued/server.crt pki/private/server.key pki/dh.pem /etc/openvpn/server/
    cd "$INSTALL_DIR"
    info "PKI certificates generated."
else
    info "PKI certificates already exist, skipping."
fi

# UDP OpenVPN server config (port 1194, subnet 10.8.0.0/24)
cat > /etc/openvpn/server/server-udp.conf <<'OVPN_UDP'
port 1194
proto udp
dev tun0
ca /etc/openvpn/server/ca.crt
cert /etc/openvpn/server/server.crt
key /etc/openvpn/server/server.key
dh /etc/openvpn/server/dh.pem
tls-crypt /etc/openvpn/server/tc.key
topology subnet
server 10.8.0.0 255.255.255.0
push "redirect-gateway def1 bypass-dhcp"
push "dhcp-option DNS 1.1.1.1"
push "dhcp-option DNS 8.8.8.8"
keepalive 10 120
cipher AES-256-GCM
auth SHA256
max-clients 4093
user nobody
group nogroup
persist-key
persist-tun
verb 3
status /var/log/openvpn/status-udp.log
log-append /var/log/openvpn/openvpn-udp.log
script-security 3
auth-user-pass-verify /etc/openvpn/server/koris-radius-auth.sh via-file
client-connect /etc/openvpn/server/koris-client-connect.sh
client-disconnect /etc/openvpn/server/koris-client-disconnect.sh
username-as-common-name
verify-client-cert none
OVPN_UDP

# TCP OpenVPN server config (port 443, subnet 10.8.1.0/24)
cat > /etc/openvpn/server/server-tcp.conf <<'OVPN_TCP'
port 443
proto tcp
dev tun1
ca /etc/openvpn/server/ca.crt
cert /etc/openvpn/server/server.crt
key /etc/openvpn/server/server.key
dh /etc/openvpn/server/dh.pem
tls-crypt /etc/openvpn/server/tc.key
topology subnet
server 10.8.1.0 255.255.255.0
push "redirect-gateway def1 bypass-dhcp"
push "dhcp-option DNS 1.1.1.1"
push "dhcp-option DNS 8.8.8.8"
keepalive 10 120
cipher AES-256-GCM
auth SHA256
max-clients 4093
user nobody
group nogroup
persist-key
persist-tun
verb 3
status /var/log/openvpn/status-tcp.log
log-append /var/log/openvpn/openvpn-tcp.log
script-security 3
auth-user-pass-verify /etc/openvpn/server/koris-radius-auth.sh via-file
client-connect /etc/openvpn/server/koris-client-connect.sh
client-disconnect /etc/openvpn/server/koris-client-disconnect.sh
username-as-common-name
verify-client-cert none
OVPN_TCP

# Enable IP forwarding
if ! grep -qs "net.ipv4.ip_forward=1" /etc/sysctl.d/99-koris-vpn.conf 2>/dev/null; then
    echo "net.ipv4.ip_forward=1" > /etc/sysctl.d/99-koris-vpn.conf
    sysctl -p /etc/sysctl.d/99-koris-vpn.conf >/dev/null 2>&1
fi

# NAT rules for both VPN subnets
DEFAULT_IFACE=$(ip route show default | awk '/default/ {print $5}' | head -1)
if [[ -n "$DEFAULT_IFACE" ]]; then
    iptables -t nat -C POSTROUTING -s 10.8.0.0/24 -o "$DEFAULT_IFACE" -j MASQUERADE 2>/dev/null || \
    iptables -t nat -A POSTROUTING -s 10.8.0.0/24 -o "$DEFAULT_IFACE" -j MASQUERADE

    iptables -t nat -C POSTROUTING -s 10.8.1.0/24 -o "$DEFAULT_IFACE" -j MASQUERADE 2>/dev/null || \
    iptables -t nat -A POSTROUTING -s 10.8.1.0/24 -o "$DEFAULT_IFACE" -j MASQUERADE

    # Persist iptables rules
    apt-get install -y -qq iptables-persistent >/dev/null 2>&1 || true
    netfilter-persistent save >/dev/null 2>&1 || true
else
    warn "Could not detect default network interface — NAT rules not applied."
    warn "You may need to manually add iptables MASQUERADE rules."
fi

# Enable and start both OpenVPN instances
systemctl enable openvpn-server@server-udp >/dev/null 2>&1 || true
systemctl enable openvpn-server@server-tcp >/dev/null 2>&1 || true
systemctl restart openvpn-server@server-udp || warn "Failed to start OpenVPN UDP. Check: journalctl -u openvpn-server@server-udp"
systemctl restart openvpn-server@server-tcp || warn "Failed to start OpenVPN TCP. Check: journalctl -u openvpn-server@server-tcp"

info "OpenVPN dual-stack setup complete (UDP:1194, TCP:443)."
mkdir -p /var/log/panel-node/
chmod 0750 /var/log/panel-node/
chown root:root /var/log/panel-node/
cp -f "$INSTALL_DIR/scripts/logrotate/koris-openvpn" /etc/logrotate.d/koris-openvpn
cp -f "$INSTALL_DIR/scripts/logrotate/koris-node-agent" /etc/logrotate.d/koris-node-agent
chmod 644 /etc/logrotate.d/koris-openvpn
chmod 644 /etc/logrotate.d/koris-node-agent

# Service
cp "$INSTALL_DIR/node/systemd/node-agent.service" /etc/systemd/system/node-agent.service
systemctl daemon-reload
systemctl enable --now node-agent >/dev/null 2>&1
systemctl restart node-agent
sleep 2

# Post-install health check
info "Verifying node agent status..."
AGENT_STATUS=$(systemctl is-active node-agent 2>/dev/null || echo "inactive")
if [[ "$AGENT_STATUS" != "active" ]]; then
    warn "node-agent service is not active (status: $AGENT_STATUS)."
    warn "Troubleshooting: check logs with 'journalctl -u node-agent -n 50'"
    warn "Try restarting: systemctl restart node-agent"
fi

# Verify panel registration
info "Checking panel registration..."
REG_RESPONSE=$(curl -fsSL --max-time 5 -H "X-Node-Token: $NODE_TOKEN" "$PANEL_URL/api/node/agent/version" 2>/dev/null) || true
if [[ -z "$REG_RESPONSE" ]]; then
    warn "Could not verify panel registration. The node may need a moment to register."
    warn "Check panel dashboard under Nodes to see if this node appears."
    warn "If it does not appear, verify NODE_TOKEN is correct and restart: systemctl restart node-agent"
else
    info "Panel registration verified successfully."
fi

# CLI
cp "$INSTALL_DIR/koris.sh" /usr/local/bin/koris 2>/dev/null || true
chmod +x /usr/local/bin/koris 2>/dev/null || true

echo ""
echo -e "${bold}${green}═══════════════════════════════════════════════${plain}"
echo -e "${bold}${green}     Node Agent Installed!${plain}"
echo -e "${bold}${green}═══════════════════════════════════════════════${plain}"
echo -e "  ${cyan}Node:${plain}    ${NODE_NAME}"
echo -e "  ${cyan}IP:${plain}      ${NODE_IP}"
echo -e "  ${cyan}Panel:${plain}   ${PANEL_URL}"
echo -e "  ${cyan}Agent:${plain}   $(systemctl is-active node-agent 2>/dev/null || echo unknown)"
echo -e "  ${cyan}OpenVPN:${plain} UDP=$(systemctl is-active openvpn-server@server-udp 2>/dev/null || echo unknown) TCP=$(systemctl is-active openvpn-server@server-tcp 2>/dev/null || echo unknown)"
echo -e "  ${cyan}Version:${plain} ${VERSION}"
echo -e "${bold}${green}───────────────────────────────────────────────${plain}"
echo -e "  ${cyan}Manage:${plain}  koris node-status | koris node-restart"
echo -e "  ${cyan}Logs:${plain}    journalctl -u node-agent -f"
echo -e "  ${cyan}VPN:${plain}     journalctl -u openvpn-server@server-udp -f"
echo -e "           journalctl -u openvpn-server@server-tcp -f"
echo -e "${bold}${green}═══════════════════════════════════════════════${plain}"
echo ""
echo -e "${yellow}The node should now appear in your panel under Services.${plain}"
