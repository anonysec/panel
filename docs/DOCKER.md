# Docker Deployment Guide

This guide covers deploying KorisPanel using Docker and Docker Compose.

## Quick Start

```bash
# One-liner install (interactive prompts for edition, domain, port, SSL)
bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/install.sh)

# Or with flags
bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/install.sh) \
  --full --domain=panel.example.com --port=2026
```

The installer handles everything: cloning the source, writing configuration, building Docker images, and starting services. Migrations run automatically on first startup.

If an existing installation is detected (panel or knode), the installer presents an interactive menu:
1. **Reinstall** — rebuild from source while preserving database data
2. **Full wipe & fresh install** — remove all data and start from scratch (requires "yes" confirmation)
3. **Update** — pull latest from `main` and rebuild in place
4. **Cancel** — exit without changes

This detection checks for `/etc/koris/panel.env`, `/etc/knode/config.toml`, and running `koris`/`knode` containers. To bypass this interactive prompt (e.g., from scripts), use the `--reinstall` flag directly.

## Installer Options

```bash
install.sh --lite              # Lite edition
install.sh --full              # Full edition (default)
install.sh --port=8080         # Custom HTTPS port (default: 2026)
install.sh --domain=panel.example.com  # Domain for Let's Encrypt
install.sh --no-knode          # Skip knode agent installation prompt
install.sh --version=v1.2.0    # Install a specific version tag
install.sh --reinstall         # Force reinstall (preserves DB data)
install.sh --uninstall         # Remove KorisPanel completely
```

Passing `--native` exits with a deprecation error — only Docker deployment is supported.

## Architecture

```
┌─────────────────────────────────────────────────┐
│  koris (port 2026 HTTPS, port 80 HTTP)          │
│  Go binary + embedded frontend + migrations     │
│  Serves TLS directly (no reverse proxy)         │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│  koris-db (port 5432 internal)                  │
│  timescale/timescaledb:latest-pg16              │
│  Persistent volume: koris_db-data               │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│  koris-pgadmin (port 5050)                      │
│  dpage/pgadmin4                                 │
│  Persistent volume: koris_pgadmin-data          │
└─────────────────────────────────────────────────┘
```

No Nginx. No MariaDB. The panel terminates TLS directly using Let's Encrypt (ACME), manual certificates, or self-signed mode.

### Volumes

| Volume | Purpose |
|--------|---------|
| `koris_db-data` | PostgreSQL/TimescaleDB data directory |
| `koris_panel-data` | Panel application data |
| `koris_pgadmin-data` | pgAdmin session and settings data |

### Health Checks

- **koris**: Container health check polls the panel's `/api/health` endpoint
- **koris-db**: PostgreSQL `pg_isready` check every 5s
- The panel service waits for the DB health check to pass before starting

## Configuration

All configuration lives in `/etc/koris/panel.env` (symlinked to `docker/panel.env` in the source tree).

### Core Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PANEL_PG_DSN` | PostgreSQL connection string | `postgres://koris:...@db:5432/koris` |
| `POSTGRES_DB` | Database name | `koris` |
| `POSTGRES_USER` | Database user | `koris` |
| `POSTGRES_PASSWORD` | Database password | Auto-generated |
| `PANEL_ADDR` | Listen address | `0.0.0.0:2026` |
| `PANEL_PORT` | HTTPS port | `2026` |
| `PANEL_TLS_MODE` | TLS mode (acme/manual/selfsigned) | `selfsigned` |
| `PANEL_DOMAIN` | Domain for ACME | — |
| `PANEL_SESSION_SECRET` | Session signing key | Auto-generated |
| `PANEL_SETUP_KEY` | First-login admin setup key | Auto-generated |
| `PANEL_MIGRATIONS` | Path to migration files | `/app/migrations` |
| `BUILD_TAGS` | Edition (full/lite) | `full` |

### pgAdmin Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PGADMIN_EMAIL` | `admin@koris.local` | pgAdmin login email |
| `PGADMIN_PASSWORD` | Auto-generated | pgAdmin login password |
| `PGADMIN_PORT` | `5050` | pgAdmin listen port |

### Optional Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PANEL_WORKERS` | `1` | Number of worker processes |
| `PANEL_GRACEFUL_WAIT` | `30` | Seconds for graceful shutdown |
| `PANEL_DB_MAX_OPEN` | Auto-tuned | Max open DB connections |
| `PANEL_DB_MAX_IDLE` | Auto-tuned | Max idle DB connections |
| `PANEL_DB_MAX_LIFETIME` | `5m` | Max connection lifetime |
| `TELEGRAM_BOT_TOKEN` | — | Telegram bot token |
| `TELEGRAM_CHAT_ID` | — | Telegram chat ID for alerts |

## Manual Deployment

If you prefer to deploy without the installer:

