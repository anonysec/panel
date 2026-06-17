# KorisPanel

**Multi-protocol, multi-node VPN management panel** with customer billing, real-time monitoring, reseller system, and modern web UI.

Manage your entire VPN infrastructure from a single dashboard: nodes, customers, subscriptions, payments, support tickets, and more.

---

## Key Features

### VPN & Networking
- **Multi-Protocol** - OpenVPN, L2TP/IPSec, IKEv2, SSH Tunnel
- **Multi-Node** - Manage unlimited VPN nodes from one panel
- **Upstream Proxy** - Route node traffic through xray, SOCKS5, or HTTP proxy
- **FreeRADIUS Integration** - Standards-based AAA with session accounting
- **Real-Time Monitoring** - Live bandwidth charts, session tracking, node health

### Business & Billing
- **Subscription Plans** - Quota-based or pay-as-you-go pricing
- **Wallet & Payments** - Per-user wallet, manual/crypto payments, payment methods
- **Reseller System** - Sub-accounts with credit allocation and customer provisioning

### Customer Experience
- **Self-Service Portal** - Simple single-page UI for customers (usage, profiles, support)
- **Download Apps** - Configurable app download links for iOS, Android, Windows, macOS
- **Ticket System** - Customer support with admin/user messaging
- **Telegram Bot** - Both admin management and customer self-service via inline buttons

### Admin Panel
- **Dashboard** - Stats overview, usage graphs, recent activity
- **Customer Management** - Accounts with data/speed limits, expiry, bulk actions
- **Theming** - 5 built-in themes (Midnight, Kiro, GitHub, Soft Dark, Corporate)
- **Dark/Light/System Mode** - Auto-detects OS preference
- **Multi-Language** - English, Persian (RTL), Chinese, Russian
- **Timezone Support** - Dates formatted per user locale and browser timezone
- **Templates** - Pre-configured customer profiles for quick provisioning

## Screenshots

> Screenshots will be added after initial release.

---

## Quick Install

### Panel Server (one-liner)

```bash
bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/install.sh)
```

### Node Agent (on each VPN server)

```bash
bash <(curl -Ls https://raw.githubusercontent.com/anonysec/panel/main/node-install.sh)
```

You will need the **Panel URL** and **Node Token** (generated in admin panel under Services > Nodes > Add Node).

---

## Management CLI

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

---

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
| Network | Must reach panel URL |
| Packages | OpenVPN, StrongSwan, xl2tpd (installed automatically) |

---

## Architecture

```
                    Panel Server
 +-------------------------------------------------+
 |  Nginx (:80/443)                                |
 |      |                                          |
 |      v                                          |
 |  Panel Backend (:8080)  -->  MariaDB            |
 |      |                       (radius DB)        |
 |      v                                          |
 |  FreeRADIUS (auth/acct)                         |
 +-------------------------------------------------+
          |  WebSocket + REST API
          v
 +-------------------------------------------------+
 |              Node Server(s)                      |
 |                                                  |
 |  Node Agent --> OpenVPN (:1194)                  |
 |             --> StrongSwan (IKEv2/L2TP)          |
 |             --> SSH Tunnel                       |
 |             --> [Upstream Proxy] (optional)      |
 +-------------------------------------------------+
```

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.22+, Chi router, GORM |
| Admin Frontend | Vue 3, TypeScript, Vite |
| Portal Frontend | Vue 3, TypeScript, Vite |
| Database | MariaDB 10.5+ |
| AAA | FreeRADIUS 3.x |
| VPN Protocols | OpenVPN, StrongSwan, xl2tpd |
| Reverse Proxy | Nginx |
| Bot | Telegram Bot API (inline keyboards) |

---

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

---

## Configuration

Panel config: `/etc/panel/panel.env`
Node config: `/etc/panel-node/node.env`

| Variable | Description | Default |
|----------|-------------|---------|
| `PANEL_ADDR` | Listen address | `127.0.0.1:8080` |
| `PANEL_DB_DSN` | MariaDB connection string | - |
| `PANEL_SETUP_KEY` | Initial admin setup key | auto-generated |
| `PANEL_SESSION_SECRET` | Cookie signing secret | auto-generated |
| `PANEL_VERSION` | Display version | from `VERSION` file |

---

## Development

```bash
# Clone
git clone https://github.com/anonysec/panel.git
cd panel

# Backend
go run ./panel/cmd/panel

# Frontend (admin)
cd panel/web/admin
npm install && npm run dev

# Frontend (portal)
cd panel/web/portal
npm install && npm run dev
```

---

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

---

## License

Private repository. All rights reserved.
