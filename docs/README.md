# KorisPanel Installation Guide

## Requirements

- **OS**: Ubuntu 22.04+ (x86_64)
- **Go**: 1.22+
- **Node.js**: 20+ (with npm)
- **MariaDB**: 10.11+
- **FreeRADIUS**: 3.x (with `rlm_sql_mysql` module)

## Installation

### 1. Clone the repository

```bash
git clone git@github.com:your-org/koris-panel.git /opt/koris
cd /opt/koris
```

### 2. Configure environment

Copy the example `.env` file and edit it with your database credentials and settings:

```bash
cp .env.example .env
nano .env
```

Key variables:

| Variable | Description |
|----------|-------------|
| `DB_HOST` | MariaDB host (default: `127.0.0.1`) |
| `DB_PORT` | MariaDB port (default: `3306`) |
| `DB_NAME` | Database name |
| `DB_USER` | Database user |
| `DB_PASS` | Database password |
| `PANEL_PORT` | Panel HTTP port (default: `8080`) |
| `PANEL_DOMAIN` | Public domain for the panel |
| `TELEGRAM_BOT_TOKEN` | (Optional) Telegram bot token for notifications |

### 3. Run the installer

```bash
chmod +x install.sh
sudo ./install.sh
```

The installer will:
- Install system dependencies (MariaDB, FreeRADIUS, etc.)
- Create the database and apply migrations
- Build the Go binary
- Build the admin and customer web interfaces
- Configure systemd services
- Set up FreeRADIUS SQL integration

### 4. Build manually (development)

```bash
# Build backend
go build -o /usr/local/bin/panel ./panel/cmd/panel

# Build admin frontend
cd panel/web/admin && npm install && npm run build

# Build customer portal frontend
cd panel/web/portal && npm install && npm run build
```

## Post-Install

1. **Access the panel** at `http://your-server:8080`
2. **Run the setup wizard** — create the owner account on first access at `/dashboard/`
3. The wizard will prompt for:
   - Admin username and password
   - Basic VPN settings (protocol, subnet)
   - Telegram bot configuration (optional)

## Systemd Services

```bash
# Panel service
sudo systemctl status koris-panel
sudo systemctl restart koris-panel

# Node agent (on VPN nodes)
sudo systemctl status koris-node-agent
```

## Updating

```bash
cd /opt/koris
git pull
./deploy.sh
```

## Memory Tuning

KorisPanel is optimized for 1GB RAM servers. See [low-memory-tuning.md](../panel/docs/low-memory-tuning.md) for details.

Runtime defaults: `GOMAXPROCS=1`, `GOGC=50`, `GOMEMLIMIT=100MB`.
