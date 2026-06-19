# KorisPanel API Reference

Base URL: `http://your-server:8080`

## Authentication

All admin endpoints require a valid session cookie (`session_id`). Sessions are created via the login endpoint.

Customer (portal) endpoints require a separate customer session cookie.

Node agent endpoints use Bearer token authentication (`Authorization: Bearer <node_token>`).

**Response format**: All endpoints return JSON with `{"ok": true/false, ...}`.

---

## Setup

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/setup/status` | Check if initial setup has been completed |
| POST | `/api/setup/owner` | Create the initial owner account (first-run only) |

## Health

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/health` | Server health check, returns version and uptime |

## Admin Auth

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/auth/admin` | Admin login (username + password) |
| GET | `/api/auth/me` | Get current admin session info |
| POST | `/api/auth/logout` | Destroy admin session |

## Customer Auth

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/auth/customer` | Customer login |
| POST | `/api/auth/customer/logout` | Customer logout |

## Dashboard

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/dashboard/stats` | Aggregated stats (users, revenue, bandwidth, nodes) |

## Customers

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/customers` | List customers (supports pagination, search, status filter) |
| POST | `/api/customers` | Create a new customer |
| POST | `/api/customers/bulk` | Bulk action (enable, disable, delete, traffic_reset) |
| GET | `/api/customers/{id}` | Get customer detail |
| PUT | `/api/customers/{id}` | Update customer |
| DELETE | `/api/customers/{id}` | Soft-delete (archive) customer |
| POST | `/api/customers/{id}/restore` | Restore archived customer |
| POST | `/api/customers/{id}/reset-password` | Reset customer password |
| POST | `/api/customers/{id}/reset-traffic` | Reset traffic counters |
| POST | `/api/customers/{id}/renew` | Renew subscription |
| GET | `/api/customers/{id}/usage` | Get customer bandwidth usage |
| GET | `/api/deleted/customers` | List archived customers |

## Plans

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/plans` | List all plans |
| POST | `/api/plans` | Create a plan |
| GET | `/api/plans/{id}` | Get plan detail |
| PUT | `/api/plans/{id}` | Update plan |
| DELETE | `/api/plans/{id}` | Archive plan |

## Nodes

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/nodes` | List all nodes |
| POST | `/api/nodes` | Create a node |
| GET | `/api/nodes/{id}` | Get node detail (includes runtime metrics) |
| PUT | `/api/nodes/{id}` | Update node configuration |
| DELETE | `/api/nodes/{id}` | Delete node |
| POST | `/api/nodes/{id}/rotate-token` | Rotate node auth token |
| GET/PUT | `/api/nodes/vpn-config/{id}` | Get/update per-node VPN config |

## Node Tasks

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/node/tasks` | List node tasks |
| POST | `/api/node/tasks` | Create a task (restart, update, etc.) |
| GET | `/api/node/tasks/{id}` | Get task detail |
| DELETE | `/api/node/tasks/{id}` | Cancel a pending task |
| GET | `/api/node/tasks/poll` | Node agent polls for pending tasks (Bearer auth) |

## Node Agent

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/node/push` | Node agent pushes status/metrics (Bearer auth) |
| GET | `/api/node/agent/version` | Get latest agent version |
| GET | `/api/node/agent/download` | Download agent binary |

## VPN Settings

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/vpn/settings` | Get global VPN configuration |
| PUT | `/api/vpn/settings` | Update VPN settings (OpenVPN, L2TP, IKEv2) |

## WireGuard

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/wireguard/peers` | List all WireGuard peers |
| POST | `/api/wireguard/peers` | Create a peer |
| GET | `/api/wireguard/peers/{id}` | Get peer detail |
| PUT | `/api/wireguard/peers/{id}` | Update peer |
| DELETE | `/api/wireguard/peers/{id}` | Delete peer |

## Payments

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/payments` | List payments (filterable by status, customer) |
| POST | `/api/payments` | Create manual payment |
| GET | `/api/payments/{id}` | Get payment detail |
| PUT | `/api/payments/{id}` | Update payment status (approve/reject) |
| GET | `/api/wallets/{username}` | Get wallet balance and transactions |

## Payment Methods

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/payment-methods` | List payment methods |
| POST | `/api/payment-methods` | Create payment method |
| PUT | `/api/payment-methods/{id}` | Update payment method |
| DELETE | `/api/payment-methods/{id}` | Deactivate payment method |

## Promo Codes

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/promo-codes` | List promo codes |
| POST | `/api/promo-codes` | Create promo code |
| GET | `/api/promo-codes/{id}` | Get promo code detail |
| PUT | `/api/promo-codes/{id}` | Update promo code |
| DELETE | `/api/promo-codes/{id}` | Delete promo code |

## Tickets

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/tickets` | List support tickets |
| POST | `/api/tickets` | Create ticket (admin-initiated) |
| GET | `/api/tickets/{id}` | Get ticket with messages |
| POST | `/api/tickets/{id}/reply` | Reply to ticket |
| PUT | `/api/tickets/{id}/status` | Close/reopen ticket |

