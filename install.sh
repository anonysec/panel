#!/usr/bin/env bash
set -euo pipefail

# KorisPanel installer — Docker only
# Usage: bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/install.sh)
#   install.sh                          # Docker mode (recommended)
#   install.sh --lite                   # Lite edition
#   install.sh --port=8080 --domain=panel.example.com

# Source shared helper functions (provides validate_version_tag, validate_port, etc.)
# shellcheck source=helpers.sh
source "$(dirname "$0")/helpers.sh" 2>/dev/null || true

REPO="anonysec/panel"
KNODE_REPO="anonysec/knode"
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
EDITION="full"
PANEL_PORT="2026"
DOMAIN=""
DB_NAME="koris"
DB_USER="koris"
DB_PASS=""
WITH_KNODE="yes"
TLS_MODE="selfsigned"
CERT_PATH="/etc/koris/cert.pem"
KEY_PATH="/etc/koris/key.pem"
IMAGE_TAG=""
FORCE_REINSTALL=""

parse_args() {
  for arg in "$@"; do
    case "${arg}" in
      --native)       err "Native mode is no longer supported. Only Docker deployment is available. Remove the --native flag and re-run." ;;
      --lite)         EDITION="lite" ;;
      --full)         EDITION="full" ;;
      --port=*)       PANEL_PORT="${arg#*=}" ;;
      --domain=*)     DOMAIN="${arg#*=}" ;;
      --no-knode)     WITH_KNODE="no" ;;
      --uninstall)    uninstall; exit 0 ;;
      --version=*)    IMAGE_TAG="${arg#*=}" ;;
      --reinstall)    FORCE_REINSTALL="yes" ;;
      -h|--help)      banner; usage; exit 0 ;;
      *)              err "Unknown flag: ${arg}" ;;
    esac
  done
}

usage() {
  echo "Flags:"
  echo "  --lite          Lite edition (OpenVPN, L2TP, basic features)"
  echo "  --full          Full edition (all features, default)"
  echo "  --port=N        Panel listen port (default: 2026)"
  echo "  --domain=X      Domain name (for SSL)"
  echo "  --no-knode      Skip knode agent installation"
  echo "  --uninstall     Remove KorisPanel"
  echo "  --version=<tag> Install a specific version tag"
  echo "  --reinstall     Force a clean reinstall"
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
  echo -e "  ${CYAN}2)${NC} Self-signed certificate (works immediately, browser warning)"
  echo -e "  ${CYAN}3)${NC} Custom cert path (provide your own cert.pem + key.pem)"
  echo ""
  read -rp "$(echo -e "${CYAN}SSL mode [1/2/3]: ${NC}")" ssl_choice </dev/tty
  case "${ssl_choice}" in
    1)
      TLS_MODE="acme"
      if [[ -z "${DOMAIN}" || "${DOMAIN}" == "_" ]]; then
        err "Let's Encrypt requires a domain. Re-run and provide one."
      fi
      ;;
    3)
      TLS_MODE="manual"
      read -rp "$(echo -e "${CYAN}Path to cert.pem [/etc/koris/cert.pem]: ${NC}")" cert_path </dev/tty
      cert_path="${cert_path:-/etc/koris/cert.pem}"
      read -rp "$(echo -e "${CYAN}Path to key.pem [/etc/koris/key.pem]: ${NC}")" key_path </dev/tty
      key_path="${key_path:-/etc/koris/key.pem}"
      CERT_PATH="${cert_path}"
      KEY_PATH="${key_path}"
      ;;
    *) TLS_MODE="selfsigned" ;;
  esac
}

# --- Check for existing installation ---
is_existing_installation() {
  [[ -f "${CONFIG_DIR}/panel.env" ]] || return 1
  return 0
}

# --- Clone/fetch source repository ---
clone_source() {
  if [[ -d "${INSTALL_DIR}/.git" ]]; then
    log "Updating source in ${INSTALL_DIR}..."
    git -C "${INSTALL_DIR}" fetch --all --tags --quiet
  else
    log "Cloning panel source..."
    rm -rf "${INSTALL_DIR}"
    git clone "https://github.com/${REPO}.git" "${INSTALL_DIR}" --quiet
  fi

  # Checkout specific version tag if requested
  if [[ -n "${IMAGE_TAG}" ]]; then
    validate_version_tag "${IMAGE_TAG}" "https://github.com/${REPO}.git"
    log "Checking out version: ${IMAGE_TAG}"
    git -C "${INSTALL_DIR}" checkout "${IMAGE_TAG}" --quiet
  else
    # Default: latest main branch
    git -C "${INSTALL_DIR}" checkout main --quiet 2>/dev/null || true
    git -C "${INSTALL_DIR}" pull origin main --quiet 2>/dev/null || true
  fi
}

