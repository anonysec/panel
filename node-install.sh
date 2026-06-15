#!/usr/bin/env bash
#
# KorisPanel Node Agent Installer
# Usage: bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/node-install.sh)
# Or:    PANEL_URL=https://... NODE_TOKEN=xxx bash <(curl -Ls ...)
#

set -e

red='\033[0;31m'; green='\033[0;32m'; yellow='\033[0;33m'; blue='\033[0;34m'; cyan='\033[0;36m'; bold='\033[1m'; plain='\033[0m'
info()  { echo -e "${green}[INFO]${plain} $*"; }
warn()  { echo -e "${yellow}[WARN]${plain} $*"; }
fatal() { echo -e "${red}[FATAL]${plain} $*"; exit 1; }

[[ $EUID -ne 0 ]] && fatal "Run as root: sudo bash node-install.sh"
[[ -f /etc/os-release ]] && source /etc/os-release || fatal "Cannot detect OS."
OS=$ID

REPO="anonysec/panel"
BRANCH="main"
INSTALL_DIR="/opt/koris-next"

clear
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
[[ -z "${PANEL_URL:-}" ]] && read -rp "$(echo -e "${cyan}Panel URL: ${plain}")" PANEL_URL
[[ -z "$PANEL_URL" ]] && fatal "Panel URL required."
PANEL_URL="${PANEL_URL%/}"

[[ -z "${NODE_TOKEN:-}" ]] && read -rp "$(echo -e "${cyan}Node Token: ${plain}")" NODE_TOKEN
[[ -z "$NODE_TOKEN" ]] && fatal "Node Token required."

DEFAULT_NAME="$(hostname -s)"
[[ -z "${NODE_NAME:-}" ]] && read -rp "$(echo -e "${cyan}Node Name [${DEFAULT_NAME}]: ${plain}")" NODE_NAME
NODE_NAME="${NODE_NAME:-$DEFAULT_NAME}"

echo ""
info "Installing dependencies..."
export DEBIAN_FRONTEND=noninteractive
case "$OS" in
    ubuntu|debian)
        apt-get update -qq >/dev/null 2>&1
        apt-get install -y -qq git curl openssl golang-go iproute2 \
            openvpn easy-rsa strongswan xl2tpd >/dev/null 2>&1
        ;;
    centos|almalinux|rocky|rhel|fedora)
        dnf install -y -q git curl openssl golang iproute \
            openvpn easy-rsa strongswan xl2tpd >/dev/null 2>&1
        ;;
    *) fatal "Unsupported OS: $OS" ;;
esac

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

# Generate a unique RADIUS secret for this node
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
    mkdir -p /etc/openvpn/scripts
    cp -f "$INSTALL_DIR/scripts/openvpn/"*.sh /etc/openvpn/scripts/ 2>/dev/null || true
    chmod +x /etc/openvpn/scripts/*.sh 2>/dev/null || true
fi

# Service
cp "$INSTALL_DIR/node/systemd/node-agent.service" /etc/systemd/system/node-agent.service
systemctl daemon-reload
systemctl enable --now node-agent >/dev/null 2>&1
systemctl restart node-agent
sleep 2

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
echo -e "  ${cyan}Status:${plain}  $(systemctl is-active node-agent 2>/dev/null || echo unknown)"
echo -e "  ${cyan}Version:${plain} ${VERSION}"
echo -e "${bold}${green}───────────────────────────────────────────────${plain}"
echo -e "  ${cyan}Manage:${plain}  koris node-status | koris node-restart"
echo -e "  ${cyan}Logs:${plain}    journalctl -u node-agent -f"
echo -e "${bold}${green}═══════════════════════════════════════════════${plain}"
echo ""
echo -e "${yellow}The node should now appear in your panel under Services.${plain}"