## Resellers

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/resellers` | List resellers |
| POST | `/api/resellers` | Create reseller |
| GET | `/api/resellers/{id}` | Get reseller detail |
| PUT | `/api/resellers/{id}` | Update reseller |
| DELETE | `/api/resellers/{id}` | Delete reseller |
| GET | `/api/resellers/transactions` | List reseller transactions |
| POST | `/api/resellers/checkout` | Reseller checkout (create customers) |
| GET | `/api/resellers/payments` | List reseller payments |

## Certificates

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/certificates` | List uploaded certificates |
| POST | `/api/certificates` | Upload a certificate |
| GET | `/api/certificates/{id}` | Get certificate detail |
| DELETE | `/api/certificates/{id}` | Delete certificate |

## Events & Audit

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/events` | List system events |
| GET | `/api/events/{id}` | Get event detail |
| GET | `/api/audit-logs` | List audit log entries |

## Reports

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/reports/revenue` | Revenue report (daily/monthly) |
| GET | `/api/reports/users` | User growth report |
| GET | `/api/reports/bandwidth` | Bandwidth usage report |

## Exports

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/export/customers.csv` | Export customers as CSV |
| GET | `/api/export/payments.csv` | Export payments as CSV |
| GET | `/api/export/radacct.csv` | Export RADIUS accounting as CSV |
| GET | `/api/export/wallet-transactions.csv` | Export wallet transactions as CSV |
| GET | `/api/export/revenue.csv` | Export revenue report as CSV |

## Backup

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/backup/export` | Create and download full backup |
| POST | `/api/backup/import` | Import backup archive |
| GET | `/api/admin/backups` | List available backups |
| GET | `/api/admin/backups/{id}` | Get backup detail |
| POST | `/api/admin/backups/restore` | Restore a backup |
| GET/PUT | `/api/admin/backups/settings` | Backup schedule settings |

## Diagnostics

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/diagnostics` | System diagnostics overview |
| GET | `/api/diagnostics/logs` | Server logs |
| GET | `/api/diagnostics/status` | Server status (CPU, memory, disk) |
| POST | `/api/diagnostics/ai` | AI-assisted diagnostics |
| GET | `/api/diagnostics/ai/history` | AI diagnostics history |
| GET/POST | `/api/diagnostics/ai/rules` | Healing rules |
| GET | `/api/diagnostics/ai/healing-log` | Healing action log |

## Settings

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET/PUT | `/api/panel-settings` | Panel settings (Telegram, branding, etc.) |
| GET | `/api/public-settings` | Public-facing settings (no auth required) |
| GET/PUT | `/api/settings/data-warning-thresholds` | Data warning thresholds |
| GET/PUT | `/api/settings/warning-config` | Warning notification config |
| GET/PUT | `/api/templates` | Notification templates |
| GET/PUT | `/api/templates/{id}` | Template detail |

## Failover

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET/POST | `/api/failover/providers` | DNS failover providers |
| GET/PUT/DELETE | `/api/failover/providers/{id}` | Provider detail |
| GET/POST | `/api/failover/domains` | Failover domains |
| GET/PUT/DELETE | `/api/failover/domains/{id}` | Domain detail |

## Sessions

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/sessions/kill` | Kill an active VPN session |

## Realtime

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/realtime` | WebSocket for live updates (sessions, bandwidth, events) |

---

## Portal (Customer) Endpoints

These endpoints require customer session authentication.

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/portal/me` | Get current customer profile and subscription |
| GET | `/api/portal/usage` | Get bandwidth usage summary |
| GET | `/api/portal/nodes` | List available VPN nodes |
| GET | `/api/portal/profiles` | List available VPN profiles for download |
| GET | `/api/portal/profiles/{type}` | Download a specific VPN profile |
| GET | `/api/portal/plans` | List available plans for renewal |
| POST | `/api/portal/renew` | Request plan renewal |
| POST | `/api/portal/password` | Change password |
| POST | `/api/portal/preferred-node` | Set preferred VPN node |
| GET | `/api/portal/payments` | List customer payments |
| GET | `/api/portal/payment-methods` | List active payment methods |
| GET | `/api/portal/tickets` | List customer tickets |
| POST | `/api/portal/tickets` | Create a support ticket |
| GET | `/api/portal/tickets/{id}` | Get ticket detail |
| POST | `/api/portal/tickets/{id}/reply` | Reply to ticket |
| GET | `/api/portal/events` | List customer events |
| GET | `/api/portal/events/{id}` | Get event detail |
| GET | `/api/portal/warnings` | Get active data warnings |
| POST | `/api/portal/apply-promo` | Apply a promo code |
| GET | `/api/portal/app-links` | Get VPN client download links |
| GET/POST | `/api/portal/wireguard/peers` | WireGuard peer management |
| GET/PUT/DELETE | `/api/portal/wireguard/peers/{id}` | WireGuard peer detail |

## Subscription Link

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/portal/sub` | Subscription link (returns config based on client User-Agent) |