# --- Write panel.env configuration ---
write_panel_env() {
  mkdir -p "${CONFIG_DIR}"
  local session_secret setup_key pgadmin_pass
  session_secret="$(gen_secret 32)"
  setup_key="$(gen_secret 16)"
  pgadmin_pass="$(gen_secret 8)"

  cat > "${CONFIG_DIR}/panel.env" <<EOF
# KorisPanel Docker Configuration
# Generated by install.sh — do not edit POSTGRES_PASSWORD manually

# ─── Database (TimescaleDB/PostgreSQL) ────────────────────────────────
PANEL_DB_BACKEND=timescaledb
PANEL_PG_DSN=postgres://${DB_USER}:${DB_PASS}@db:5432/${DB_NAME}?sslmode=disable
POSTGRES_DB=${DB_NAME}
POSTGRES_USER=${DB_USER}
POSTGRES_PASSWORD=${DB_PASS}

# ─── Panel Server ────────────────────────────────────────────────────
PANEL_ADDR=0.0.0.0:${PANEL_PORT}
PANEL_PORT=${PANEL_PORT}
PANEL_SESSION_SECRET=${session_secret}
PANEL_SETUP_KEY=${setup_key}
PANEL_MIGRATIONS=/app/migrations
PANEL_TLS_MODE=${TLS_MODE}
PANEL_DOMAIN=${DOMAIN:-}

# ─── Build Tags ──────────────────────────────────────────────────────
BUILD_TAGS=${EDITION}

# ─── pgAdmin ─────────────────────────────────────────────────────────
PGADMIN_EMAIL=admin@koris.local
PGADMIN_PASSWORD=${pgadmin_pass}
PGADMIN_PORT=5050
EOF

  # Symlink for docker-compose env_file
  ln -sf "${CONFIG_DIR}/panel.env" "${INSTALL_DIR}/docker/panel.env" 2>/dev/null || true
  log "Configuration written to ${CONFIG_DIR}/panel.env"
}

# --- Write version file after successful install ---
write_version_file() {
  local version="${IMAGE_TAG:-}"
  if [[ -z "${version}" ]]; then
    # No explicit tag — read version from VERSION file in source
    version=$(cat "${INSTALL_DIR}/VERSION" 2>/dev/null || echo "latest")
  fi
  mkdir -p "${CONFIG_DIR}"
  echo "${version}" > "${CONFIG_DIR}/version"
  log "Version recorded: ${version}"
}

# --- Docker installation (sole installation path) ---
install_docker() {
  # Ensure Docker is available
  if ! command -v docker &>/dev/null; then
    log "Installing Docker..."
    curl -fsSL https://get.docker.com | sh
  fi
  docker info &>/dev/null || err "Docker installed but daemon is not running"

  # Ensure git is available
  if ! command -v git &>/dev/null; then
    apt-get update -qq && apt-get install -y -qq git >/dev/null 2>&1
  fi

  # Clone or update source
  clone_source

  # Write config (skip if reinstalling with existing config)
  if [[ "${FORCE_REINSTALL}" != "yes" ]] || ! is_existing_installation; then
    write_panel_env
  else
    # Reinstall with existing config — source DB_PASS for docker compose
    if [[ -f "${CONFIG_DIR}/panel.env" ]]; then
      DB_PASS=$(grep -oP 'POSTGRES_PASSWORD=\K.*' "${CONFIG_DIR}/panel.env" 2>/dev/null || true)
      if [[ -z "${DB_PASS}" ]]; then
        err "Reinstall failed: POSTGRES_PASSWORD not found in ${CONFIG_DIR}/panel.env"
      fi
      log "Reusing existing configuration from ${CONFIG_DIR}/panel.env"
    fi
  fi

  # Build and start the Docker Compose stack
  log "Building and starting Docker Compose stack..."
  cd "${INSTALL_DIR}"
  docker compose build || err "Docker build failed — check output above"
  docker compose up -d || err "Docker Compose failed to start services"

  # Wait for panel to become healthy
  log "Waiting for panel to become healthy..."
  local attempts=0
  while [[ ${attempts} -lt 30 ]]; do
    if docker inspect --format='{{.State.Health.Status}}' koris 2>/dev/null | grep -q "healthy"; then
      log "Panel is healthy"
      write_version_file
      return
    fi
    sleep 2
    attempts=$((attempts + 1))
  done
  warn "Panel did not reach healthy state within 60 seconds — check: docker logs koris"
  # Still write version file — containers are running even if health check timed out
  write_version_file
}

