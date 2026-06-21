#!/usr/bin/env bash
#
# KorisPanel Installer
# Usage: bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/install.sh)
#

set -e

export TERM="${TERM:-xterm}"

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
INSTALL_DIR="/opt/KorisPanel"
CONFIG_DIR="/etc/panel"

# Banner
clear 2>/dev/null || true
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
read -rp "$(echo -e "${cyan}Panel port [8080]: ${plain}")" PANEL_PORT </dev/tty; PANEL_PORT="${PANEL_PORT:-8080}"
read -rp "$(echo -e "${cyan}Domain (blank for IP): ${plain}")" DOMAIN </dev/tty; DOMAIN="${DOMAIN:-_}"
read -rp "$(echo -e "${cyan}DB name [radius]: ${plain}")" DB_NAME </dev/tty; DB_NAME="${DB_NAME:-radius}"
read -rp "$(echo -e "${cyan}DB user [radius]: ${plain}")" DB_USER </dev/tty; DB_USER="${DB_USER:-radius}"
DB_PASS_DEFAULT="$(gen_secret 16)"
read -rp "$(echo -e "${cyan}DB pass [auto]: ${plain}")" DB_PASS </dev/tty; DB_PASS="${DB_PASS:-$DB_PASS_DEFAULT}"
SETUP_KEY="$(gen_secret 16)"
SESSION_SECRET="$(gen_secret 32)"
PANEL_SECRET="$(gen_secret 32)"

# Input validation
[[ ! "$DB_NAME" =~ ^[a-zA-Z0-9_]+$ ]] && fatal "Invalid DB name (alphanumeric and underscore only)"
[[ ! "$DB_USER" =~ ^[a-zA-Z0-9_]+$ ]] && fatal "Invalid DB user (alphanumeric and underscore only)"
[[ ! "$PANEL_PORT" =~ ^[0-9]+$ ]] && fatal "Port must be numeric"

echo ""
info "Installing dependencies..."
export DEBIAN_FRONTEND=noninteractive
case "$OS" in
    ubuntu|debian)
        apt-get update -qq >/dev/null 2>&1
        apt-get install -y -qq git curl openssl ca-certificates mariadb-server \
            freeradius freeradius-mysql freeradius-utils nginx golang-go iproute2 \
            wireguard-tools openvpn easy-rsa strongswan xl2tpd certbot python3-certbot-nginx haproxy >/dev/null 2>&1
        # Node.js for frontend build
        if ! command -v npm >/dev/null 2>&1; then
            curl -fsSL https://deb.nodesource.com/setup_20.x 2>/dev/null | bash - >/dev/null 2>&1 || true
            apt-get install -y -qq nodejs >/dev/null 2>&1 || true
        fi
        ;;
    centos|almalinux|rocky|rhel|fedora)
        dnf install -y -q git curl openssl ca-certificates mariadb-server \
            freeradius freeradius-mysql freeradius-utils nginx golang iproute \
            wireguard-tools openvpn strongswan xl2tpd certbot python3-certbot-nginx haproxy >/dev/null 2>&1
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
CREATE USER IF NOT EXISTS '${DB_USER}'@'127.0.0.1' IDENTIFIED BY '${DB_PASS}';
ALTER USER '${DB_USER}'@'127.0.0.1' IDENTIFIED BY '${DB_PASS}';
GRANT ALL PRIVILEGES ON ${DB_NAME}.* TO '${DB_USER}'@'localhost';
GRANT ALL PRIVILEGES ON ${DB_NAME}.* TO '${DB_USER}'@'127.0.0.1';
FLUSH PRIVILEGES;
SQL
SCHEMA="/etc/freeradius/3.0/mods-config/sql/main/mysql/schema.sql"
if [[ -f "$SCHEMA" ]]; then
    mysql -u root "$DB_NAME" -N -B -e "SHOW TABLES LIKE 'radcheck';" 2>/dev/null | grep -q '^radcheck$' || mysql -u root "$DB_NAME" < "$SCHEMA"
