#!/usr/bin/env bash
set -euo pipefail

# KorisPanel installer — Docker (default) or native (--native flag)
# Usage: bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/install.sh)
#   install.sh                          # Docker mode (recommended)
#   install.sh --native                 # Native mode (systemd)
#   install.sh --native --lite          # Lite edition (native)
#   install.sh --port=8080 --domain=panel.example.com

REPO="anonysec/panel"
KNODE_REPO="anonysec/knode"
IMAGE="ghcr.io/${REPO}:latest"
INSTALL_DIR="/opt/KorisPanel"
CONFIG_DIR="/etc/koris"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

log()  { echo -e "${GREEN}[+]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err()  { echo -e "${RED}[✗]${NC} $1"; exit 1; }

banner() {
  echo -e "${BOLD}${CYAN}"
  cat << 'EOF'
  ██╗  ██╗ ██████╗ ██████╗ ██╗███████╗
  ██║ ██╔╝██╔═══██╗██╔══██╗██║██╔════╝
  █████╔╝ ██║   ██║██████╔╝██║███████╗
  ██╔═██╗ ██║   ██║██╔══██╗██║╚════██║
  ██║  ██╗╚██████╔╝██║  ██║██║███████║
  ╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═╝╚═╝╚══════╝
EOF
  echo -e "${NC} ${GREEN}KorisPanel — VPN Management Panel Installer${NC}\n"
}

detect_os() {
  [[ -f /etc/os-release ]] || err "Unsupported OS: /etc/os-release not found"
  local os_id os_version
  os_id=$(. /etc/os-release && echo "$ID")
  os_version=$(. /etc/os-release && echo "$VERSION_ID")
  case "${os_id}" in
    ubuntu|debian) log "Detected ${os_id} ${os_version}" ;;
    *) err "Unsupported: ${os_id} ${os_version}. Need Ubuntu 22.04+ or Debian 12+" ;;
  esac
}

gen_secret() { openssl rand -hex "${1:-32}" 2>/dev/null || head -c "${1:-32}" /dev/urandom | od -An -tx1 | tr -d ' \n'; }

# --- Parse flags ---
INSTALL_MODE="docker"
EDITION="full"
PANEL_PORT="2026"
DOMAIN=""
DB_NAME="koris"
DB_USER="koris"
DB_PASS=""
WITH_KNODE="yes"
TLS_MODE="selfsigned"

parse_args() {
  for arg in "$@"; do
    case "${arg}" in
      --docker)       INSTALL_MODE="docker" ;;
      --native)       INSTALL_MODE="native" ;;
      --lite)         EDITION="lite" ;;
      --full)         EDITION="full" ;;
      --port=*)       PANEL_PORT="${arg#*=}" ;;
      --domain=*)     DOMAIN="${arg#*=}" ;;
      --db-name=*)    DB_NAME="${arg#*=}" ;;
      --db-user=*)    DB_USER="${arg#*=}" ;;
      --db-pass=*)    DB_PASS="${arg#*=}" ;;
      --no-knode)     WITH_KNODE="no" ;;
      --uninstall)    uninstall; exit 0 ;;
      -h|--help)      banner; usage; exit 0 ;;
      *)              err "Unknown flag: ${arg}" ;;
    esac
  done
}

usage() {
  echo "Flags:"
  echo "  --docker        Docker mode (default, recommended)"
  echo "  --native        Native mode (systemd + MariaDB + Nginx)"
  echo "  --lite          Lite edition (OpenVPN, L2TP, basic features)"
  echo "  --full          Full edition (all features, default)"
  echo "  --port=N        Panel listen port (default: 8080)"
  echo "  --domain=X      Domain name (for SSL)"
  echo "  --db-name=X     Database name (default: radius)"
  echo "  --db-user=X     Database user (default: radius)"
  echo "  --db-pass=X     Database password (auto-generated if empty)"
  echo "  --no-knode      Skip knode agent installation"
  echo "  --uninstall     Remove KorisPanel"
}