```bash
# 1. Clone the repository
git clone https://github.com/anonysec/panel.git /opt/KorisPanel
cd /opt/KorisPanel

# 2. Create configuration
mkdir -p /etc/koris
cp docker/panel.env.example /etc/koris/panel.env
ln -sf /etc/koris/panel.env docker/panel.env

# 3. Edit configuration (set passwords, domain, port)
nano /etc/koris/panel.env

# 4. Build and start
docker compose build
docker compose up -d

# 5. Verify health
docker inspect --format='{{.State.Health.Status}}' koris
```

## Scaling

### Worker Processes

Scale within a single container by setting `PANEL_WORKERS`:

```env
PANEL_WORKERS=4
```

Each worker shares the same port via `SO_REUSEPORT`. Only one worker holds the background task leader lock.

### Scaling Guidelines

| RAM | `PANEL_WORKERS` | `PANEL_DB_MAX_OPEN` |
|-----|----------------|---------------------|
| 1 GB | 1 | 10 |
| 2 GB | 2 | 25 |
| 4 GB | 4 | 50 |
| 8 GB+ | 4 | 50 |

## Reinstall & Uninstall

### Reinstall (preserve database)

```bash
# Via CLI (recommended)
koris reinstall              # Rebuild from latest main, preserve DB data
koris reinstall --clean      # Same, but prune Docker build cache before rebuilding

# Via installer
install.sh --reinstall
```

The `koris reinstall` command:
1. Verifies `/etc/koris/panel.env` exists with `POSTGRES_PASSWORD` — aborts if missing
2. Stops and removes all Compose stack containers and project images
3. Removes `koris_panel-data` and `koris_pgadmin-data` volumes (preserves `koris_db-data`)
4. Optionally prunes Docker build cache (`--clean`)
5. Pulls latest source from the `main` branch
6. Rebuilds all containers with `docker compose build`
7. Runs a health check — polls `/api/health` every 5s for up to 60s
8. On failure, displays the last 20 lines of container logs and exits

Reinstall preserves `koris_db-data` and reuses the existing `/etc/koris/panel.env`.

### Uninstall

```bash
# Via installer
install.sh --uninstall

# Manually
cd /opt/KorisPanel
docker compose down -v --remove-orphans
docker images --filter "label=com.docker.compose.project=koris" -q | xargs -r docker rmi -f
rm -rf /opt/KorisPanel /etc/koris /usr/local/bin/koris
```

## Database Management

The `koris db` command provides full database lifecycle management:

```bash
koris db backup                # Backup to /var/backups/koris/
koris db backup --path=/mnt/backups  # Backup to custom directory
koris db restore <file>        # Restore from a specific backup file
koris db restore               # List available backups and select interactively
koris db migrate               # Run pending database migrations
koris db reset                 # Drop and recreate DB, run all migrations (prompts for confirmation)
koris db shell                 # Open interactive psql session in the DB container
koris db status                # Show DB size, connections, TimescaleDB version, replication status
```

| Subcommand | Description |
|------------|-------------|
| `backup` | Gzipped `pg_dump` saved as `koris-<YYYYMMDD-HHMMSS>.sql.gz` |
| `backup --path=<dir>` | Save to a specific directory (must exist and be writable) |
| `restore <file>` | Validate file, drop+recreate DB, restore dump (confirms before overwriting) |
| `restore` | List backups sorted by date, prompt for selection |
| `migrate` | Run pending migrations inside the panel container, display count applied |
| `reset` | Drop+recreate DB and run all migrations from scratch (requires "yes" confirmation) |
| `shell` | Interactive `psql -U koris -d koris` inside `koris-db` container |
| `status` | Database size, active connections, TimescaleDB version, replication info |

All `db` subcommands require the `koris-db` container to be running — they exit with an error if it is not.

### Backup Details

Backups are gzipped SQL dumps named `koris-<YYYYMMDD-HHMMSS>.sql.gz`. Restore drops and recreates the database, then applies the dump. A confirmation prompt is shown before overwriting.

### Manual (Docker commands)

```bash
# Backup to gzipped SQL dump
docker exec koris-db pg_dump -U koris -d koris | gzip > backup_$(date +%Y%m%d).sql.gz

# Restore
gunzip -c backup_20240101.sql.gz | docker exec -i koris-db psql -U koris -d koris
```

### Volume Backup

```bash
# Stop services for consistency
docker compose stop

# Backup DB volume
docker run --rm -v koris_db-data:/data -v $(pwd):/backup alpine \
    tar czf /backup/db-data-$(date +%Y%m%d).tar.gz -C /data .

# Restart services
docker compose start
```

## Interactive Menu

Running `koris` without arguments launches an interactive menu:

```bash
koris
```

The main menu offers numbered options (0–17) for all operations: start, stop, restart, status, logs, live logs, update, config, enable/disable autostart, uninstall, SSL, clean, DB management, pgAdmin management, reinstall, and downgrade.