fi

# MariaDB performance tuning
cat > /etc/mysql/mariadb.conf.d/99-koris-performance.cnf <<MYCNF
[mysqld]
innodb_buffer_pool_size = 1G
innodb_log_file_size = 256M
innodb_flush_log_at_trx_commit = 2
innodb_flush_method = O_DIRECT
max_connections = 300
thread_cache_size = 32
tmp_table_size = 64M
max_heap_table_size = 64M
skip-name-resolve
MYCNF

# MariaDB security: bind to localhost only
sed -i 's/^bind-address\s*=.*/bind-address = 127.0.0.1/' /etc/mysql/mariadb.conf.d/50-server.cnf 2>/dev/null || true
systemctl restart mariadb >/dev/null 2>&1
info "Database ready (optimized, bound to localhost)."

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

info "Building node agent..."
go build -ldflags="-s -w" -o /usr/local/bin/panel-node ./node/cmd/node
chmod +x /usr/local/bin/panel-node

# Build frontend
if command -v npm >/dev/null 2>&1; then
    info "Building shared components..."
    (cd panel/web/shared && npm install --no-audit --no-fund --silent 2>/dev/null) || true
    info "Building admin frontend..."
    (cd panel/web/admin && npm install --no-audit --no-fund --silent 2>/dev/null && npm run build >/dev/null 2>&1) || true
    info "Building portal frontend..."
    (cd panel/web/portal && npm install --no-audit --no-fund --silent 2>/dev/null && npm run build >/dev/null 2>&1) || true
else
    info "Using prebuilt frontend assets."
fi

# Install files
mkdir -p /opt/KorisPanel/panel/web/admin /opt/KorisPanel/panel/web/portal
cp -a "$INSTALL_DIR/panel/migrations" /opt/KorisPanel/panel/migrations 2>/dev/null || true
cp -a "$INSTALL_DIR/panel/web/admin/www" /opt/KorisPanel/panel/web/admin/www 2>/dev/null || true
cp -a "$INSTALL_DIR/panel/web/portal/www" /opt/KorisPanel/panel/web/portal/www 2>/dev/null || true