# --- Install knode alongside panel ---
install_knode_docker() {
  log "Installing knode agent on this host..."
  curl -fsSL "https://raw.githubusercontent.com/${KNODE_REPO}/master/install.sh" | bash
}

# --- Clean reinstall (remove containers/images, preserve db-data) ---
clean_reinstall() {
  log "Performing clean reinstall..."
  cd "${INSTALL_DIR}" 2>/dev/null || true
  docker compose down --remove-orphans 2>/dev/null || true
  docker compose rm -f 2>/dev/null || true
  # Remove panel and pgadmin volumes, keep db-data
  docker volume rm koris_panel-data koris_pgadmin-data 2>/dev/null || true
  # Remove project images
  docker images --filter "label=com.docker.compose.project=koris" -q | xargs -r docker rmi -f 2>/dev/null || true
}

# --- Uninstall ---
uninstall() {
  log "Uninstalling KorisPanel..."

  # Stop and remove Docker Compose stack
  if [[ -d "${INSTALL_DIR}" ]]; then
    cd "${INSTALL_DIR}"
    docker compose down -v --remove-orphans 2>/dev/null || true
  fi

  # Remove images
  docker images --filter "label=com.docker.compose.project=koris" -q | xargs -r docker rmi -f 2>/dev/null || true

  # Remove directories
  rm -rf "${INSTALL_DIR}"
  rm -rf "${CONFIG_DIR}"
  rm -f /usr/local/bin/koris

  log "KorisPanel uninstalled"
}

# --- Show installation result ---
show_result() {
  local SERVER_IP
  SERVER_IP=$(curl -fsS4 --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')

  echo ""
  echo -e "${GREEN}═══════════════════════════════════════${NC}"
  echo -e "${GREEN}  KorisPanel installed successfully!${NC}"
  echo -e "${GREEN}═══════════════════════════════════════${NC}"
  echo ""
  echo -e "  Edition:   ${CYAN}${EDITION}${NC}"
  echo -e "  URL:       ${CYAN}https://${DOMAIN:-${SERVER_IP}}:${PANEL_PORT}${NC}"
  echo -e "  Port:      ${CYAN}${PANEL_PORT}${NC}"
  echo -e "  Config:    ${CONFIG_DIR}/panel.env"
  echo -e "  Source:    ${INSTALL_DIR}"
  echo ""
  echo -e "  ${CYAN}Logs:${NC}      docker compose -f ${INSTALL_DIR}/docker-compose.yml logs -f"
  echo -e "  ${CYAN}Restart:${NC}   docker compose -f ${INSTALL_DIR}/docker-compose.yml restart"
  echo -e "  ${CYAN}Stop:${NC}      docker compose -f ${INSTALL_DIR}/docker-compose.yml down"
  echo ""
  if [[ -f "${CONFIG_DIR}/panel.env" ]]; then
    local setup_key
    setup_key=$(grep -oP 'PANEL_SETUP_KEY=\K.*' "${CONFIG_DIR}/panel.env" 2>/dev/null || echo "")
    if [[ -n "${setup_key}" ]]; then
      echo -e "  ${YELLOW}Setup Key:${NC} ${setup_key}"
      echo -e "  (Use this key on first login to create your admin account)"
      echo ""
    fi
  fi
  echo -e "${GREEN}═══════════════════════════════════════${NC}"
  echo ""
}

# --- Detect existing installations ---
detect_existing() {
  local has_panel="" has_knode="" panel_ver=""

  # Check for existing panel
  if [[ -f "${CONFIG_DIR}/panel.env" ]] || docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx koris; then
    has_panel="yes"
    panel_ver=$(cat "${INSTALL_DIR}/VERSION" 2>/dev/null || echo "unknown")
  fi

  # Check for existing knode
  if [[ -f "/etc/knode/config.toml" ]] || docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx knode; then
    has_knode="yes"
  fi

  if [[ -z "${has_panel}" && -z "${has_knode}" ]]; then
    return 1  # No existing installation found
  fi

  # Show what we found
  echo -e "${BOLD}Existing installation detected:${NC}"
  echo ""
  if [[ "${has_panel}" == "yes" ]]; then
    local panel_state
    panel_state=$(docker inspect -f '{{.State.Status}}' koris 2>/dev/null || echo "stopped")
    echo -e "  ${CYAN}●${NC} KorisPanel v${panel_ver} (${panel_state})"
  fi
  if [[ "${has_knode}" == "yes" ]]; then
    local knode_state
    knode_state=$(docker inspect -f '{{.State.Status}}' knode 2>/dev/null || echo "stopped")
    echo -e "  ${CYAN}●${NC} knode (${knode_state})"
  fi
  echo ""

  # Ask what to do
  echo -e "  ${CYAN}1)${NC} Update (pull latest, rebuild — no downtime beyond restart)"
  echo -e "  ${CYAN}2)${NC} Clean reinstall (wipe containers/images, keep DB, rebuild from scratch)"
  echo -e "  ${CYAN}3)${NC} Full wipe & fresh install (removes ALL data including database)"
  echo -e "  ${CYAN}4)${NC} Cancel"
  echo ""
  read -rp "$(echo -e "${CYAN}Choose [1/2/3/4]: ${NC}")" reinstall_choice </dev/tty

  case "${reinstall_choice}" in
    1)
      log "Updating to latest version..."
      cd "${INSTALL_DIR}"
      git fetch origin main --depth=1 >/dev/null 2>&1
      git reset --hard origin/main >/dev/null 2>&1
      docker compose up -d --build
      [[ -f "${INSTALL_DIR}/koris.sh" ]] && cp "${INSTALL_DIR}/koris.sh" /usr/local/bin/koris && chmod +x /usr/local/bin/koris
      log "Updated to v$(cat "${INSTALL_DIR}/VERSION" 2>/dev/null || echo '?')"
      exit 0
      ;;
    2)
      log "Clean reinstall — database data will be preserved"
      FORCE_REINSTALL="yes"
      clean_reinstall
      ;;
    3)
      echo ""
      echo -e "${RED}WARNING: This will delete ALL data including the database.${NC}"
      read -rp "Type 'yes' to confirm: " wipe_confirm </dev/tty
      if [[ "${wipe_confirm}" != "yes" ]]; then
        log "Cancelled."
        exit 0
      fi
      log "Full wipe — removing everything..."
      cd "${INSTALL_DIR}" 2>/dev/null && docker compose down --volumes --remove-orphans 2>/dev/null || true
      docker rm -f koris koris-db koris-pgadmin knode 2>/dev/null || true
      docker volume rm koris_db-data koris_panel-data koris_pgadmin-data 2>/dev/null || true
      docker images --format '{{.ID}} {{.Repository}}' 2>/dev/null | awk '$2 ~ /^koris/ {print $1}' | xargs -r docker rmi -f 2>/dev/null || true
      docker images --filter "label=com.docker.compose.project=koris" -q 2>/dev/null | xargs -r docker rmi -f 2>/dev/null || true
      rm -rf "${INSTALL_DIR}" "${CONFIG_DIR}" /usr/local/bin/koris
      rm -rf /etc/knode
      log "Wipe complete. Starting fresh install..."
      ;;
    4|*)
      log "Cancelled."
      exit 0
      ;;
  esac

  return 0
}

