#!/usr/bin/env bash
#
# KorisPanel Management CLI
# Usage: koris [command]
#

red='\033[0;31m'; green='\033[0;32m'; yellow='\033[0;33m'; blue='\033[0;34m'; cyan='\033[0;36m'; bold='\033[1m'; dim='\033[2m'; plain='\033[0m'
info()  { echo -e "${green}[+]${plain} $*"; }
warn()  { echo -e "${yellow}[!]${plain} $*"; }
error() { echo -e "${red}[-]${plain} $*"; }

INSTALL_DIR="/opt/KorisPanel"
PANEL_ENV="/etc/koris/panel.env"
NODE_ENV="/etc/knode/node.env"
COMPOSE_FILE="${INSTALL_DIR}/docker-compose.yml"

# Signal trapping for clean exit
trap 'echo ""; echo -e "${yellow}Operation cancelled.${plain}"; exit 130' INT TERM

is_panel() { [[ -f "$COMPOSE_FILE" ]] && command -v docker &>/dev/null; }
is_node()  { docker ps --format '{{.Names}}' 2>/dev/null | grep -qx knode; }
get_version() { cat "$INSTALL_DIR/VERSION" 2>/dev/null || echo "?"; }

panel_status() {
    docker inspect -f '{{.State.Status}}' koris 2>/dev/null || echo "not running"
}
node_status() {
    if docker ps --format '{{.Names}}' 2>/dev/null | grep -qx knode; then
        echo "running"
    else
        echo "not running"
    fi
}

cmd_start() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }
    cd "$INSTALL_DIR" && docker compose up -d && info "Panel started"
}
cmd_stop() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }
    cd "$INSTALL_DIR" && docker compose down && info "Panel stopped"
}
cmd_restart() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }
    cd "$INSTALL_DIR" && docker compose restart && info "Panel restarted"
}

cmd_status() {
    echo -e "${bold}${blue}KorisPanel${plain} v$(get_version)"
    echo "───────────────────────────────────"
    printf "  %-14s %s\n" "Panel:" "$(panel_status)"
    printf "  %-14s %s\n" "Node Agent:" "$(node_status)"
    if is_panel; then
        local addr=$(grep 'PANEL_ADDR' "$PANEL_ENV" 2>/dev/null | cut -d= -f2 | tr -d "'\"")
        printf "  %-14s %s\n" "Listen:" "${addr:-?}"
        curl -fsSk "https://${addr}/api/health" >/dev/null 2>&1 && printf "  %-14s ${green}%s${plain}\n" "Health:" "OK" || printf "  %-14s ${red}%s${plain}\n" "Health:" "FAIL"
    fi
    echo "───────────────────────────────────"
    printf "  %-14s %s\n" "CPU:" "$(nproc) cores"
    printf "  %-14s %s\n" "RAM:" "$(free -h | awk '/^Mem:/{print $3"/"$2}')"
    printf "  %-14s %s\n" "Disk:" "$(df -h / | awk 'NR==2{print $3"/"$2" ("$5")"}')"
}

cmd_logs() {
    cd "$INSTALL_DIR" && docker compose logs --tail 50
}

cmd_follow() {
    cd "$INSTALL_DIR" && exec docker compose logs -f
}

cmd_update() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }
    [[ ! -d "$INSTALL_DIR/.git" ]] && { error "Not a git install at $INSTALL_DIR"; exit 1; }

    # Parse --version flag
    local target_tag=""
    for arg in "$@"; do
        case "${arg}" in
            --version=*) target_tag="${arg#*=}" ;;
            *)           error "Unknown flag: ${arg}"; exit 1 ;;
        esac
    done

    cd "$INSTALL_DIR"

    # Read current version
    local old_version
    old_version=$(cat "$INSTALL_DIR/VERSION" 2>/dev/null || echo "unknown")

    # Store current version before modification (for rollback reference)
    mkdir -p /etc/koris
    echo "${old_version}" > /etc/koris/version

    # Fetch all tags and branches
    git fetch --all --tags --quiet 2>/dev/null

    if [[ -n "${target_tag}" ]]; then
        # Validate tag exists
        local tag_exists
        tag_exists=$(git tag -l "${target_tag}")
        if [[ -z "${tag_exists}" ]]; then
            # Try remote
            tag_exists=$(git ls-remote --tags origin "refs/tags/${target_tag}" 2>/dev/null)
            if [[ -z "${tag_exists}" ]]; then
                error "Tag '${target_tag}' not found in repository"; exit 1
            fi
        fi

        # Check if already at target version
        if [[ "${old_version}" == "${target_tag}" ]]; then
            info "Already at version ${target_tag}. Nothing to do."
            return
        fi

        git checkout "${target_tag}" --quiet 2>/dev/null || { error "Failed to checkout tag '${target_tag}'"; exit 1; }
    else
        # Pull latest from main
        git checkout main --quiet 2>/dev/null || true
        git pull origin main --quiet 2>/dev/null || { error "Failed to pull latest from main"; exit 1; }

        local new_version
        new_version=$(cat "$INSTALL_DIR/VERSION" 2>/dev/null || echo "unknown")

        # Check if already up to date
        if [[ "${old_version}" == "${new_version}" ]]; then
            info "Already up to date (v${new_version})."
            return
        fi
    fi

    local new_version
    new_version=$(cat "$INSTALL_DIR/VERSION" 2>/dev/null || echo "unknown")

    info "Updating v${old_version} → v${new_version}..."

    # Rebuild
    docker compose up -d --build || { error "Docker build/start failed"; exit 1; }

    # Display changelog (max 50 lines)
    if [[ "${old_version}" != "unknown" && "${new_version}" != "unknown" ]]; then
        local changelog
        changelog=$(git log --oneline "${old_version}..${new_version}" 2>/dev/null | head -50)
        if [[ -n "${changelog}" ]]; then
            echo ""
            echo -e "${cyan}Changelog (${old_version} → ${new_version}):${plain}"
            echo "${changelog}" | sed 's/^/  /'
            echo ""
        fi
    fi

    # Health check: poll every 2s for 60s
    info "Checking health..."
    local attempts=0
    local healthy=""
    local panel_port
    panel_port=$(grep -oP 'PANEL_PORT=\K.*' "$PANEL_ENV" 2>/dev/null || echo "2026")

    while [[ ${attempts} -lt 30 ]]; do
        if curl -fsSk "https://localhost:${panel_port}/api/health" >/dev/null 2>&1; then
            healthy="yes"
            break
        fi
        sleep 2
        attempts=$((attempts + 1))
    done

    if [[ "${healthy}" == "yes" ]]; then
        info "Update complete: v${old_version} → v${new_version} ✓"
        echo "${new_version}" > /etc/koris/version
    else
        error "Health check failed after 60 seconds"
        echo ""
        echo -e "${red}Last 20 lines of container logs:${plain}"
        docker logs koris --tail 20 2>&1 | sed 's/^/  /'
        echo ""
        warn "Suggest: koris downgrade ${old_version}"
        exit 1
    fi

    # Update CLI self
    [[ -f "$INSTALL_DIR/panel/koris.sh" ]] && { cp "$INSTALL_DIR/panel/koris.sh" /usr/local/bin/koris 2>/dev/null; chmod +x /usr/local/bin/koris 2>/dev/null; }
}