Submenus provide guided access to complex operations:

| Submenu | Options |
|---------|---------|
| **DB Management** (14) | backup, restore, migrate, reset, shell, status |
| **pgAdmin Management** (15) | status, enable, disable, URL, reset password, change port |
| **Clean** (13) | basic clean, clean with volumes, full clean |
| **Reinstall** (16) | Prompts for confirmation before executing |
| **Downgrade** (17) | Prompts for target version, then confirms |

Invalid input (non-numeric or out-of-range) displays an error and re-shows the menu.

## Troubleshooting

### Port Conflicts

```bash
# Find what's using the port
sudo ss -tlnp | grep :2026

# Change port in /etc/koris/panel.env:
# PANEL_PORT=8443
# PANEL_ADDR=0.0.0.0:8443
docker compose up -d
```

### Database Connection Failures

```bash
# Check if DB is healthy
docker compose ps koris-db
docker compose logs koris-db --tail=20

# Restart panel after DB is ready
docker compose restart koris
```

### Panel Not Starting

```bash
# Check exit code and logs
docker compose ps -a
docker compose logs koris --tail=50

# Rebuild from scratch
docker compose down
docker compose build --no-cache
docker compose up -d
```

### Viewing Logs

```bash
# All services
docker compose logs -f

# Panel only
docker compose logs -f koris

# Database
docker compose logs -f koris-db

# Last 50 lines
docker compose logs --tail=50 koris
```

### Migrations Failing

```bash
# Check migration logs
docker compose logs koris | grep -i migrat

# Connect to DB directly
docker exec -it koris-db psql -U koris -d koris

# If stuck, reset (WARNING: data loss)
docker exec -it koris-db psql -U koris -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
docker compose restart koris
```

## Clean

Remove unused Docker images and build cache to reclaim disk space:

```bash
koris clean                   # Remove project images + prune build cache
koris clean --volumes         # Also remove panel-data and pgadmin-data volumes (preserves DB)
koris clean --volumes --include-db  # Also remove the database volume
koris clean --all             # Remove everything (all volumes, images, build cache)
koris clean --all --force     # Same as --all but skip confirmation prompt
```

| Flag | Effect |
|------|--------|
| *(none)* | Remove images with `com.docker.compose.project=koris` label or `koris` prefix, prune build cache |
| `--volumes` | Additionally remove `koris_panel-data` and `koris_pgadmin-data` volumes |
| `--include-db` | When combined with `--volumes`, also remove `koris_db-data` |
| `--all` | Remove all project volumes, images, and build cache (prompts for confirmation) |
| `--force` | Skip confirmation prompts |

Volumes that are currently in use by a running container are skipped with a warning. The command displays total disk space reclaimed on completion.

## pgAdmin Management

Manage the pgAdmin service from the CLI:

```bash
koris pgadmin status            # Show running state, URL, and port
koris pgadmin enable            # Start pgAdmin service (waits up to 30s)
koris pgadmin disable           # Stop pgAdmin and disable autostart
koris pgadmin url               # Print access URL (error if not running)
koris pgadmin reset-password    # Set new password (min 8 chars), restarts service
koris pgadmin port <number>     # Change listen port (1024–65535), restarts service
```

| Subcommand | Description |
|------------|-------------|
| `status` | Displays running/stopped state; if running, shows URL and port |
| `enable` | Starts the container, sets restart policy to `unless-stopped` |
| `disable` | Stops the container, sets restart policy to `no` |
| `url` | Prints `http://<server-ip>:<port>` if running |
| `reset-password` | Prompts for a new password, updates `panel.env`, restarts pgAdmin |
| `port <N>` | Validates port (1024–65535), updates `panel.env`, restarts pgAdmin |

## Upgrading

```bash
cd /opt/KorisPanel
git pull origin main
docker compose build
docker compose up -d

# Verify
docker compose ps
docker compose logs --tail=20 koris
```

Or use the CLI tool:

```bash
koris update                    # Pull latest and rebuild
koris update --version=v1.3.0   # Update to specific version
```

The `update` command:
1. Stores the current version to `/etc/koris/version` before pulling (for rollback reference)
2. Fetches the latest code (or checks out the specified tag)
3. Skips rebuild if already at the target version
4. Rebuilds with `docker compose up -d --build`
5. Displays a changelog (up to 50 commits) between old and new versions
6. Runs a health check — polls `/api/health` every 2s for up to 60s
7. On success, writes the new version to `/etc/koris/version`
8. On health check failure, displays the last 20 lines of container logs and suggests `koris downgrade <previous-version>`

## Version Pinning

Install or update to a specific git tag:

```bash
# Fresh install at a specific version
install.sh --version=v1.2.0

# Update to a specific version
koris update --version=v1.2.0

# Downgrade
koris downgrade v1.1.0
```

The installed version is recorded in `/etc/koris/version`.
