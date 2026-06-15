#!/usr/bin/env bash
#
# KorisPanel Installer
# Usage: bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/install.sh)
#

set -e

red='\033[0;31m'; green='\033[0;32m'; yellow='\033[0;33m'; blue='\033[0;34m'; cyan='\033[0;36m'; bold='\033[1m'; plain='\033[0m'
info()  { echo -e "${green}[INFO]${plain} $*"; }
warn()  { echo -e "${yellow}[WARN]${plain} $*"; }
error() { echo -e "${red}[ERROR]${plain} $*"; }
fatal() { echo -e "${red}[FATAL]${plain} $*"; exit 1; }
gen_secret() { openssl rand -hex "$1" 2>/dev/null || head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'; }

[[ $EUID -ne 0 ]] && fatal "Run as root: sudo bash install.sh"

# OS detect
[[ -f /etc/os-release ]] && source /etc/os-release || fatal "Cannot detect OS."
OS=$ID

REPO="anonysec/panel"
BRANCH="main"
INSTALL_DIR="/opt/koris-next"
CONFIG_DIR="/etc/panel"

# Banner
clear
echo -e "${bold}${blue}"
cat << 'EOF'
  ██╗  ██╗ ██████╗ ██████╗ ██╗███████╗
  ██║ ██╔╝██╔═══██╗██╔══██╗██║██╔════╝
  █████╔╝ ██║   ██║██████╔╝██║███████╗
  ██╔═██╗ ██║   ██║██╔══██╗██║╚════██║
  ██║  ██╗╚██████╔╝██║  ██║██║███████║
  ╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═╝╚═╝╚══════╝
                         Panel Installer
EOF
echo -e "${plain}"
echo -e "  ${cyan}OS:${plain} ${PRETTY_NAME:-$OS} ($(uname -m))"
echo ""

# Config prompts
read -rp "$(echo -e "${cyan}Panel port [8080]: ${plain}")" PANEL_PORT; PANEL_PORT="${PANEL_PORT:-8080}"
read -rp "$(echo -e "${cyan}Domain (blank for IP): ${plain}")" DOMAIN; DOMAIN="${DOMAIN:-_}"
read -rp "$(echo -e "${cyan}DB name [radius]: ${plain}")" DB_NAME; DB_NAME="${DB_NAME:-radius}"
read -rp "$(echo -e "${cyan}DB user [radius]: ${plain}")" DB_USER; DB_USER="${DB_USER:-radius}"
DB_PASS_DEFAULT="$(gen_secret 16)"
read -rp "$(echo -e "${cyan}DB pass [auto]: ${plain}")" DB_PASS; DB_PASS="${DB_PASS:-$DB_PASS_DEFAULT}"
SETUP_KEY="$(gen_secret 16)"
SESSION_SECRET="$(gen_secret 32)"

echo ""
info "Installing dependencies..."
export DEBIAN_FRONTEND=noninteractive
case "$OS" in
    ubuntu|debian)
        apt-get update -qq >/dev/null 2>&1
        apt-get install -y -qq git curl openssl ca-certificates mariadb-server \
            freeradius freeradius-mysql freeradius-utils nginx golang-go iproute2 >/dev/null 2>&1
        # Node.js for frontend build
        if ! command -v npm >/dev/null 2>&1; then
            curl -fsSL https://deb.nodesource.com/setup_20.x 2>/dev/null | bash - >/dev/null 2>&1 || true
            apt-get install -y -qq nodejs >/dev/null 2>&1 || true
        fi
        ;;
    centos|almalinux|rocky|rhel|fedora)
        dnf install -y -q git curl openssl ca-certificates mariadb-server \
            freeradius freeradius-mysql freeradius-utils nginx golang iproute >/dev/null 2>&1
        ;;
    *) fatal "Unsupported OS: $OS" ;;
esac
info "Dependencies installed."