cmd_uninstall() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }

    local keep_data=""
    for arg in "$@"; do
        case "${arg}" in
            --keep-data) keep_data="yes" ;;
            *)           error "Unknown flag: ${arg}"; exit 1 ;;
        esac
    done

    # Display summary
    echo -e "${red}${bold}KorisPanel Uninstall${plain}"
    echo ""
    echo "  The following will be removed:"
    echo "    • Docker containers: koris, koris-db, koris-pgadmin"
    echo "    • Docker images: koris project images"
    if [[ "${keep_data}" == "yes" ]]; then
        echo "    • Docker volumes: koris_panel-data, koris_pgadmin-data (DB preserved)"
    else
        echo "    • Docker volumes: koris_db-data, koris_panel-data, koris_pgadmin-data"
    fi
    echo "    • Installation directory: /opt/KorisPanel"
    echo "    • Configuration: /etc/koris"
    echo "    • CLI binary: /usr/local/bin/koris"
    echo "    • Certbot cron jobs for Koris"
    if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx knode; then
        echo "    • knode container, image, and /etc/knode"
    fi
    echo ""

    read -rp "Type 'yes' to confirm uninstall: " confirm
    if [[ "${confirm}" != "yes" ]]; then
        info "Uninstall cancelled."
        return
    fi

    local -a failures=()

    # 1. Stop and remove Docker Compose stack
    info "Stopping containers..."
    if [[ -d "$INSTALL_DIR" ]]; then
        cd "$INSTALL_DIR"
        docker compose down --remove-orphans 2>/dev/null || failures+=("docker compose down")
    fi
    # Force-remove individual containers if still present
    for ctr in koris koris-db koris-pgadmin; do
        if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx "${ctr}"; then
            docker rm -f "${ctr}" 2>/dev/null || failures+=("remove container ${ctr}")
        fi
    done

    # 2. Remove project images
    info "Removing project images..."
    local images
    images=$(docker images --format '{{.ID}} {{.Repository}}' 2>/dev/null | awk '$2 ~ /^koris/ {print $1}')
    images+=" $(docker images --filter "label=com.docker.compose.project=koris" -q 2>/dev/null)"
    for img in $(echo "${images}" | tr ' ' '\n' | sort -u | grep -v '^$'); do
        docker rmi -f "${img}" 2>/dev/null || failures+=("remove image ${img}")
    done

    # 3. Remove volumes
    info "Removing volumes..."
    if [[ "${keep_data}" == "yes" ]]; then
        for vol in koris_panel-data koris_pgadmin-data; do
            docker volume rm "${vol}" 2>/dev/null || failures+=("remove volume ${vol}")
        done
    else
        for vol in koris_db-data koris_panel-data koris_pgadmin-data; do
            docker volume rm "${vol}" 2>/dev/null || failures+=("remove volume ${vol}")
        done
    fi

    # 4. Remove installation directory
    info "Removing /opt/KorisPanel..."
    rm -rf /opt/KorisPanel 2>/dev/null || failures+=("remove /opt/KorisPanel")

    # 5. Remove configuration
    if [[ "${keep_data}" != "yes" ]]; then
        info "Removing /etc/koris..."
        rm -rf /etc/koris 2>/dev/null || failures+=("remove /etc/koris")
    fi

    # 6. Remove CLI binary
    rm -f /usr/local/bin/koris 2>/dev/null || failures+=("remove /usr/local/bin/koris")

    # 7. Remove certbot cron jobs
    if crontab -l 2>/dev/null | grep -q "koris\|KorisPanel"; then
        crontab -l 2>/dev/null | grep -v "koris\|KorisPanel" | crontab - 2>/dev/null || failures+=("remove certbot cron")
    fi

    # 8. Handle knode if present
    if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx knode; then
        info "Removing knode..."
        docker stop knode 2>/dev/null || true
        docker rm -f knode 2>/dev/null || failures+=("remove knode container")
        docker rmi knode:latest 2>/dev/null || failures+=("remove knode image")
        rm -rf /etc/knode 2>/dev/null || failures+=("remove /etc/knode")
    fi

    # Summary
    echo ""
    if [[ ${#failures[@]} -gt 0 ]]; then
        warn "Uninstall completed with ${#failures[@]} error(s):"
        for f in "${failures[@]}"; do
            echo -e "    ${red}✗${plain} ${f}"
        done
    else
        info "KorisPanel completely uninstalled."
    fi
    if [[ "${keep_data}" == "yes" ]]; then
        info "Database volume and backups preserved."
    fi
}

cmd_config() {
    is_panel && { echo -e "${cyan}Panel Config:${plain}"; grep -v 'SECRET\|PASSWORD\|TOKEN' "$PANEL_ENV" 2>/dev/null | sed 's/^/  /'; echo "  (secrets hidden)"; }
    is_node  && { echo -e "${cyan}Node Config:${plain}"; grep -v 'TOKEN' "$NODE_ENV" 2>/dev/null | sed 's/^/  /'; echo "  (token hidden)"; }
}

cmd_ssl() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }
    echo -e "${bold}${cyan}SSL Certificate Manager${plain}"
    echo ""

    # Show current SSL status
    if [[ -f /etc/letsencrypt/live/*/fullchain.pem ]] 2>/dev/null; then
        CERT_DOMAIN=$(ls /etc/letsencrypt/live/ 2>/dev/null | grep -v README | head -1)
        if [[ -n "$CERT_DOMAIN" ]]; then
            EXPIRY=$(openssl x509 -enddate -noout -in "/etc/letsencrypt/live/${CERT_DOMAIN}/fullchain.pem" 2>/dev/null | cut -d= -f2)
            echo -e "  ${green}●${plain} SSL active for: ${cyan}${CERT_DOMAIN}${plain}"
            echo -e "  ${dim}Expires: ${EXPIRY}${plain}"
            echo ""
        fi
    else
        echo -e "  ${yellow}●${plain} No SSL certificate found"
        echo ""
    fi

    echo -e "  ${green}1.${plain} Install/renew SSL certificate"
    echo -e "  ${green}2.${plain} Force renewal"
    echo -e "  ${green}3.${plain} Remove SSL (revert to HTTP)"
    echo -e "  ${green}0.${plain} Back"
    echo ""
    read -rp "$(echo -e "${cyan}Choose: ${plain}")" ssl_ch

    case "$ssl_ch" in
        1)
            read -rp "$(echo -e "${cyan}Domain: ${plain}")" SSL_DOMAIN
            [[ -z "$SSL_DOMAIN" ]] && { error "Domain required."; return; }
            read -rp "$(echo -e "${cyan}Email (for renewal notices, blank to skip): ${plain}")" SSL_EMAIL

            # Install certbot if missing
            if ! command -v certbot >/dev/null 2>&1; then
                info "Installing Certbot..."
                if [[ -f /etc/debian_version ]]; then
                    apt-get update -qq >/dev/null 2>&1
                    apt-get install -y -qq certbot >/dev/null 2>&1
                else
                    dnf install -y -q certbot >/dev/null 2>&1
                fi
            fi

            if ! command -v certbot >/dev/null 2>&1; then
                error "Failed to install certbot."; return
            fi

            # Obtain certificate using standalone mode (panel serves TLS directly)
            EMAIL_ARG="--register-unsafely-without-email"
            [[ -n "$SSL_EMAIL" ]] && EMAIL_ARG="--email $SSL_EMAIL"

            # Stop panel briefly for HTTP-01 challenge on port 80
            info "Stopping panel temporarily for certificate challenge..."
            cd "$INSTALL_DIR" && docker compose stop koris

            info "Requesting certificate for ${SSL_DOMAIN}..."
            if certbot certonly --standalone -d "$SSL_DOMAIN" --non-interactive --agree-tos $EMAIL_ARG; then
                info "SSL certificate obtained successfully!"
                # Copy certs to /etc/koris/ for the panel to use
                cp "/etc/letsencrypt/live/${SSL_DOMAIN}/fullchain.pem" /etc/koris/tls.crt 2>/dev/null
                cp "/etc/letsencrypt/live/${SSL_DOMAIN}/privkey.pem" /etc/koris/tls.key 2>/dev/null
                chmod 600 /etc/koris/tls.key

                # Enable secure cookies
                if grep -q '^PANEL_SECURE_COOKIES=' "$PANEL_ENV" 2>/dev/null; then
                    sed -i "s|^PANEL_SECURE_COOKIES=.*|PANEL_SECURE_COOKIES='true'|" "$PANEL_ENV"
                else
                    echo "PANEL_SECURE_COOKIES='true'" >> "$PANEL_ENV"
                fi

                # Restart panel with new certs
                docker compose up -d
                echo ""
                echo -e "  ${green}✓${plain} https://${SSL_DOMAIN}/dashboard/"
                echo -e "  ${dim}Auto-renewal is enabled via certbot timer.${plain}"
            else
                error "Certbot failed. Check that:"
                echo "  - DNS A record for ${SSL_DOMAIN} points to this server"
                echo "  - Port 80 is open (for HTTP-01 challenge)"
                # Restart panel regardless
                docker compose up -d
            fi
            ;;
        2)
            info "Stopping panel temporarily for renewal..."
            cd "$INSTALL_DIR" && docker compose stop koris
            certbot renew --force-renewal
            # Copy renewed certs
            CERT_DOMAIN=$(ls /etc/letsencrypt/live/ 2>/dev/null | grep -v README | head -1)
            if [[ -n "$CERT_DOMAIN" ]]; then
                cp "/etc/letsencrypt/live/${CERT_DOMAIN}/fullchain.pem" /etc/koris/tls.crt 2>/dev/null
                cp "/etc/letsencrypt/live/${CERT_DOMAIN}/privkey.pem" /etc/koris/tls.key 2>/dev/null
                chmod 600 /etc/koris/tls.key
            fi
            docker compose up -d
            info "Done."
            ;;
        3)
            read -rp "$(echo -e "${yellow}Remove SSL and revert to HTTP? [y/N]: ${plain}")" confirm
            [[ "$confirm" != "y" && "$confirm" != "Y" ]] && return
            # Remove cert files
            rm -f /etc/koris/tls.crt /etc/koris/tls.key
            # Disable secure cookies
            sed -i "s|^PANEL_SECURE_COOKIES=.*|PANEL_SECURE_COOKIES='false'|" "$PANEL_ENV" 2>/dev/null
            cd "$INSTALL_DIR" && docker compose restart koris
            info "SSL removed. Panel is now HTTP-only."
            ;;
        0) return ;;
        *) warn "Invalid." ;;
    esac
}

# Format bytes to human-readable (B/KB/MB/GB)
human_bytes() {
    local bytes="${1:-0}"
    if ! [[ "${bytes}" =~ ^[0-9]+$ ]]; then
        echo "0 B"
        return
    fi
    if (( bytes >= 1073741824 )); then echo "$(( bytes / 1073741824 )) GB"
    elif (( bytes >= 1048576 )); then echo "$(( bytes / 1048576 )) MB"
    elif (( bytes >= 1024 )); then echo "$(( bytes / 1024 )) KB"
    else echo "${bytes} B"; fi
}

cmd_clean() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }
    docker info &>/dev/null || { error "Docker daemon not available"; exit 1; }

    local do_volumes="" do_include_db="" do_all="" do_force=""
    for arg in "$@"; do
        case "${arg}" in
            --volumes)    do_volumes="yes" ;;
            --include-db) do_include_db="yes" ;;
            --all)        do_all="yes" ;;
            --force)      do_force="yes" ;;
            *)            error "Unknown flag: ${arg}"; exit 1 ;;
        esac
    done

    # --all implies everything
    if [[ "${do_all}" == "yes" ]]; then
        if [[ "${do_force}" != "yes" ]]; then
            echo -e "${red}This will remove ALL project volumes (including database), images, and build cache.${plain}"
            read -rp "Type 'yes' to confirm: " confirm
            [[ "${confirm}" != "yes" ]] && { info "Cancelled."; return; }
        fi
        do_volumes="yes"
        do_include_db="yes"
    fi

    local total_reclaimed=0

    # Remove dangling/project images
    info "Removing project images..."
    local img_output
    img_output=$(docker images --filter "label=com.docker.compose.project=koris" -q 2>/dev/null)
    # Also get images starting with "koris"
    local koris_images
    koris_images=$(docker images --format '{{.ID}} {{.Repository}}' | awk '$2 ~ /^koris/ {print $1}')
    local all_images
    all_images=$(echo -e "${img_output}\n${koris_images}" | sort -u | grep -v '^$')

    if [[ -n "${all_images}" ]]; then
        for img_id in ${all_images}; do
            local size
            size=$(docker image inspect "${img_id}" --format='{{.Size}}' 2>/dev/null || echo "0")
            if docker rmi -f "${img_id}" &>/dev/null; then
                total_reclaimed=$((total_reclaimed + size))
            fi
        done
    fi

    # Prune build cache
    info "Pruning Docker build cache..."
    local cache_output
    cache_output=$(docker builder prune -f 2>&1 || true)
    # Extract reclaimed bytes from builder prune output if possible
    local cache_bytes
    cache_bytes=$(echo "${cache_output}" | grep -oP 'Total:\s+\K[0-9]+' || echo "0")
    if [[ -n "${cache_bytes}" && "${cache_bytes}" =~ ^[0-9]+$ ]]; then
        total_reclaimed=$((total_reclaimed + cache_bytes))
    fi

    # Remove volumes if requested
    if [[ "${do_volumes}" == "yes" ]]; then
        info "Removing project volumes..."
        for vol in koris_panel-data koris_pgadmin-data; do
            # Check if volume is in use
            local container_using
            container_using=$(docker ps --filter "volume=${vol}" --format '{{.Names}}' 2>/dev/null | head -1)
            if [[ -n "${container_using}" ]]; then
                warn "Volume '${vol}' is in use by container '${container_using}' — skipping"
                continue
            fi
            if docker volume rm "${vol}" &>/dev/null; then
                info "Removed volume: ${vol}"
            fi
        done

        if [[ "${do_include_db}" == "yes" ]]; then
            local container_using
            container_using=$(docker ps --filter "volume=koris_db-data" --format '{{.Names}}' 2>/dev/null | head -1)
            if [[ -n "${container_using}" ]]; then
                warn "Volume 'koris_db-data' is in use by container '${container_using}' — skipping"
            elif docker volume rm koris_db-data &>/dev/null; then
                info "Removed volume: koris_db-data"
            fi
        fi
    fi

    info "Clean complete. Space reclaimed: $(human_bytes ${total_reclaimed})"
}

cmd_db() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }

    # Check koris-db container is running
    local db_state
    db_state=$(docker inspect -f '{{.State.Status}}' koris-db 2>/dev/null || echo "not found")
    [[ "${db_state}" != "running" ]] && { error "Database container 'koris-db' is not running (state: ${db_state})"; exit 1; }

    local subcmd="${1:-}"
    shift 2>/dev/null || true

    case "${subcmd}" in
        backup)  cmd_db_backup "$@" ;;
        restore) cmd_db_restore "$@" ;;
        migrate) cmd_db_migrate ;;
        reset)   cmd_db_reset ;;
        shell)   cmd_db_shell ;;
        status)  cmd_db_status ;;
        *)       error "Usage: koris db [backup|restore|migrate|reset|shell|status]"; exit 1 ;;
    esac
}

cmd_db_backup() {
    local backup_dir="/var/backups/koris"

    # Parse --path flag
    for arg in "$@"; do
        case "${arg}" in
            --path=*) backup_dir="${arg#*=}" ;;
            *)        error "Unknown flag: ${arg}"; exit 1 ;;
        esac
    done

    # Validate backup directory
    if [[ ! -d "${backup_dir}" ]]; then
        mkdir -p "${backup_dir}" 2>/dev/null || { error "Cannot create directory: ${backup_dir}"; exit 1; }
    fi
    [[ -w "${backup_dir}" ]] || { error "Directory not writable: ${backup_dir}"; exit 1; }

    local db_name db_user
    db_name=$(grep -oP 'POSTGRES_DB=\K.*' "$PANEL_ENV" 2>/dev/null || echo "koris")
    db_user=$(grep -oP 'POSTGRES_USER=\K.*' "$PANEL_ENV" 2>/dev/null || echo "koris")

    local timestamp
    timestamp=$(date -u +"%Y%m%d-%H%M%S")
    local backup_file="${backup_dir}/koris-${timestamp}.sql.gz"

    info "Creating database backup..."
    docker exec koris-db pg_dump -U "${db_user}" "${db_name}" | gzip > "${backup_file}" \
        || { rm -f "${backup_file}"; error "Backup failed"; exit 1; }

    local size
    size=$(stat -c%s "${backup_file}" 2>/dev/null || echo "0")
    info "Backup saved: ${backup_file} ($(human_bytes ${size}))"
}

cmd_db_restore() {
    local restore_file="${1:-}"
    local db_name db_user
    db_name=$(grep -oP 'POSTGRES_DB=\K.*' "$PANEL_ENV" 2>/dev/null || echo "koris")
    db_user=$(grep -oP 'POSTGRES_USER=\K.*' "$PANEL_ENV" 2>/dev/null || echo "koris")

    # If no file specified, list available backups
    if [[ -z "${restore_file}" ]]; then
        local backup_dir="/var/backups/koris"
        if [[ ! -d "${backup_dir}" ]] || [[ -z "$(ls -A "${backup_dir}" 2>/dev/null)" ]]; then
            error "No backups found in ${backup_dir}"; exit 1
        fi
        info "Available backups:"
        local i=1
        local -a files=()
        while IFS= read -r f; do
            files+=("${f}")
            printf "  ${cyan}%d)${plain} %s (%s)\n" "$i" "$(basename "${f}")" "$(human_bytes $(stat -c%s "${f}" 2>/dev/null || echo 0))"
            i=$((i + 1))
        done < <(ls -t "${backup_dir}"/koris-*.sql.gz 2>/dev/null)

        [[ ${#files[@]} -eq 0 ]] && { error "No .sql.gz backups found"; exit 1; }

        echo ""
        read -rp "$(echo -e "${cyan}Select backup number: ${plain}")" selection
        if ! [[ "${selection}" =~ ^[0-9]+$ ]] || (( selection < 1 || selection > ${#files[@]} )); then
            error "Invalid selection"; exit 1
        fi
        restore_file="${files[$((selection - 1))]}"
    fi

    # Validate file exists
    [[ -f "${restore_file}" ]] || { error "File not found: ${restore_file}"; exit 1; }
    # Validate file is valid gzip
    gzip -t "${restore_file}" 2>/dev/null || { error "File is not a valid gzip archive: ${restore_file}"; exit 1; }

    # Confirmation prompt
    echo -e "${red}This will OVERWRITE the current database with the backup.${plain}"
    read -rp "Type 'yes' to confirm: " confirm
    [[ "${confirm}" != "yes" ]] && { info "Cancelled."; return; }

    info "Restoring database from: $(basename "${restore_file}")..."

    # Drop and recreate database
    docker exec koris-db psql -U "${db_user}" -d postgres -c "DROP DATABASE IF EXISTS ${db_name};" 2>/dev/null
    docker exec koris-db psql -U "${db_user}" -d postgres -c "CREATE DATABASE ${db_name} OWNER ${db_user};" 2>/dev/null

    # Restore dump
    gunzip -c "${restore_file}" | docker exec -i koris-db psql -U "${db_user}" -d "${db_name}" >/dev/null 2>&1 \
        || { error "Restore failed"; exit 1; }

    info "Database restored successfully from $(basename "${restore_file}")"
}

cmd_db_migrate() {
    local db_name db_user
    db_name=$(grep -oP 'POSTGRES_DB=\K.*' "$PANEL_ENV" 2>/dev/null || echo "koris")
    db_user=$(grep -oP 'POSTGRES_USER=\K.*' "$PANEL_ENV" 2>/dev/null || echo "koris")

    info "Running database migrations..."
    local output
    output=$(docker exec koris /app/migrate-db 2>&1) || { error "Migration failed: ${output}"; exit 1; }

    # Try to extract migration count from output
    local count
    count=$(echo "${output}" | grep -oP '\d+ migration' | grep -oP '\d+' || echo "")
    if [[ -n "${count}" ]]; then
        info "Migrations complete: ${count} applied"
    else
        info "Migrations complete"
        echo "${output}" | tail -5
    fi
}

cmd_db_reset() {
    local db_name db_user
    db_name=$(grep -oP 'POSTGRES_DB=\K.*' "$PANEL_ENV" 2>/dev/null || echo "koris")
    db_user=$(grep -oP 'POSTGRES_USER=\K.*' "$PANEL_ENV" 2>/dev/null || echo "koris")

    echo -e "${red}WARNING: This will DROP and recreate the database, then run all migrations from scratch.${plain}"
    echo -e "${red}ALL DATA WILL BE LOST.${plain}"
    read -rp "Type 'yes' to confirm: " confirm
    [[ "${confirm}" != "yes" ]] && { info "Cancelled."; return; }

    info "Dropping database '${db_name}'..."
    docker exec koris-db psql -U "${db_user}" -d postgres -c "DROP DATABASE IF EXISTS ${db_name};" 2>/dev/null
    docker exec koris-db psql -U "${db_user}" -d postgres -c "CREATE DATABASE ${db_name} OWNER ${db_user};" 2>/dev/null
    info "Database recreated. Running migrations..."

    # Run migrations
    local output
    output=$(docker exec koris /app/migrate-db 2>&1) || { error "Migration failed: ${output}"; exit 1; }
    info "Database reset complete — all migrations applied from scratch"
}

cmd_db_shell() {
    local db_name db_user
    db_name=$(grep -oP 'POSTGRES_DB=\K.*' "$PANEL_ENV" 2>/dev/null || echo "koris")
    db_user=$(grep -oP 'POSTGRES_USER=\K.*' "$PANEL_ENV" 2>/dev/null || echo "koris")

    info "Opening psql shell (Ctrl+D or \\q to exit)..."
    exec docker exec -it koris-db psql -U "${db_user}" -d "${db_name}"
}

cmd_db_status() {
    local db_name db_user
    db_name=$(grep -oP 'POSTGRES_DB=\K.*' "$PANEL_ENV" 2>/dev/null || echo "koris")
    db_user=$(grep -oP 'POSTGRES_USER=\K.*' "$PANEL_ENV" 2>/dev/null || echo "koris")

    echo -e "${bold}${cyan}Database Status${plain}"
    echo "───────────────────────────────────"

    # Database size
    local db_size
    db_size=$(docker exec koris-db psql -U "${db_user}" -d "${db_name}" -t -c "SELECT pg_size_pretty(pg_database_size('${db_name}'));" 2>/dev/null | xargs)
    printf "  %-20s %s\n" "Size:" "${db_size:-unknown}"

    # Active connections
    local connections
    connections=$(docker exec koris-db psql -U "${db_user}" -d "${db_name}" -t -c "SELECT count(*) FROM pg_stat_activity WHERE datname='${db_name}';" 2>/dev/null | xargs)
    printf "  %-20s %s\n" "Connections:" "${connections:-unknown}"

    # TimescaleDB version
    local tsdb_version
    tsdb_version=$(docker exec koris-db psql -U "${db_user}" -d "${db_name}" -t -c "SELECT extversion FROM pg_extension WHERE extname='timescaledb';" 2>/dev/null | xargs)
    printf "  %-20s %s\n" "TimescaleDB:" "${tsdb_version:-not installed}"

    # Replication status
    local replication
    replication=$(docker exec koris-db psql -U "${db_user}" -d "${db_name}" -t -c "SELECT count(*) FROM pg_stat_replication;" 2>/dev/null | xargs)
    if [[ "${replication}" == "0" || -z "${replication}" ]]; then
        printf "  %-20s %s\n" "Replication:" "none (standalone)"
    else
        printf "  %-20s %s replicas\n" "Replication:" "${replication}"
    fi

    echo "───────────────────────────────────"
}

cmd_pgadmin() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }
    docker info &>/dev/null || { error "Docker daemon not available"; exit 1; }

    local subcmd="${1:-}"
    shift 2>/dev/null || true

    case "${subcmd}" in
        status)         pgadmin_status ;;
        enable)         pgadmin_enable ;;
        disable)        pgadmin_disable ;;
        url)            pgadmin_url ;;
        reset-password) pgadmin_reset_password ;;
        port)           pgadmin_port "$@" ;;
        *)              error "Usage: koris pgadmin [status|enable|disable|url|reset-password|port <number>]"; exit 1 ;;
    esac
}

pgadmin_status() {
    local state
    state=$(docker inspect -f '{{.State.Status}}' koris-pgadmin 2>/dev/null || echo "not found")
    if [[ "${state}" == "running" ]]; then
        local port
        port=$(grep -oP 'PGADMIN_PORT=\K.*' "$PANEL_ENV" 2>/dev/null || echo "5050")
        local ip
        ip=$(curl -fsS4 --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')
        echo -e "  ${green}●${plain} pgAdmin is ${green}running${plain}"
        echo -e "  URL:  ${cyan}http://${ip}:${port}${plain}"
        echo -e "  Port: ${port}"
    else
        echo -e "  ${red}●${plain} pgAdmin is ${red}${state}${plain}"
    fi
}

pgadmin_enable() {
    local state
    state=$(docker inspect -f '{{.State.Status}}' koris-pgadmin 2>/dev/null || echo "not found")
    if [[ "${state}" == "running" ]]; then
        info "pgAdmin is already running."
        return
    fi

    info "Starting pgAdmin..."
    docker start koris-pgadmin 2>/dev/null || { cd "$INSTALL_DIR" && docker compose up -d koris-pgadmin; }
    docker update --restart unless-stopped koris-pgadmin 2>/dev/null || true

    # Wait up to 30s
    local attempts=0
    while [[ ${attempts} -lt 15 ]]; do
        state=$(docker inspect -f '{{.State.Status}}' koris-pgadmin 2>/dev/null || echo "")
        [[ "${state}" == "running" ]] && break
        sleep 2
        attempts=$((attempts + 1))
    done

    if [[ "${state}" == "running" ]]; then
        local port ip
        port=$(grep -oP 'PGADMIN_PORT=\K.*' "$PANEL_ENV" 2>/dev/null || echo "5050")
        ip=$(curl -fsS4 --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')
        info "pgAdmin is running at http://${ip}:${port}"
    else
        error "pgAdmin failed to start within 30 seconds"
    fi
}

pgadmin_disable() {
    docker stop koris-pgadmin 2>/dev/null || true
    docker update --restart no koris-pgadmin 2>/dev/null || true
    info "pgAdmin stopped and autostart disabled."
}

pgadmin_url() {
    local state
    state=$(docker inspect -f '{{.State.Status}}' koris-pgadmin 2>/dev/null || echo "not found")
    if [[ "${state}" != "running" ]]; then
        error "pgAdmin is not running. Start it with: koris pgadmin enable"; exit 1
    fi
    local port ip
    port=$(grep -oP 'PGADMIN_PORT=\K.*' "$PANEL_ENV" 2>/dev/null || echo "5050")
    ip=$(curl -fsS4 --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')
    echo "http://${ip}:${port}"
}

pgadmin_reset_password() {
    local state
    state=$(docker inspect -f '{{.State.Status}}' koris-pgadmin 2>/dev/null || echo "not found")
    [[ "${state}" != "running" ]] && { error "pgAdmin is not running"; exit 1; }

    read -rsp "$(echo -e "${cyan}New pgAdmin password (min 8 chars): ${plain}")" new_pass
    echo ""
    if [[ ${#new_pass} -lt 8 ]]; then
        error "Password must be at least 8 characters"; exit 1
    fi

    # Update panel.env
    sed -i "s|^PGADMIN_PASSWORD=.*|PGADMIN_PASSWORD=${new_pass}|" "$PANEL_ENV"

    # Restart pgAdmin with new password
    docker stop koris-pgadmin 2>/dev/null
    cd "$INSTALL_DIR" && docker compose up -d koris-pgadmin
    info "pgAdmin password updated and service restarted."
}

pgadmin_port() {
    local new_port="${1:-}"
    [[ -z "${new_port}" ]] && { error "Usage: koris pgadmin port <number>"; exit 1; }

    # Validate port range
    if ! [[ "${new_port}" =~ ^[0-9]+$ ]] || (( new_port < 1024 || new_port > 65535 )); then
        error "Invalid port '${new_port}': must be between 1024 and 65535"; exit 1
    fi

    # Update panel.env
    sed -i "s|^PGADMIN_PORT=.*|PGADMIN_PORT=${new_port}|" "$PANEL_ENV"

    # Restart pgAdmin with new port
    info "Updating pgAdmin port to ${new_port}..."
    cd "$INSTALL_DIR"
    docker compose stop koris-pgadmin 2>/dev/null
    docker compose up -d koris-pgadmin

    # Wait up to 30s
    local attempts=0
    while [[ ${attempts} -lt 15 ]]; do
        local state
        state=$(docker inspect -f '{{.State.Status}}' koris-pgadmin 2>/dev/null || echo "")
        [[ "${state}" == "running" ]] && break
        sleep 2
        attempts=$((attempts + 1))
    done

    local ip
    ip=$(curl -fsS4 --max-time 3 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')
    info "pgAdmin is now available at http://${ip}:${new_port}"
}

cmd_reinstall() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }

    local do_clean=""
    for arg in "$@"; do
        case "${arg}" in
            --clean) do_clean="yes" ;;
            *)       error "Unknown flag: ${arg}"; exit 1 ;;
        esac
    done

    # Verify panel.env exists with POSTGRES_PASSWORD
    if [[ ! -f "$PANEL_ENV" ]]; then
        error "Configuration file not found: $PANEL_ENV"
        echo "  Cannot reinstall without existing configuration."
        exit 1
    fi
    local db_pass
    db_pass=$(grep -oP 'POSTGRES_PASSWORD=\K.*' "$PANEL_ENV" 2>/dev/null || true)
    if [[ -z "${db_pass}" ]]; then
        error "POSTGRES_PASSWORD not found in $PANEL_ENV"
        echo "  Cannot reinstall without database password."
        exit 1
    fi

    info "Reinstalling KorisPanel (database data preserved)..."

    # 1. Stop and remove containers
    info "Stopping and removing containers..."
    cd "$INSTALL_DIR" 2>/dev/null || true
    docker compose down --remove-orphans 2>/dev/null || true
    for ctr in koris koris-db koris-pgadmin; do
        docker rm -f "${ctr}" 2>/dev/null || true
    done

    # 2. Remove project images
    info "Removing project images..."
    local images
    images=$(docker images --format '{{.ID}} {{.Repository}}' 2>/dev/null | awk '$2 ~ /^koris/ {print $1}')
    images+=" $(docker images --filter "label=com.docker.compose.project=koris" -q 2>/dev/null)"
    for img in $(echo "${images}" | tr ' ' '\n' | sort -u | grep -v '^$'); do
        docker rmi -f "${img}" 2>/dev/null || true
    done

    # 3. Remove panel-data and pgadmin-data volumes (preserve db-data)
    docker volume rm koris_panel-data koris_pgadmin-data 2>/dev/null || true

    # 4. Prune build cache if --clean
    if [[ "${do_clean}" == "yes" ]]; then
        info "Pruning Docker build cache..."
        docker builder prune -f 2>/dev/null || true
    fi

    # 5. Pull latest source
    info "Pulling latest source from main..."
    if [[ -d "$INSTALL_DIR/.git" ]]; then
        git -C "$INSTALL_DIR" fetch origin main --quiet 2>/dev/null || { error "Git fetch failed"; exit 1; }
        git -C "$INSTALL_DIR" checkout main --quiet 2>/dev/null || true
        git -C "$INSTALL_DIR" reset --hard origin/main --quiet 2>/dev/null || { error "Git pull failed"; exit 1; }
    else
        error "Source directory $INSTALL_DIR is not a git repository"
        exit 1
    fi

    # 6. Rebuild all containers
    info "Building containers..."
    cd "$INSTALL_DIR"
    docker compose build || { error "Docker build failed — database data is intact"; exit 1; }
    docker compose up -d || { error "Docker Compose failed to start services"; exit 1; }

    # 7. Health check: poll every 5s for 60s
    info "Checking health..."
    local attempts=0
    local healthy=""
    local panel_port
    panel_port=$(grep -oP 'PANEL_PORT=\K.*' "$PANEL_ENV" 2>/dev/null || echo "2026")

    while [[ ${attempts} -lt 12 ]]; do
        if curl -fsSk "https://localhost:${panel_port}/api/health" >/dev/null 2>&1; then
            healthy="yes"
            break
        fi
        sleep 5
        attempts=$((attempts + 1))
    done

    if [[ "${healthy}" == "yes" ]]; then
        info "Reinstall complete — panel is healthy ✓"
    else
        error "Health check timed out after 60 seconds"
        docker logs koris --tail 20 2>&1 | sed 's/^/  /'
        exit 1
    fi

    # Update CLI self
    [[ -f "$INSTALL_DIR/panel/koris.sh" ]] && { cp "$INSTALL_DIR/panel/koris.sh" /usr/local/bin/koris 2>/dev/null; chmod +x /usr/local/bin/koris 2>/dev/null; }
}

cmd_downgrade() {
    [[ $EUID -ne 0 ]] && { error "Need root"; exit 1; }

    local target_tag="${1:-}"
    if [[ -z "${target_tag}" ]]; then
        error "Usage: koris downgrade <version-tag>"
        echo "  Example: koris downgrade v1.2.0"
        exit 1
    fi

    # Don't accept flags as the tag
    if [[ "${target_tag}" == --* ]]; then
        error "Usage: koris downgrade <version-tag>"
        echo "  Example: koris downgrade v1.2.0"
        exit 1
    fi

    info "Downgrading to version: ${target_tag}"
    info "This will rebuild the panel at the specified version while preserving database data."
    echo ""

    # Invoke the panel installer with --version and --reinstall
    if [[ -f "$INSTALL_DIR/install.sh" ]]; then
        bash "$INSTALL_DIR/install.sh" --version="${target_tag}" --reinstall
    else
        error "Panel installer not found at $INSTALL_DIR/install.sh"
        exit 1
    fi
}

show_menu() {
    clear
    echo -e "${bold}${blue}KorisPanel${plain} v$(get_version)    Panel: $(panel_status)  Node: $(node_status)"
    echo ""
    echo -e "  ${green}1.${plain}  Start               ${green}10.${plain} Disable autostart"
    echo -e "  ${green}2.${plain}  Stop                ${green}11.${plain} Uninstall"
    echo -e "  ${green}3.${plain}  Restart             ${green}12.${plain} SSL Certificate"
    echo -e "  ${green}4.${plain}  Status              ${green}13.${plain} Clean"
    echo -e "  ${green}5.${plain}  Logs                ${green}14.${plain} DB Management"
    echo -e "  ${green}6.${plain}  Live logs           ${green}15.${plain} pgAdmin Management"
    echo -e "  ${green}7.${plain}  Update              ${green}16.${plain} Reinstall"
    echo -e "  ${green}8.${plain}  Config              ${green}17.${plain} Downgrade"
    echo -e "  ${green}9.${plain}  Enable autostart    ${green}0.${plain}  Exit"
    echo ""
    read -rp "$(echo -e "${cyan}Choose [0-17]: ${plain}")" ch
    case "$ch" in
        1)  cmd_start ;;
        2)  cmd_stop ;;
        3)  cmd_restart ;;
        4)  cmd_status ;;
        5)  cmd_logs ;;
        6)  cmd_follow ;;
        7)  cmd_update ;;
        8)  cmd_config ;;
        9)  docker update --restart unless-stopped koris koris-db koris-pgadmin 2>/dev/null; info "Autostart enabled." ;;
        10) docker update --restart no koris koris-db koris-pgadmin 2>/dev/null; info "Autostart disabled." ;;
        11) cmd_uninstall ;;
        12) cmd_ssl ;;
        13) menu_clean ;;
        14) menu_db ;;
        15) menu_pgadmin ;;
        16) menu_reinstall ;;
        17) menu_downgrade ;;
        0)  exit 0 ;;
        *)  warn "Invalid selection. Enter a number 0-17." ;;
    esac
    echo ""; read -rp "Press Enter to continue..." _; show_menu
}

menu_db() {
    echo ""
    echo -e "${bold}${cyan}Database Management${plain}"
    echo ""
    echo -e "  ${green}1.${plain} Backup"
    echo -e "  ${green}2.${plain} Restore"
    echo -e "  ${green}3.${plain} Migrate"
    echo -e "  ${green}4.${plain} Reset"
    echo -e "  ${green}5.${plain} Shell"
    echo -e "  ${green}6.${plain} Status"
    echo -e "  ${green}0.${plain} Back"
    echo ""
    read -rp "$(echo -e "${cyan}Choose [0-6]: ${plain}")" db_ch
    case "$db_ch" in
        1) cmd_db backup ;;
        2) cmd_db restore ;;
        3) cmd_db migrate ;;
        4) cmd_db reset ;;
        5) cmd_db shell ;;
        6) cmd_db status ;;
        0) return ;;
        *) warn "Invalid selection. Enter a number 0-6." ;;
    esac
}

menu_pgadmin() {
    echo ""
    echo -e "${bold}${cyan}pgAdmin Management${plain}"
    echo ""
    echo -e "  ${green}1.${plain} Status"
    echo -e "  ${green}2.${plain} Enable"
    echo -e "  ${green}3.${plain} Disable"
    echo -e "  ${green}4.${plain} URL"
    echo -e "  ${green}5.${plain} Reset password"
    echo -e "  ${green}6.${plain} Change port"
    echo -e "  ${green}0.${plain} Back"
    echo ""
    read -rp "$(echo -e "${cyan}Choose [0-6]: ${plain}")" pg_ch
    case "$pg_ch" in
        1) cmd_pgadmin status ;;
        2) cmd_pgadmin enable ;;
        3) cmd_pgadmin disable ;;
        4) cmd_pgadmin url ;;
        5) cmd_pgadmin reset-password ;;
        6)
            read -rp "$(echo -e "${cyan}New port number: ${plain}")" new_port
            cmd_pgadmin port "${new_port}"
            ;;
        0) return ;;
        *) warn "Invalid selection. Enter a number 0-6." ;;
    esac
}

menu_clean() {
    echo ""
    echo -e "${bold}${cyan}Clean Docker Artifacts${plain}"
    echo ""
    echo -e "  ${green}1.${plain} Basic clean (remove images and build cache)"
    echo -e "  ${green}2.${plain} Clean with volumes (remove images, cache, panel+pgadmin volumes)"
    echo -e "  ${green}3.${plain} Full clean (remove everything including database volume)"
    echo -e "  ${green}0.${plain} Cancel"
    echo ""
    read -rp "$(echo -e "${cyan}Choose [0-3]: ${plain}")" clean_ch
    case "$clean_ch" in
        1) cmd_clean ;;
        2) cmd_clean --volumes ;;
        3) cmd_clean --all ;;
        0) return ;;
        *) warn "Invalid selection. Enter a number 0-3." ;;
    esac
}

menu_reinstall() {
    echo ""
    echo -e "${yellow}This will stop all containers, remove images, and rebuild from source.${plain}"
    echo -e "${yellow}Database data will be preserved.${plain}"
    echo ""
    read -rp "$(echo -e "${cyan}Proceed with reinstall? [y/N]: ${plain}")" confirm
    if [[ "${confirm}" =~ ^[yY] ]]; then
        cmd_reinstall
    else
        info "Cancelled."
    fi
}

menu_downgrade() {
    echo ""
    read -rp "$(echo -e "${cyan}Target version tag (e.g. v1.2.0): ${plain}")" target_ver
    if [[ -z "${target_ver}" ]]; then
        warn "No version specified."
        return
    fi
    echo ""
    echo -e "${yellow}This will rebuild the panel at version ${target_ver}.${plain}"
    read -rp "$(echo -e "${cyan}Confirm downgrade? [y/N]: ${plain}")" confirm
    if [[ "${confirm}" =~ ^[yY] ]]; then
        cmd_downgrade "${target_ver}"
    else
        info "Cancelled."
    fi
}

case "${1:-}" in
    start)     cmd_start;; stop) cmd_stop;; restart) cmd_restart;;
    status)    cmd_status;; logs) cmd_logs;; follow|logs-live) cmd_follow;;
    update)    shift; cmd_update "$@";; config) cmd_config;; uninstall) shift; cmd_uninstall "$@";;
    ssl)       cmd_ssl;; clean) shift; cmd_clean "$@";;
    db)        shift; cmd_db "$@";;
    pgadmin)   shift; cmd_pgadmin "$@";;
    reinstall) shift; cmd_reinstall "$@";;
    downgrade) shift; cmd_downgrade "$@";;
    enable)    docker update --restart unless-stopped koris koris-db koris-pgadmin 2>/dev/null; info "Autostart enabled.";;
    disable)   docker update --restart no koris koris-db koris-pgadmin 2>/dev/null; info "Autostart disabled.";;
    node-status)  echo "Node Agent: $(node_status)";;
    node-restart) docker restart knode 2>/dev/null && info "Node restarted." || error "Failed to restart knode.";;
    node-logs)    docker logs knode --tail 50 2>/dev/null || error "knode container not found.";;
    help|-h|--help) echo "Usage: koris [start|stop|restart|status|logs|follow|update|config|ssl|uninstall|reinstall|downgrade|clean|db|pgadmin|enable|disable|node-status|node-restart|node-logs]"; echo "Run without args for interactive menu.";;
    "") show_menu;;
    *) error "Unknown: $1. Run 'koris help'."; exit 1;;
esac
