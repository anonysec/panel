#!/usr/bin/env bash
# shellcheck disable=SC2034
#
# KorisPanel — Shared Helper Functions Library
# Source this file from panel/install.sh, knode/install.sh, or panel/koris.sh:
#   source "$(dirname "$0")/helpers.sh"
#
# Provides: log, warn, err, gen_secret, detect_os, require_root, require_docker,
#           require_container, confirm, human_bytes, validate_port, validate_version_tag

# ─── Colors ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# ─── Logging ─────────────────────────────────────────────────────────────────

log() {
  echo -e "${GREEN}[+]${NC} $*"
}

warn() {
  echo -e "${YELLOW}[!]${NC} $*"
}

err() {
  echo -e "${RED}[✗]${NC} $*" >&2
  exit 1
}

# ─── Cryptographic Utilities ─────────────────────────────────────────────────

# Generate a cryptographically random hex string.
# Usage: gen_secret [bytes]   (default: 32 bytes → 64 hex chars)
gen_secret() {
  local length="${1:-32}"
  openssl rand -hex "${length}" 2>/dev/null \
    || head -c "${length}" /dev/urandom | od -An -tx1 | tr -d ' \n'
}

# ─── OS Detection ────────────────────────────────────────────────────────────

# Validate that the host OS is a supported distribution.
# Exits with error if unsupported.
detect_os() {
  [[ -f /etc/os-release ]] || err "Unsupported OS: /etc/os-release not found"
  local os_id os_version
  os_id=$(. /etc/os-release && echo "$ID")
  os_version=$(. /etc/os-release && echo "$VERSION_ID")
  case "${os_id}" in
    ubuntu)
      if [[ "${os_version%%.*}" -lt 22 ]]; then
        err "Unsupported: ${os_id} ${os_version}. Need Ubuntu 22.04+"
      fi
      ;;
    debian)
      if [[ "${os_version%%.*}" -lt 12 ]]; then
        err "Unsupported: ${os_id} ${os_version}. Need Debian 12+"
      fi
      ;;
    *)
      err "Unsupported: ${os_id} ${os_version}. Need Ubuntu 22.04+ or Debian 12+"
      ;;
  esac
  log "Detected ${os_id} ${os_version}"
}

# ─── Precondition Checks ─────────────────────────────────────────────────────

# Require the script is running as root (EUID 0).
require_root() {
  [[ "$(id -u)" -eq 0 ]] || err "Must run as root"
}

# Require Docker daemon is installed and reachable.
require_docker() {
  command -v docker &>/dev/null || err "Docker is not installed"
  docker info &>/dev/null || err "Docker daemon not available"
}

# Require a specific Docker container is running.
# Usage: require_container <name>
require_container() {
  local name="${1:?require_container: container name required}"
  require_docker
  local state
  state=$(docker inspect -f '{{.State.Status}}' "${name}" 2>/dev/null) || true
  if [[ "${state}" != "running" ]]; then
    err "Container '${name}' is not running (state: ${state:-not found})"
  fi
}

# ─── User Interaction ─────────────────────────────────────────────────────────

# Prompt the user for confirmation. Aborts (exit 1) if not "yes".
# Usage: confirm "Are you sure you want to do X?"
confirm() {
  local msg="${1:-Are you sure?}"
  echo -e "${YELLOW}${msg}${NC}"
  read -rp "Type 'yes' to confirm: " answer </dev/tty
  if [[ "${answer}" != "yes" ]]; then
    echo "Operation cancelled."
    exit 1
  fi
}

# ─── Formatting ──────────────────────────────────────────────────────────────

# Format a byte count into the largest whole-number unit (B, KB, MB, GB).
# Uses 1024-based units.
# Usage: human_bytes 1048576  → "1 MB"
human_bytes() {
  local bytes="${1:?human_bytes: byte count required}"

  # Validate input is a non-negative integer
  if ! [[ "${bytes}" =~ ^[0-9]+$ ]]; then
    echo "0 B"
    return
  fi

  if (( bytes >= 1073741824 )); then
    echo "$(( bytes / 1073741824 )) GB"
  elif (( bytes >= 1048576 )); then
    echo "$(( bytes / 1048576 )) MB"
  elif (( bytes >= 1024 )); then
    echo "$(( bytes / 1024 )) KB"
  else
    echo "${bytes} B"
  fi
}

# ─── Validation ──────────────────────────────────────────────────────────────

# Validate that a port number is within the acceptable range (1024–65535).
# Exits with error if invalid.
# Usage: validate_port 8080
validate_port() {
  local port="${1:?validate_port: port number required}"

  # Must be a positive integer
  if ! [[ "${port}" =~ ^[0-9]+$ ]]; then
    err "Invalid port '${port}': must be a number between 1024 and 65535"
  fi

  if (( port < 1024 || port > 65535 )); then
    err "Invalid port '${port}': must be between 1024 and 65535"
  fi
}

# Validate that a version tag exists in a remote git repository.
# Uses `git ls-remote` to check without cloning.
# Usage: validate_version_tag <tag> <repo_url>
#   validate_version_tag "v1.2.3" "https://github.com/anonysec/panel.git"
validate_version_tag() {
  local tag="${1:?validate_version_tag: tag required}"
  local repo_url="${2:?validate_version_tag: repository URL required}"

  command -v git &>/dev/null || err "git is not installed"

  local refs
  refs=$(git ls-remote --tags --refs "${repo_url}" "refs/tags/${tag}" 2>/dev/null)

  if [[ -z "${refs}" ]]; then
    err "Tag '${tag}' not found in remote repository ${repo_url}"
  fi
}
