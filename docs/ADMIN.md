# KorisPanel Admin Guide

## Accessing the Admin Panel

Navigate to `http://your-server:8080/dashboard/` and log in with your admin credentials.

The admin panel supports multiple languages: English, Farsi, Chinese, and Russian. Switch language from the sidebar footer.

---

## Dashboard Overview

The dashboard shows a summary of your VPN service:

- **Total customers** — active, disabled, expired, limited counts
- **Revenue** — today, this month, and total
- **Active sessions** — currently connected users (real-time via WebSocket)
- **Node status** — online/offline/stale node indicators
- **Bandwidth** — live bandwidth graph across all nodes

The data refreshes automatically via WebSocket connection.

---

## Managing Customers

### Customer List

The Customers view shows all users with status, plan, balance, and creation date. Use the tabs:

- **Customers** — active user list with status filters (All, Active, Online, Limited, Disabled, Expired)
- **Archived** — soft-deleted customers
- **Resellers** — reseller accounts

### Create Customer

Click **+ New User** and fill in:
- Username (used for VPN login)
- Display name
- Password
- Plan assignment
- Optional: set expiry date, data limit, connection limit

### Edit Customer

Click any customer row to open their detail view:
- Update profile information
- Change plan or status
- View usage statistics
- Reset password or traffic
- Renew subscription
- View payment and ticket history

### Bulk Actions

Select multiple customers using checkboxes, then use the bulk toolbar:
- **Enable** — activate selected accounts
- **Disable** — suspend selected accounts
- **Traffic Reset** — reset bandwidth counters
- **Delete** — archive selected accounts

### Customer Statuses

| Status | Description |
|--------|-------------|
| `active` | Account is working, can connect |
| `disabled` | Manually suspended by admin |
| `expired` | Subscription end date passed |
| `limited` | Data limit reached |
| `deleted` | Archived (soft-deleted) |

---

## Managing Nodes

Nodes are the VPN servers that customers connect to.

### Add a Node

1. Go to **Services** (nodes) in the sidebar
2. Click **+ Add Node**
3. Enter:
   - Name (display label)
   - Host/IP address
   - Location (country)
   - Protocols supported
4. Save — the panel generates an auth token

### Node Agent Setup

On the VPN node server, run the node installer:

```bash
curl -sSL http://panel-host:8080/api/node/agent/download -o /usr/local/bin/koris-node
chmod +x /usr/local/bin/koris-node
```

Configure `/etc/koris-node/config.json` with the panel URL and auth token, then start the agent service.

### Configure Protocols

Each node can have individual protocol configurations:
- OpenVPN (UDP/TCP)
- L2TP/IPsec
- IKEv2
- SSH Tunnel
- WireGuard

Configure via the per-node VPN config page.

### Enable/Disable Node

Toggle a node's status to control whether customers can connect to it. Disabled nodes are hidden from the customer portal.

### Node Statuses

| Status | Description |
|--------|-------------|
| `online` | Agent is reporting, node is healthy |
| `offline` | Agent has not reported recently |
| `stale` | Agent missed multiple heartbeats |
| `disabled` | Manually disabled by admin |

---

## VPN Configuration (Cores Tab)

The **VPN Settings** page controls global VPN parameters:

- **OpenVPN**: Server subnet, port, protocol, cipher, DNS push, TLS settings
- **L2TP**: PSK, authentication settings
- **IKEv2**: Certificate selection, identity settings
- **WireGuard**: Interface config, peer management

Changes here affect the generated client profiles. Restart nodes after significant changes using Node Tasks.

---

## Settings

### Telegram Integration

Configure a Telegram bot for:
- New customer notifications
- Payment alerts
- Node offline warnings
- Subscription expiry reminders

Set the bot token and chat ID in **Settings > Telegram**.

### Certificates

Upload TLS/SSL certificates for:
- IKEv2 VPN authentication
- Panel HTTPS (if configured)
- OpenVPN TLS-auth keys

Manage via **Settings > Certificates**. The panel supports automatic certificate rotation.

### Promo Codes

Create promotional discount codes:
- **Percent** type — percentage discount on renewal
- **Fixed** type — fixed amount discount
- Set usage limits, expiry dates, and per-customer restrictions

### Backup

Configure automated backups:
- **Manual export** — download a full backup archive (database + configs)
- **Manual import** — restore from a backup file
- **Scheduled backups** — set retention days and frequency
- **Restore** — restore a previous backup from the list

### Panel Settings

- Branding (panel name, logo)
- Default language
- Session timeout
- Rate limiting
- Data warning thresholds
- Notification templates (customizable per-event messages)

### Failover (DNS)

Configure DNS-based failover for node domains:
- Add DNS providers (Cloudflare, etc.)
- Register domains
- Automatic health-check based failover between nodes

---

## Reports

Access analytics and reports from the sidebar:
- **Revenue** — daily/monthly income charts
- **Users** — growth and churn metrics
- **Bandwidth** — traffic analysis per node

All reports support CSV export.

---

## Audit Logs

Every admin action is logged with:
- Who performed it (actor)
- What changed (before/after state)
- When and from which IP

Access audit logs from **Settings > Audit Logs**.

---

## Diagnostics

The diagnostics page provides:
- Server status (CPU, memory, disk)
- Recent server logs
- AI-assisted troubleshooting (identifies common issues)
- Auto-healing rules (automatic responses to detected problems)