# Database
info "Setting up MariaDB..."
systemctl enable --now mariadb >/dev/null 2>&1
mysql -u root <<SQL
CREATE DATABASE IF NOT EXISTS ${DB_NAME} CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS '${DB_USER}'@'localhost' IDENTIFIED BY '${DB_PASS}';
ALTER USER '${DB_USER}'@'localhost' IDENTIFIED BY '${DB_PASS}';
GRANT ALL PRIVILEGES ON ${DB_NAME}.* TO '${DB_USER}'@'localhost';
FLUSH PRIVILEGES;
SQL
SCHEMA="/etc/freeradius/3.0/mods-config/sql/main/mysql/schema.sql"
if [[ -f "$SCHEMA" ]]; then
    mysql -u root "$DB_NAME" -N -B -e "SHOW TABLES LIKE 'radcheck';" 2>/dev/null | grep -q '^radcheck$' || mysql -u root "$DB_NAME" < "$SCHEMA"
fi
info "Database ready."

# FreeRADIUS
info "Configuring FreeRADIUS..."
SQL_MOD="/etc/freeradius/3.0/mods-available/sql"
if [[ -f "$SQL_MOD" ]]; then
    sed -i -e 's/^\s*dialect = .*/\tdialect = "mysql"/' \
           -e "s/^\s*login = .*/\tlogin = \"${DB_USER}\"/" \
           -e "s/^\s*password = .*/\tpassword = \"${DB_PASS}\"/" \
           -e "s/^\s*radius_db = .*/\tradius_db = \"${DB_NAME}\"/" "$SQL_MOD"
    ln -sf ../mods-available/sql /etc/freeradius/3.0/mods-enabled/sql 2>/dev/null || true
    systemctl restart freeradius >/dev/null 2>&1 || true
fi

# Clone/Update repo
info "Downloading KorisPanel..."
if [[ -d "$INSTALL_DIR/.git" ]]; then
    cd "$INSTALL_DIR" && git fetch origin "$BRANCH" --depth=1 >/dev/null 2>&1 && git reset --hard "origin/$BRANCH" >/dev/null 2>&1
else
    rm -rf "$INSTALL_DIR"
    git clone --depth=1 -b "$BRANCH" "https://github.com/${REPO}.git" "$INSTALL_DIR" >/dev/null 2>&1
fi
cd "$INSTALL_DIR"
VERSION="$(cat VERSION 2>/dev/null || echo dev)"
info "Source ready (v${VERSION})."

# Build binary
info "Building panel binary..."
go mod tidy >/dev/null 2>&1
go build -ldflags="-s -w" -o /usr/local/bin/panel ./panel/cmd/panel
chmod +x /usr/local/bin/panel

# Build frontend
if command -v npm >/dev/null 2>&1; then
    info "Building admin frontend..."
    (cd panel/web/admin && npm install --no-audit --no-fund --silent 2>/dev/null && npm run build >/dev/null 2>&1) || true
    info "Building portal frontend..."
    (cd panel/web/portal && npm install --no-audit --no-fund --silent 2>/dev/null && npm run build >/dev/null 2>&1) || true
else
    info "Using prebuilt frontend assets."
fi

# Install files
mkdir -p /opt/koris-next/panel/web/admin /opt/koris-next/panel/web/portal
cp -a "$INSTALL_DIR/panel/migrations" /opt/koris-next/panel/migrations 2>/dev/null || true
cp -a "$INSTALL_DIR/panel/web/admin/www" /opt/koris-next/panel/web/admin/www 2>/dev/null || true
cp -a "$INSTALL_DIR/panel/web/portal/www" /opt/koris-next/panel/web/portal/www 2>/dev/null || true