prompt_config() {
  # Edition selection
  echo -e "${BOLD}What do you want to install?${NC}"
  echo ""
  echo -e "  ${CYAN}1)${NC} koris      — Full panel (billing, tickets, xray, reseller, all features)"
  echo -e "  ${CYAN}2)${NC} korislite  — Lite panel (OpenVPN, L2TP, users, nodes, settings)"
  echo -e "  ${CYAN}3)${NC} knode      — Node agent only (install on VPN servers)"
  echo ""
  read -rp "$(echo -e "${CYAN}Choose [1/2/3]: ${NC}")" edition_choice </dev/tty
  case "$edition_choice" in
    1) EDITION="full" ;;
    2) EDITION="lite" ;;
    3) EDITION="knode" ;;
    *) err "Invalid choice. Run the script again." ;;
  esac
  echo ""

  # If knode-only, skip panel prompts
  if [[ "${EDITION}" == "knode" ]]; then
    log "Selected: knode (node agent only)"
    return
  fi

  log "Selected: ${EDITION}"
  echo ""

  [[ -z "${DB_PASS}" ]] && DB_PASS="$(gen_secret 16)"

  if [[ -z "${DOMAIN}" ]]; then
    read -rp "$(echo -e "${CYAN}Domain (blank for IP-only): ${NC}")" DOMAIN </dev/tty
  fi
  if [[ "${PANEL_PORT}" == "2026" ]]; then
    read -rp "$(echo -e "${CYAN}Panel port [2026]: ${NC}")" input_port </dev/tty
    PANEL_PORT="${input_port:-2026}"
  fi
  if [[ "${DB_NAME}" == "koris" ]]; then
    read -rp "$(echo -e "${CYAN}DB name [koris]: ${NC}")" input_db </dev/tty
    DB_NAME="${input_db:-koris}"
  fi
  if [[ "${DB_USER}" == "koris" ]]; then
    read -rp "$(echo -e "${CYAN}DB user [koris]: ${NC}")" input_user </dev/tty
    DB_USER="${input_user:-koris}"
  fi

  # SSL mode selection
  echo ""
  echo -e "  ${CYAN}1)${NC} Let's Encrypt (requires domain pointed to this server)"
  echo -e "  ${CYAN}2)${NC} Manual cert (place cert.pem + key.pem in /etc/koris/)"
  echo -e "  ${CYAN}3)${NC} No SSL — plain HTTP (default, use reverse proxy for HTTPS)"
  echo ""
  read -rp "$(echo -e "${CYAN}SSL mode [1/2/3, default=3]: ${NC}")" ssl_choice </dev/tty
  case "${ssl_choice}" in
    1)
      TLS_MODE="acme"
      if [[ -z "${DOMAIN}" || "${DOMAIN}" == "_" ]]; then
        err "Let's Encrypt requires a domain. Re-run and provide one."
      fi
      ;;
    2) TLS_MODE="manual" ;;
    *) TLS_MODE="disabled" ;;
  esac

  echo ""
  log "Edition:  ${EDITION}"
  log "Mode:     ${INSTALL_MODE}"
  log "Port:     ${PANEL_PORT}"
  log "Domain:   ${DOMAIN:-<none>}"
  log "SSL:      ${TLS_MODE}"
  log "Database: ${DB_NAME} (user: ${DB_USER})"
  echo ""
}