# VPN hook scripts
if [[ -d "$INSTALL_DIR/scripts/openvpn" ]]; then
    mkdir -p /etc/openvpn/server
    cp -f "$INSTALL_DIR/scripts/openvpn/"*.sh /etc/openvpn/server/ 2>/dev/null || true
    chmod +x /etc/openvpn/server/*.sh 2>/dev/null || true
fi

# Config
info "Writing configuration..."
PANEL_ADDR="127.0.0.1:${PANEL_PORT}"
mkdir -p "$CONFIG_DIR"
cat > "${CONFIG_DIR}/panel.env" <<ENV
PANEL_ADDR='${PANEL_ADDR}'
PANEL_DB_DSN='${DB_USER}:${DB_PASS}@tcp(127.0.0.1:3306)/${DB_NAME}?parseTime=true&multiStatements=true&charset=utf8mb4,utf8'
PANEL_MIGRATIONS='/opt/KorisPanel/panel/migrations'
PANEL_SETUP_KEY='${SETUP_KEY}'
PANEL_SESSION_SECRET='${SESSION_SECRET}'
PANEL_SECRET='${PANEL_SECRET}'
PANEL_PUBLIC_BASE='/dashboard'
PANEL_ADMIN_WEB_DIR='/opt/KorisPanel/panel/web/admin/www'
PANEL_PORTAL_WEB_DIR='/opt/KorisPanel/panel/web/portal/www'
PANEL_VERSION='${VERSION}'
ENV
chmod 600 "${CONFIG_DIR}/panel.env"

# Systemd — Panel
info "Installing panel service..."
cp "$INSTALL_DIR/panel/systemd/panel.service" /etc/systemd/system/panel.service 2>/dev/null || cat > /etc/systemd/system/panel.service <<SVC
[Unit]
Description=Koris Next Panel
After=network-online.target mariadb.service
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=${CONFIG_DIR}/panel.env
ExecStart=/usr/local/bin/panel
Restart=always
RestartSec=3
User=root

[Install]
WantedBy=multi-user.target
SVC

# Systemd — Node Agent
info "Installing node agent service..."
NODE_TOKEN="kn_$(gen_secret 24)"
mkdir -p /etc/panel-node
cat > /etc/panel-node/node.env <<NENV
PANEL_URL='http://${PANEL_ADDR}'
NODE_TOKEN='${NODE_TOKEN}'
NODE_NAME='$(hostname -s)'
NENV
chmod 600 /etc/panel-node/node.env

cat > /etc/systemd/system/node-agent.service <<SVC
[Unit]
Description=Koris Next Node Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/panel-node/node.env
ExecStart=/usr/local/bin/panel-node
Restart=always
RestartSec=3
User=root
WorkingDirectory=/opt/KorisPanel

[Install]
WantedBy=multi-user.target
SVC

systemctl daemon-reload
systemctl enable --now panel >/dev/null 2>&1
systemctl enable --now node-agent >/dev/null 2>&1
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

# Security hardening
info "Applying security hardening..."

# Swap (2GB) if none exists
if ! swapon --show | grep -q '/'; then
    TOTAL_RAM_MB=$(free -m | awk '/Mem:/{print $2}')
    SWAP_SIZE="2G"
    [[ $TOTAL_RAM_MB -ge 8000 ]] && SWAP_SIZE="4G"
    fallocate -l "$SWAP_SIZE" /swapfile 2>/dev/null || dd if=/dev/zero of=/swapfile bs=1M count=${SWAP_SIZE%G}000 status=none
    chmod 600 /swapfile
    mkswap /swapfile >/dev/null 2>&1
    swapon /swapfile
    grep -q '/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
    # Tune swappiness for a server workload
    sysctl -w vm.swappiness=10 >/dev/null 2>&1
    grep -q 'vm.swappiness' /etc/sysctl.conf || echo 'vm.swappiness=10' >> /etc/sysctl.conf
    info "Swap configured (${SWAP_SIZE})."
fi

# fail2ban
if ! command -v fail2ban-client >/dev/null 2>&1; then
    apt-get install -y -qq fail2ban >/dev/null 2>&1 || dnf install -y -q fail2ban >/dev/null 2>&1 || true
fi
if command -v fail2ban-client >/dev/null 2>&1; then
    cat > /etc/fail2ban/jail.local <<F2B
[DEFAULT]
bantime = 3600
findtime = 600
maxretry = 5
ignoreip = 127.0.0.1/8

[sshd]
enabled = true
port = ssh
maxretry = 3
bantime = 7200

[nginx-limit-req]
enabled = true
port = http,https
logpath = /var/log/nginx/error.log
maxretry = 10
findtime = 60
bantime = 600

[nginx-botsearch]
enabled = true
port = http,https
logpath = /var/log/nginx/access.log
maxretry = 10
findtime = 60
bantime = 3600
F2B
    systemctl enable --now fail2ban >/dev/null 2>&1
    info "fail2ban installed (SSH + Nginx jails)."
fi

info "Security hardening complete."

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
echo -e "  ${cyan}Node Token:${plain} ${NODE_TOKEN}"
echo -e "  ${cyan}Version:${plain}    ${VERSION}"
echo -e "${bold}${green}───────────────────────────────────────────────${plain}"
echo -e "  ${cyan}Manage:${plain}     koris"
echo -e "  ${cyan}SSL:${plain}        koris ssl"
echo -e "  ${cyan}Commands:${plain}   koris status|restart|update|logs|uninstall"
echo -e "${bold}${green}═══════════════════════════════════════════════${plain}"
echo ""
echo -e "${yellow}Open the Dashboard and use the Setup Key to create your admin account.${plain}"
echo -e "${yellow}SSL: run 'koris ssl' to setup HTTPS with Let's Encrypt.${plain}"