# Config
info "Writing configuration..."
PANEL_ADDR="127.0.0.1:${PANEL_PORT}"
mkdir -p "$CONFIG_DIR"
cat > "${CONFIG_DIR}/panel.env" <<ENV
PANEL_ADDR='${PANEL_ADDR}'
PANEL_DB_DSN='${DB_USER}:${DB_PASS}@tcp(127.0.0.1:3306)/${DB_NAME}?parseTime=true&multiStatements=true&charset=utf8mb4,utf8'
PANEL_MIGRATIONS='/opt/koris-next/panel/migrations'
PANEL_SETUP_KEY='${SETUP_KEY}'
PANEL_SESSION_SECRET='${SESSION_SECRET}'
PANEL_PUBLIC_BASE='/dashboard'
PANEL_ADMIN_WEB_DIR='/opt/koris-next/panel/web/admin/www'
PANEL_PORTAL_WEB_DIR='/opt/koris-next/panel/web/portal/www'
PANEL_VERSION='${VERSION}'
ENV
chmod 600 "${CONFIG_DIR}/panel.env"

# Systemd
info "Installing service..."
cp "$INSTALL_DIR/panel/systemd/panel.service" /etc/systemd/system/panel.service
systemctl daemon-reload
systemctl enable --now panel >/dev/null 2>&1
systemctl restart panel
sleep 2

# Health
if curl -fsS "http://${PANEL_ADDR}/api/health" >/dev/null 2>&1; then
    info "Health check ${green}PASSED${plain}"
else
    warn "Health check failed — checking logs:"
    journalctl -u panel -n 20 --no-pager
fi

# Nginx
info "Configuring Nginx..."
cat > /etc/nginx/sites-available/koris-panel.conf <<NGINX
server {
    listen 80 default_server;
    server_name ${DOMAIN};
    client_max_body_size 20m;
    location = / { return 302 /dashboard/; }
    location = /dashboard { return 302 /dashboard/; }
    location /dashboard/ { proxy_pass http://${PANEL_ADDR}; proxy_set_header Host \$host; proxy_set_header X-Real-IP \$remote_addr; proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for; proxy_set_header X-Forwarded-Proto \$scheme; }
    location /api/ { proxy_pass http://${PANEL_ADDR}; proxy_http_version 1.1; proxy_set_header Upgrade \$http_upgrade; proxy_set_header Connection "upgrade"; proxy_set_header Host \$host; proxy_set_header X-Real-IP \$remote_addr; proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for; proxy_set_header X-Forwarded-Proto \$scheme; }
    location = /portal { return 302 /portal/; }
    location /portal/ { proxy_pass http://${PANEL_ADDR}; proxy_set_header Host \$host; proxy_set_header X-Real-IP \$remote_addr; proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for; proxy_set_header X-Forwarded-Proto \$scheme; }
}
NGINX
rm -f /etc/nginx/sites-enabled/default 2>/dev/null
ln -sf /etc/nginx/sites-available/koris-panel.conf /etc/nginx/sites-enabled/koris-panel.conf
nginx -t >/dev/null 2>&1 && systemctl reload nginx

# Management CLI
cp "$INSTALL_DIR/koris.sh" /usr/local/bin/koris 2>/dev/null || true
chmod +x /usr/local/bin/koris 2>/dev/null || true

# Result
SERVER_IP=$(curl -fsS4 --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')
echo ""
echo -e "${bold}${green}═══════════════════════════════════════════════${plain}"
echo -e "${bold}${green}     KorisPanel Installed Successfully!${plain}"
echo -e "${bold}${green}═══════════════════════════════════════════════${plain}"
echo -e "  ${cyan}Dashboard:${plain}  http://${SERVER_IP}/dashboard/"
echo -e "  ${cyan}Portal:${plain}     http://${SERVER_IP}/portal/"
echo -e "  ${cyan}Setup Key:${plain}  ${yellow}${SETUP_KEY}${plain}"
echo -e "  ${cyan}DB Pass:${plain}    ${DB_PASS}"
echo -e "  ${cyan}Version:${plain}    ${VERSION}"
echo -e "${bold}${green}───────────────────────────────────────────────${plain}"
echo -e "  ${cyan}Manage:${plain}     koris"
echo -e "  ${cyan}Commands:${plain}   koris status|restart|update|logs|uninstall"
echo -e "${bold}${green}═══════════════════════════════════════════════${plain}"
echo ""
echo -e "${yellow}Open the Dashboard and use the Setup Key to create your admin account.${plain}"