# ═══════════════════════════════════════════════════════════════════════
# DOCKER MODE (default)
# ═══════════════════════════════════════════════════════════════════════
install_docker() {
  # Install Docker if not present
  if ! command -v docker &>/dev/null; then
    log "Installing Docker..."
    curl -fsSL https://get.docker.com | sh
    systemctl enable --now docker
  fi

  # Ensure docker compose is available
  if ! docker compose version &>/dev/null; then
    err "docker compose not available. Install Docker Compose V2."
  fi

  log "Setting up KorisPanel via Docker Compose..."

  # Clone/update source
  clone_source

  # Generate secrets
  local session_secret="$(gen_secret 32)"
  local setup_key="$(gen_secret 16)"

  # Write env file
  mkdir -p "${CONFIG_DIR}"
  cat > "${CONFIG_DIR}/panel.env" <<ENV
# KorisPanel Docker Configuration (TimescaleDB + built-in TLS)

# Database (TimescaleDB/PostgreSQL)
PANEL_PG_DSN=postgres://${DB_USER}:${DB_PASS}@db:5432/${DB_NAME}?sslmode=disable
POSTGRES_DB=${DB_NAME}
POSTGRES_USER=${DB_USER}
POSTGRES_PASSWORD=${DB_PASS}

# Panel
PANEL_ADDR=0.0.0.0:${PANEL_PORT}
PANEL_SESSION_SECRET=${session_secret}
PANEL_SETUP_KEY=${setup_key}
PANEL_MIGRATIONS=/app/migrations
PANEL_TLS_MODE=${TLS_MODE}
PANEL_DOMAIN=${DOMAIN:-}
ENV
  chmod 600 "${CONFIG_DIR}/panel.env"

  # Copy env to working directory
  cd "${INSTALL_DIR}"
  cp "${CONFIG_DIR}/panel.env" docker/panel.env
  # Docker Compose reads .env at project root for variable interpolation
  ln -sf docker/panel.env .env

  # Build and start
  log "Building Docker images (this may take a few minutes)..."
  docker compose up -d --build

  # Wait for health check
  log "Waiting for panel to start..."
  local attempts=0
  local health_url="http://localhost:${PANEL_PORT}/api/health"
  [[ "${TLS_MODE}" != "disabled" ]] && health_url="https://localhost:${PANEL_PORT}/api/health"
  while [[ $attempts -lt 30 ]]; do
    if docker inspect -f '{{.State.Health.Status}}' koris 2>/dev/null | grep -q healthy; then
      break
    fi
    sleep 3
    attempts=$((attempts + 1))
  done

  if [[ $attempts -ge 30 ]]; then
    warn "Panel health check timed out. Check: koris logs"
  else
    log "Panel is ${GREEN}running${NC}"
  fi

  # Install knode alongside if requested
  if [[ "${WITH_KNODE}" == "yes" ]]; then
    read -rp "$(echo -e "${CYAN}Install knode agent on this server? [y/N]: ${NC}")" install_knode </dev/tty
    if [[ "${install_knode}" =~ ^[yY] ]]; then
      install_knode_docker
    fi
  fi

  # Install koris CLI
  cp "${INSTALL_DIR}/koris.sh" /usr/local/bin/koris
  chmod +x /usr/local/bin/koris
  log "CLI installed: run 'koris' from anywhere"

  show_result "${setup_key}"
}

install_knode_docker() {
  log "Setting up knode agent (Docker)..."

  local knode_dir="/opt/knode"
  if [[ -d "${knode_dir}/.git" ]]; then
    cd "${knode_dir}" && git fetch origin master --depth=1 >/dev/null 2>&1 && git reset --hard origin/master >/dev/null 2>&1
  else
    rm -rf "${knode_dir}"
    git clone --depth=1 "https://github.com/${KNODE_REPO}.git" "${knode_dir}" >/dev/null 2>&1
  fi

  # Build knode image
  docker build -t knode:latest "${knode_dir}" >/dev/null 2>&1 || {
    warn "knode Docker build failed — skipping"
    return
  }

  # Generate knode config
  local api_key="$(gen_secret 16)"
  mkdir -p /etc/knode
  cat > /etc/knode/config.toml <<TOML
[api]
listen_addr = "0.0.0.0:62050"
api_keys = ["${api_key}"]
enable_rest = false

[logging]
level = "info"
format = "json"

[performance]
gogc = 100
mem_limit = "256MB"
TOML

  # Run knode container
  docker rm -f knode 2>/dev/null || true
  docker run -d --name knode --network host --restart unless-stopped \
    --cap-add NET_ADMIN --cap-add NET_RAW \
    -v /etc/knode:/etc/knode \
    knode:latest

  sleep 2
  if docker ps --format '{{.Names}}' | grep -qx knode; then
    log "knode is ${GREEN}running${NC} on port 62050"
  else
    warn "knode container may have failed. Check: docker logs knode"
  fi
}

