# KorisPanel

Multi-protocol, multi-node VPN management panel with customer billing, real-time monitoring, and reseller system.

## Features

- **Multi-Protocol VPN** — OpenVPN, L2TP/IPSec, IKEv2, SSH Tunnel
- **Multi-Node** — Manage unlimited VPN nodes from a single panel
- **Customer Management** — User accounts with data/speed limits, expiry, status control
- **Subscription Plans** — Create plans with data caps, speed limits, duration, pricing
- **Wallet & Billing** — Per-user wallet, manual/automated payments, payment methods
- **Reseller System** — Sub-accounts with credit allocation and customer provisioning
- **Real-Time Monitoring** — Live bandwidth charts, session tracking, node health metrics
- **Ticket System** — Customer support tickets with admin/user messaging
- **Telegram Bot** — Remote panel management via Telegram commands
- **FreeRADIUS Integration** — Standards-based AAA with session accounting
- **Admin Dashboard** — Analytics, usage monitor, live sessions, audit logs

## Quick Install

### Panel (one-liner)

```bash
bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/install.sh)
```

### Node Agent (on each VPN server)

```bash
bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/node-install.sh)
```

You'll need the **Panel URL** and **Node Token** (generated in panel admin under Services > Nodes > + New Node).

## Management

After installation, use the `koris` command:

```bash
koris              # Interactive menu
koris status       # Service status + system info
koris restart      # Restart panel & node
koris update       # Pull latest & rebuild
koris logs         # Recent logs
koris follow       # Live log stream
koris uninstall    # Remove everything
```

## Requirements

### Panel Server

| Component | Minimum |
|-----------|---------|
| OS | Ubuntu 20.04+ / Debian 11+ / CentOS 8+ |
| RAM | 1 GB |
| Disk | 10 GB |
| Go | 1.22+ (installed automatically) |
| MariaDB | 10.5+ (installed automatically) |
| FreeRADIUS | 3.x (installed automatically) |
| Nginx | Any (installed automatically) |
| Node.js | 18+ (for frontend build, installed automatically) |

### Node Server

| Component | Minimum |
|-----------|---------|
| OS | Ubuntu 20.04+ / Debian 11+ |
| RAM | 512 MB |
| Outbound | Must reach panel URL |
| Packages | OpenVPN, StrongSwan, xl2tpd (installed automatically) |

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   Panel Server                       │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────┐ │
│  │  Nginx   │→ │  Panel   │→ │  MariaDB         │ │
│  │  :80/443 │  │  :8080   │  │  (radius DB)     │ │
│  └──────────┘  └──────────┘  └──────────────────┘ │
│                      │                              │
│                      ↓                              │
│              ┌──────────────┐                       │
│              │ FreeRADIUS   │                       │
│              │ (auth/acct)  │                       │
│              └──────────────┘                       │
└─────────────────────────────────────────────────────┘
         │ WebSocket + REST API
         ↓
┌─────────────────────────────────────────────────────┐
│                Node Server(s)                        │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────┐ │
│  │  Node    │  │ OpenVPN  │  │ StrongSwan       │ │
│  │  Agent   │→ │ :1194    │  │ IKEv2/L2TP       │ │
│  └──────────┘  └──────────┘  └──────────────────┘ │
└─────────────────────────────────────────────────────┘
```

## Admin Panel

Access at `http://YOUR_IP/dashboard/` after installation.

**Sidebar navigation:**
- **Dashboard** — Stats overview, usage monitor (day/week/month), recent users
- **Analytics** — Revenue, bandwidth charts, user distribution, live sessions
- **Transactions** — Payment recording, approval queue, payment methods management
- **Users** — Accounts (with status filters), Tickets, Resellers
- **Services** — Nodes (health metrics), Cores (per-protocol config cards)
- **Plans** — Subscription plan CRUD
- **Settings** — Panel status, VPN settings, Telegram bot, certificates, audit logs, backups

## API

All admin operations are available via REST API at `/api/`. Key endpoints:

| Endpoint | Description |
|----------|-------------|
| `GET /api/health` | Panel health check |
| `GET /api/dashboard/stats` | Dashboard statistics |
| `GET /api/customers` | List customers |
| `POST /api/customers` | Create customer |
| `GET /api/nodes` | List nodes |
| `POST /api/nodes` | Register node |
| `GET /api/payments` | List payments |
| `WS /api/realtime` | WebSocket for live stats/sessions |

## Configuration

Panel config: `/etc/panel/panel.env`
Node config: `/etc/panel-node/node.env`

Key panel environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `PANEL_ADDR` | Listen address | `127.0.0.1:8080` |
| `PANEL_DB_DSN` | MariaDB connection string | — |
| `PANEL_SETUP_KEY` | Initial admin setup key | auto-generated |
| `PANEL_SESSION_SECRET` | Cookie signing secret | auto-generated |
| `PANEL_VERSION` | Display version | from `VERSION` file |

## Development

```bash
# Clone
git clone https://github.com/anonysec/panel.git
cd panel

# Backend
go run ./panel/cmd/panel

# Frontend (admin)
cd panel/web/admin
npm install
npm run dev

# Frontend (portal)
cd panel/web/portal
npm install
npm run dev
```

## Updating

On your server:
```bash
koris update
```

Or manually:
```bash
cd /opt/koris-next
git pull origin main
bash deploy.sh
```

## License

Private repository. All rights reserved.