# --- Main ---
main() {
  banner
  [[ "$(id -u)" -eq 0 ]] || err "Must run as root"
  detect_os
  parse_args "$@"

  # Handle explicit --reinstall flag (non-interactive, e.g. from koris downgrade)
  if [[ "${FORCE_REINSTALL}" == "yes" ]]; then
    if is_existing_installation; then
      clean_reinstall
    fi
  else
    # Interactive: detect existing installation and ask user what to do
    if detect_existing 2>/dev/null; then
      # User chose option 1 (reinstall) or 2 (wipe) — continue with install
      :
    fi
  fi

  # If knode-only edition was selected, delegate to knode installer
  if [[ "${EDITION}" == "knode" ]]; then
    install_knode_docker
    exit 0
  fi

  # Interactive prompts (skipped if reinstalling with existing config)
  if [[ "${FORCE_REINSTALL}" != "yes" ]]; then
    prompt_config
  fi

  # Docker installation — the only supported path
  install_docker

  # Install CLI management tool
  if [[ -f "${INSTALL_DIR}/koris.sh" ]]; then
    cp "${INSTALL_DIR}/koris.sh" /usr/local/bin/koris
    chmod +x /usr/local/bin/koris
    log "CLI installed: /usr/local/bin/koris"
  fi

  # Optional knode co-installation
  if [[ "${WITH_KNODE}" == "yes" && "${EDITION}" != "knode" ]]; then
    echo ""
    read -rp "$(echo -e "${CYAN}Install knode agent on this server too? [y/N]: ${NC}")" install_knode </dev/tty
    if [[ "${install_knode}" =~ ^[yY] ]]; then
      install_knode_docker
    fi
  fi

  show_result
}

main "$@"