# ═══════════════════════════════════════════════════════════════════════
# NATIVE MODE (--native)
# ═══════════════════════════════════════════════════════════════════════
install_native() {
  log "Installing dependencies..."
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq >/dev/null 2>&1
  apt-get install -y -qq git curl openssl ca-certificates mariadb-server \
    freeradius freeradius-mysql freeradius-utils nginx golang-go iproute2 \
    wireguard-tools openvpn easy-rsa strongswan xl2tpd certbot python3-certbot-nginx >/dev/null 2>&1
  log "Dependencies installed."

  # Database setup
  setup_database

  # FreeRADIUS
  setup_freeradius

  # Clone source & build
  clone_source
  build_panel

  # Install knode
  if [[ "${WITH_KNODE}" == "yes" ]]; then
    read -rp "$(echo -e "${CYAN}Install knode agent on this server? [y/N]: ${NC}")" install_knode </dev/tty
    if [[ "${install_knode}" =~ ^[yY] ]]; then
      build_knode
    else
      WITH_KNODE="no"
    fi
  fi

  # Generate secrets
  local session_secret="$(gen_secret 32)"
  local setup_key="$(gen_secret 16)"
  local panel_secret="$(gen_secret 32)"
  local knode_api_key="$(gen_secret 32)"

  # Write panel config
  mkdir -p "${CONFIG_DIR}"
  local binary_name="koris"
  [[ "${EDITION}" == "lite" ]] && binary_name="korislite"

  cat > "${CONFIG_DIR}/panel.env" <<ENV
PANEL_ADDR='127.0.0.1:${PANEL_PORT}'
PANEL_DB_DSN='${DB_USER}:${DB_PASS}@tcp(127.0.0.1:3306)/${DB_NAME}?parseTime=true&multiStatements=true&charset=utf8mb4,utf8'
PANEL_MIGRATIONS='/opt/KorisPanel/panel/migrations'
PANEL_SETUP_KEY='${setup_key}'
PANEL_SESSION_SECRET='${session_secret}'
PANEL_SECRET='${panel_secret}'
PANEL_PUBLIC_BASE='/dashboard'
PANEL_ADMIN_WEB_DIR='/opt/KorisPanel/panel/web/admin/www'
PANEL_PORTAL_WEB_DIR='/opt/KorisPanel/panel/web/portal/www'
PANEL_VERSION='$(cat "${INSTALL_DIR}/VERSION" 2>/dev/null || echo dev)'
ENV
  chmod 600 "${CONFIG_DIR}/panel.env"

  # Systemd — Panel
  local service_name="${binary_name}"
  cat > "/etc/systemd/system/${service_name}.service" <<SVC
[Unit]
Description=KorisPanel (${EDITION})
After=network-online.target mariadb.service
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=${CONFIG_DIR}/panel.env
ExecStart=/usr/local/bin/${binary_name}
Restart=always
RestartSec=3
User=root
WorkingDirectory=/opt/KorisPanel
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
SVC

  # Knode config & service
  if [[ "${WITH_KNODE}" == "yes" ]]; then
    mkdir -p /etc/knode
    cat > /etc/knode/config.toml <<TOML
[api]
listen_addr = "0.0.0.0:62050"
api_keys = ["${knode_api_key}"]
enable_rest = false

[logging]
level = "info"
format = "json"

[performance]
gogc = 100
mem_limit = "256MB"
TOML
    chmod 600 /etc/knode/config.toml

    cat > /etc/systemd/system/knode.service <<SVC
[Unit]
Description=Koris Node Agent
After=network-online.target ${service_name}.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/knode
Restart=always
RestartSec=3
User=root
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
SVC
  fi

  # Nginx
  setup_nginx

  # Start services
  systemctl daemon-reload
  systemctl enable --now "${service_name}" >/dev/null 2>&1
  [[ "${WITH_KNODE}" == "yes" ]] && systemctl enable --now knode >/dev/null 2>&1
  sleep 2

  # Health check
  if curl -fsS "http://127.0.0.1:${PANEL_PORT}/api/health" >/dev/null 2>&1; then
    log "Health check ${GREEN}PASSED${NC}"
  else
    warn "Health check failed — check: journalctl -u ${service_name} -n 20"
  fi

  # Swap
  setup_swap

  show_result "${setup_key}"
}

