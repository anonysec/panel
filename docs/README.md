# KorisPanel Installation Guide

## Requirements

- **OS**: Ubuntu 22.04+ or Debian 12+ (x86_64)
- **Docker**: 24+ with Docker Compose v2
- **Git**: 2.x

No other dependencies are required — the panel, database, and frontend are all built and run inside Docker containers.

## Installation

### Quick Install (recommended)

```bash
bash <(curl -Ls https://raw.githubusercontent.com/anonysec/KorisPanel/main/install.sh)
```

The installer accepts the following flags:

| Flag | Description |
|------|-------------|
| `--lite` | Install lite edition (excludes billing, SLA, teleproxy) |
| `--full` | Install full edition (default) |
| `--port=N` | Set panel HTTPS port (default: 2026) |
| `--domain=X` | Set public domain for auto-TLS |
| `--no-knode` | Skip knode installation on same host |
| `--uninstall` | Uninstall the panel |
| `--version=<tag>` | Install a specific version tag |
| `--reinstall` | Reinstall preserving database data |

### Manual Installation

```bash
git clone https://github.com/anonysec/KorisPanel.git /opt/KorisPanel
cd /opt/KorisPanel
docker compose build && docker compose up -d
```

### Configuration

Panel configuration is stored in `/etc/koris/`:

| File | Description |
|------|-------------|
| `panel.env` | Environment variables for Docker Compose |
| `version` | Currently installed version tag (written automatically on install/update) |

**Environment variables** in `panel.env`:

| Variable | Description |
|----------|-------------|
| `POSTGRES_PASSWORD` | PostgreSQL password (auto-generated) |
| `PANEL_PORT` | Panel HTTPS port (default: `2026`) |
| `PANEL_PORT` | Panel HTTPS port (default: `2026`) |
| `PANEL_DOMAIN` | Public domain for auto-TLS (Let's Encrypt) |
| `PANEL_EDITION` | `full` or `lite` |
| `TELEGRAM_BOT_TOKEN` | (Optional) Telegram bot token for notifications |

## Docker Stack

The panel runs as a Docker Compose stack with three services:

| Service | Image | Port | Description |
|---------|-------|------|-------------|
| `koris` | Custom (multi-stage build) | 2026 (HTTPS), 80 (HTTP) | Panel app (Go + embedded frontend) |
| `koris-db` | `timescale/timescaledb:latest-pg16` | 5432 (internal) | PostgreSQL 16 + TimescaleDB |
| `koris-pgadmin` | pgAdmin 4 | 5050 | Database admin UI |

No Nginx or reverse proxy — the panel serves TLS directly with automatic Let's Encrypt certificate management.

## Post-Install

1. **Access the panel** at `https://your-server:2026`
2. **Run the setup wizard** — create the owner account on first access at `/dashboard/`
3. The wizard will prompt for:
   - Admin username and password
   - Basic VPN settings (protocol, subnet)
   - Telegram bot configuration (optional)

## CLI Management

After installation, use the `koris` CLI:

```bash
koris                # Launch interactive menu (numbered options with submenus)
koris start          # Start all services
koris stop           # Stop all services
koris restart        # Restart all services
koris status         # Show service status
koris logs           # View panel logs
koris update         # Update to latest version
koris downgrade v1.x # Downgrade to a specific version
koris reinstall      # Rebuild from source (preserves DB)
koris db backup      # Backup database
koris db restore     # Restore database
koris pgadmin status # Manage pgAdmin service
koris clean          # Remove unused images and build cache
koris uninstall      # Full uninstall
```

Running `koris` without arguments opens an interactive menu with submenus for DB management, pgAdmin, clean operations, reinstall, and downgrade.

## knode (VPN Node Agent)

Install knode on each VPN node server:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/anonysec/knode/master/install.sh)
```

Flags:

| Flag | Description |
|------|-------------|
| `--port=N` | Set API listen port |
| `--name=NAME` | Set instance name (for multi-instance) |

knode runs as a standalone Docker container with host networking:

```bash
docker logs -f knode       # View logs
docker restart knode       # Restart
docker stop knode          # Stop
```

Configuration: `/etc/knode/config.toml`

## Updating

```bash
koris update                    # Update to latest
koris update --version=v1.2.3   # Update to specific version
```

## Memory Tuning

KorisPanel is optimized for 1GB RAM servers. See [low-memory-tuning.md](../panel/docs/low-memory-tuning.md) for details.

Runtime defaults: `GOMAXPROCS=1`, `GOGC=50`, `GOMEMLIMIT=100MB`.
