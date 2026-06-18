#!/usr/bin/env bash
# DEPRECATED: Use node-install.sh from the repository root instead.
# This script is kept for backward compatibility but receives no updates.
# Usage: bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/node-install.sh)
set -euo pipefail
[ "$(id -u)" = 0 ] || { echo "run as root"; exit 1; }
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PANEL_URL="${1:-${PANEL_URL:-}}"
NODE_TOKEN="${2:-${NODE_TOKEN:-}}"
NODE_NAME="${3:-${NODE_NAME:-node1}}"
[ -n "$PANEL_URL" ] || { echo "usage: install-node.sh PANEL_URL NODE_TOKEN [NODE_NAME]"; exit 1; }
[ -n "$NODE_TOKEN" ] || { echo "missing NODE_TOKEN"; exit 1; }

echo "[info] Installing Koris Next node skeleton..."
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq >/dev/null
apt-get install -y -qq golang-go curl openssl iproute2 >/dev/null
cd "$ROOT"
go mod tidy
go build -o /usr/local/bin/panel-node ./node/cmd/node
chmod +x /usr/local/bin/panel-node
mkdir -p /etc/panel-node /opt/KorisPanel
cat > /etc/panel-node/node.env <<ENV
PANEL_URL='${PANEL_URL}'
NODE_TOKEN='${NODE_TOKEN}'
NODE_NAME='${NODE_NAME}'
ENV
chmod 600 /etc/panel-node/node.env
cp "$ROOT/node/systemd/node-agent.service" /etc/systemd/system/node-agent.service
systemctl daemon-reload
systemctl enable --now node-agent.service
sleep 2
systemctl status node-agent --no-pager || true