# ═══════════════════════════════════════════════════════════════════════
# HELPERS
# ═══════════════════════════════════════════════════════════════════════

clone_source() {
  log "Downloading KorisPanel..."
  if [[ -d "${INSTALL_DIR}/.git" ]]; then
    cd "${INSTALL_DIR}" && git fetch origin main --depth=1 >/dev/null 2>&1 && git reset --hard origin/main >/dev/null 2>&1
  else
    rm -rf "${INSTALL_DIR}"
    git clone --depth=1 -b main "https://github.com/${REPO}.git" "${INSTALL_DIR}" >/dev/null 2>&1
  fi
  cd "${INSTALL_DIR}"
  log "Source ready."
}

build_panel() {
  local binary_name="koris"
  local build_tags=""
  [[ "${EDITION}" == "lite" ]] && binary_name="korislite" && build_tags="-tags lite"

  log "Building ${binary_name}..."
  cd "${INSTALL_DIR}"
  go mod tidy >/dev/null 2>&1
  go build -ldflags="-s -w" ${build_tags} -o "/usr/local/bin/${binary_name}" ./panel/cmd/panel/
  chmod +x "/usr/local/bin/${binary_name}"
  log "${binary_name} built."
}

build_knode() {
  log "Building knode..."
  local knode_dir="/opt/knode"
  if [[ -d "${knode_dir}/.git" ]]; then
    cd "${knode_dir}" && git fetch origin master --depth=1 >/dev/null 2>&1 && git reset --hard origin/master >/dev/null 2>&1
  else
    rm -rf "${knode_dir}"
    git clone --depth=1 "https://github.com/${KNODE_REPO}.git" "${knode_dir}" >/dev/null 2>&1
  fi
  cd "${knode_dir}"
  go build -ldflags="-s -w" -o /usr/local/bin/knode ./cmd/node/
  chmod +x /usr/local/bin/knode
  cd "${INSTALL_DIR}"
  log "knode built."
}

setup_database() {
  log "Setting up MariaDB..."
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

  # FreeRADIUS schema
  local schema="/etc/freeradius/3.0/mods-config/sql/main/mysql/schema.sql"
  if [[ -f "$schema" ]]; then
    mysql -u root "$DB_NAME" -N -B -e "SHOW TABLES LIKE 'radcheck';" 2>/dev/null | grep -q '^radcheck$' || mysql -u root "$DB_NAME" < "$schema"
  fi

  # Performance tuning
  local total_ram=$(free -m | awk '/Mem:/{print $2}')
  local pool="256M"
  [[ $total_ram -ge 2000 ]] && pool="512M"
  [[ $total_ram -ge 4000 ]] && pool="1G"
  cat > /etc/mysql/mariadb.conf.d/99-koris.cnf <<MYCNF
[mysqld]
innodb_buffer_pool_size = ${pool}
innodb_log_file_size = 128M
innodb_flush_log_at_trx_commit = 2
innodb_flush_method = O_DIRECT
max_connections = 200
thread_cache_size = 16
skip-name-resolve
MYCNF
  systemctl restart mariadb >/dev/null 2>&1
  log "Database ready."
}

setup_freeradius() {
  log "Configuring FreeRADIUS..."
  local sql_mod="/etc/freeradius/3.0/mods-available/sql"
  if [[ -f "$sql_mod" ]]; then
    sed -i -e 's/^\s*dialect = .*/\tdialect = "mysql"/' \
           -e "s/^\s*login = .*/\tlogin = \"${DB_USER}\"/" \
           -e "s/^\s*password = .*/\tpassword = \"${DB_PASS}\"/" \
           -e "s/^\s*radius_db = .*/\tradius_db = \"${DB_NAME}\"/" "$sql_mod"
    ln -sf ../mods-available/sql /etc/freeradius/3.0/mods-enabled/sql 2>/dev/null || true
    systemctl restart freeradius >/dev/null 2>&1 || true
  fi
}

setup_nginx() {
  log "Configuring Nginx..."
  if [[ ! -f "${CONFIG_DIR}/cert.pem" ]]; then
    openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
      -keyout "${CONFIG_DIR}/key.pem" -out "${CONFIG_DIR}/cert.pem" \
      -subj "/CN=${DOMAIN:-localhost}" >/dev/null 2>&1
  fi

  local server_name="${DOMAIN:-_}"
  cat > /etc/nginx/sites-available/koris.conf <<NGINX
server {
    listen 80 default_server;
    server_name ${server_name};
    return 301 https://\$host\$request_uri;
}
server {
    listen 443 ssl default_server;
    server_name ${server_name};
    client_max_body_size 20m;
    ssl_certificate ${CONFIG_DIR}/cert.pem;
    ssl_certificate_key ${CONFIG_DIR}/key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    ssl_session_cache shared:SSL:10m;

    location = / { return 302 /dashboard/; }
    location = /dashboard { return 302 /dashboard/; }
    location /dashboard/ {
        proxy_pass http://127.0.0.1:${PANEL_PORT};
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }
    location /api/ {
        proxy_pass http://127.0.0.1:${PANEL_PORT};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }
    location = /portal { return 302 /portal/; }
    location /portal/ {
        proxy_pass http://127.0.0.1:${PANEL_PORT};
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }
}
NGINX
  rm -f /etc/nginx/sites-enabled/default 2>/dev/null
  ln -sf /etc/nginx/sites-available/koris.conf /etc/nginx/sites-enabled/koris.conf
  nginx -t >/dev/null 2>&1 && systemctl reload nginx
}

setup_swap() {
  if swapon --show | grep -q '/'; then return; fi
  local total_ram=$(free -m | awk '/Mem:/{print $2}')
  local swap_size="2G"
  [[ $total_ram -ge 4000 ]] && swap_size="4G"
  fallocate -l "$swap_size" /swapfile 2>/dev/null || dd if=/dev/zero of=/swapfile bs=1M count=2048 status=none
  chmod 600 /swapfile && mkswap /swapfile >/dev/null 2>&1 && swapon /swapfile
  grep -q '/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
  sysctl -w vm.swappiness=10 >/dev/null 2>&1
  log "Swap configured (${swap_size})."
}

show_result() {
  local setup_key="$1"
  local server_ip
  server_ip=$(curl -fsS4 --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')

  # Build access URL with port
  local access_host="${DOMAIN:-${server_ip}}"
  local access_url="http://${access_host}:${PANEL_PORT}"
  if [[ "${TLS_MODE}" == "acme" || "${TLS_MODE}" == "manual" ]]; then
    access_url="https://${access_host}:${PANEL_PORT}"
  fi

  echo ""
  echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
  echo -e "${GREEN}  KorisPanel Installed Successfully!${NC}"
  echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
  echo ""
  echo -e "  Edition:    ${CYAN}${EDITION}${NC}"
  echo -e "  Mode:       ${CYAN}${INSTALL_MODE}${NC}"
  echo -e "  Dashboard:  ${CYAN}${access_url}/dashboard/${NC}"
  echo -e "  Portal:     ${CYAN}${access_url}/portal/${NC}"
  echo ""
  echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
  echo -e "${YELLOW}  Setup Key (use to create admin account):${NC}"
  echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
  echo ""
  echo -e "  ${CYAN}${setup_key}${NC}"
  echo ""
  echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
  echo ""
  echo -e "  ${CYAN}TLS/SSL:${NC}   Place cert files at:"
  echo -e "             /etc/koris/cert.pem"
  echo -e "             /etc/koris/key.pem"
  echo -e "             Then: koris restart"
  echo ""
  if [[ "${INSTALL_MODE}" == "docker" ]]; then
    echo -e "  ${CYAN}Logs:${NC}      koris logs"
    echo -e "  ${CYAN}Restart:${NC}   koris restart"
    echo -e "  ${CYAN}Stop:${NC}      koris stop"
    echo -e "  ${CYAN}Update:${NC}    koris update"
    echo -e "  ${CYAN}Status:${NC}    koris status"
  else
    local svc="koris"
    [[ "${EDITION}" == "lite" ]] && svc="korislite"
    echo -e "  ${CYAN}Logs:${NC}      koris logs"
    echo -e "  ${CYAN}Restart:${NC}   koris restart"
    echo -e "  ${CYAN}Stop:${NC}      koris stop"
    echo -e "  ${CYAN}Update:${NC}    koris update"
  fi
  echo ""
  echo -e "${GREEN}═══════════════════════════════════════════════${NC}"
  echo -e "  ${CYAN}Uninstall:${NC}   koris uninstall"
  echo ""
}

# --- Uninstall ---
uninstall() {
  log "Uninstalling KorisPanel..."

  # Docker cleanup
  if command -v docker &>/dev/null; then
    if [[ -f "${INSTALL_DIR}/docker-compose.yml" ]]; then
      cd "${INSTALL_DIR}" && docker compose down -v 2>/dev/null || true
      log "Removed Docker containers and volumes"
    fi
    docker rm -f knode 2>/dev/null && log "Removed knode container" || true
  fi

  # Systemd services
  for svc in koris korislite knode; do
    if [[ -f "/etc/systemd/system/${svc}.service" ]]; then
      systemctl stop "${svc}" 2>/dev/null || true
      systemctl disable "${svc}" 2>/dev/null || true
      rm -f "/etc/systemd/system/${svc}.service"
      log "Removed service: ${svc}"
    fi
  done
  systemctl daemon-reload 2>/dev/null || true

  # Binaries
  rm -f /usr/local/bin/koris /usr/local/bin/korislite /usr/local/bin/knode

  # Config
  rm -rf "${CONFIG_DIR}"
  rm -rf /etc/knode

  # Nginx config
  rm -f /etc/nginx/sites-enabled/koris.conf /etc/nginx/sites-available/koris.conf
  systemctl reload nginx 2>/dev/null || true

  # Source (prompt)
  if [[ -d "${INSTALL_DIR}" ]]; then
    read -rp "$(echo -e "${YELLOW}Remove source code at ${INSTALL_DIR}? [y/N]: ${NC}")" confirm </dev/tty
    [[ "${confirm}" =~ ^[yY] ]] && rm -rf "${INSTALL_DIR}" && log "Removed ${INSTALL_DIR}"
  fi

  log "KorisPanel uninstalled."
}

# --- Main ---
main() {
  banner
  [[ "$(id -u)" -eq 0 ]] || err "Must run as root"
  detect_os
  parse_args "$@"
  prompt_config

  case "${INSTALL_MODE}" in
    docker) install_docker ;;
    native) install_native ;;
  esac
}

main "$@"
