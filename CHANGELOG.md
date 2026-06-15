# KorisPanel / koris-next Changelog

All notable changes to the clean Go + Vue rewrite are tracked here.

## 0.36.0 - 2026-06-15

### Added
- **Telegram Bot** — native Go implementation with full admin and customer commands
  - Dual mode: webhook (`PANEL_TG_WEBHOOK_URL`) and long-polling (default)
  - Admin commands: `/stats`, `/users`, `/find`, `/create`, `/enable`, `/disable`, `/traffic`
  - Customer commands: `/me`, `/usage`, `/plans`
  - Inline keyboards for quick user actions
  - Auto-notifications to admin chats (payments, node status, expiry)
  - Webhook endpoint at `/api/bot/webhook`
- **Per-node VPN configuration** (migration 011)
  - Separate OpenVPN/L2TP/IKEv2/SSH config per node
  - API: `GET/POST /api/nodes/vpn-config/{node_id}`
- **SSH VPN protocol** support
  - SSH tunnel accounts table with per-node assignment
  - Configurable port, max connections, expiry
- **Static certificate management**
  - Upload CA/tls-crypt/client certs for server-switching with same config
  - API: `GET/POST /api/certificates`, `GET/DELETE /api/certificates/{id}`
- **Panel settings** API (migration 012)
  - Key-value settings: theme, branding, SSH toggle, Telegram config
  - API: `GET/PATCH /api/panel-settings`
- **Phase 1 Premium UX** — modal-first design, grouped sidebar nav (Overview/Manage/System)

### Changed
- `/api/health` version is now `0.36.0`
- Sidebar nav restructured: OVERVIEW (Dashboard, Analytics, Transactions) / MANAGE (Users, Services, Billing) / SYSTEM (Settings)
- All detail views open as modals (user detail, ticket detail)
- "+ New User" button only on Users page
- Toast notifications positioned top-right with auto-dismiss
- Resellers moved inside Users as sub-tab

### Fixed
- Deploy script: excludes `panel/web/` from git dirty check
- Build artifacts (www/assets/) removed from git tracking
- Background no longer scrolls behind content
- Portal ticket messages properly stacked in modal

### Security
- SQL injection fixed in OpenVPN connect/disconnect scripts
- DB connection pool configured (50 max, 5min lifetime)
- Security warnings logged when credentials not configured
- Reseller self-checkout gated behind env flag
- PSK removed from Clash YAML config comments

## 0.33.0 - 2026-06-14

### Added
- Prominent copyable subscription link widgets with clipboard copy helpers:
  * Inside **Admin Panel** (on customer details screen under active plan information).
  * Inside **Customer Portal** (on customer dashboard under VPN profile downloads).
- Active L2TP and IKEv2 dynamic `.mobileconfig` download profiles rendered in the Customer Portal (replying to previous 'profile soon' placeholder blocks).

### Fixed
- Started stopped systemd service daemons `xl2tpd` and `strongswan-starter` ensuring L2TP/IPSec and IKEv2 profiles compile and connect correctly.

### Changed
- `/api/health` version is now `0.33.0`.

## 0.32.0 - 2026-06-14

### Optimized (UI/UX Pro Max Guideline Alignments)
- Clean Tabbed Customer Detail Layout: Restructured the customer details section. Instead of dumping 8 massive, cluttered, vertical cards (which caused high cognitive load), they are now grouped under a premium, secondary tabbed structure:
  * **Profile & Wallet**: Core edits, password reset, wallet balance sets, plan application.
  * **VPN Sessions & Usage**: Live network metrics summary, active VPN sessions log table.
  * **Ledgers & History**: Full audit records, billing logs, subscriptions history, raw FreeRADIUS row attributes.

### Added
- Reseller manual receipt request system `POST /api/resellers/payments` allowing resellers to submit Card-to-Card receipts to the master owner for offline credit allocation approval.

### Changed
- `/api/health` version is now `0.32.0`.

## 0.31.0 - 2026-06-14

### Fixed
- Unhandled null arrays crash bug: Added safety fallbacks `|| []` to customer `wallet_transactions`, `subscriptions`, `radius_checks`, and `radius_replies` arrays in Vue template (preventing page from crashing into a dark screen when clicking customers with no activity history like 'wini').

### Added
- URL Hash Router: Section state is now synchronized with `window.location.hash`, preventing the dashboard from resetting to the first tab when a page refresh is performed.
- Modern Glass Slate Theme: Adjusted styles and gradients in `style.css` to bring a more readable, slightly lighter, premium deep slate blue aesthetic instead of excessively dark black shades.

### Removed
- Topbar Manual Sync button: Everything is now completely dynamic and live-synchronized.

### Changed
- `/api/health` version is now `0.31.0`.

## 0.29.0 - 2026-06-14

### Added
- Reseller Audit Ledger UI on the Resellers page, showing live credit transactions, allocations, and plan deductions.
- Step-by-step native connection guides for iOS, macOS, and Windows inside the dynamic subscriber web portal.
- Auto-disconnect session kicks (RADIUS CoA Disconnect-Requests) whenever an admin manually disables a customer or updates their active plan/data limits.

### Changed
- `/api/health` version is now `0.29.0`.

## 0.28.0 - 2026-06-14

### Added
- Multi-language subscriber portal supporting English, Persian, Russian, and Chinese (`lang=en/fa/ru/zh` query selection) with clean translation structures and RTL support.
- Node analytics history API tracking the last 15 throughput and online users snapshots of each active node (`node_usage_snapshots`).
- Real-time SVG throughput line charts rendered directly on node details cards inside the admin panel.

### Changed
- `/api/health` version is now `0.28.0`.

## 0.26.0 - 2026-06-14

### Added
- Reseller ledger table `reseller_transactions` tracking all balance allocations and plan deductions.
- Clash YAML dynamic parser in `/portal/sub` subscription engine (serving elegant YAML proxy nodes for Clash clients).
- Session kill trigger `/api/sessions/kill` using local RADIUS Disconnect-Request (CoA) packets and database session closing.
- Disconnect button in active connections speedometer table in admin overview dashboard.

### Changed
- `/api/health` version is now `0.26.0`.

## 0.25.0 - 2026-06-14

### Added
- Full pre-paid reseller sub-admin ecosystem with balance credit column.
- Segmented admin view isolating reseller created customers.
- Real-time speedometer streaming of active sessions (in KB/s) via WebSocket connection.
- Smart HTML subscriber portal rendering on standard browser agents.

### Changed
- `/api/health` version is now `0.25.0`.

## 0.24.0 - 2026-06-14

### Added
- Real-time diagnostics endpoint `GET /api/diagnostics` and corresponding admin view.
- Diagnostics UI checking disk, memory, and services status (Nginx, MariaDB, FreeRADIUS, Panel, OpenVPN, node-agent, L2TP, IKEv2).
- Dynamic listing of listening ports (HTTP, OpenVPN, RADIUS) directly in the admin dashboard.

### Fixed
- L2TP and IKEv2 mobile config profiles now correctly respect the selected `node_id` query parameter using `openVPNEndpointNode`.

### Changed
- `/api/health` version is now `0.24.0`.

## 0.23.0 - 2026-06-14

### Added
- Customer self-service password change via portal.
- `POST /api/portal/password` endpoint with old password verification and radcheck update.
- Portal security card with current/new password form.
- Event auto-creation on successful password change.

### Changed
- `/api/health` version is now `0.23.0`.

## 0.22.0 - 2026-06-14

### Added
- Multi-node profile selection for portal users.
- `GET /api/portal/nodes` — list available non-disabled nodes for the logged-in customer.
- `portalProfiles` now accepts `node_id` query parameter to generate profiles for a specific node.
- `portalProfileDownload` now respects `node_id` query parameter for OpenVPN, L2TP, and IKEv2 downloads.
- `openVPNEndpointNode` helper for node-specific endpoint resolution.
- Portal VPN access card now shows a node selector dropdown when multiple nodes are available.
- Profile download URLs include `node_id` query parameter.

### Changed
- `/api/health` version is now `0.22.0`.

## 0.21.0 - 2026-06-14

### Added
- Event notification system using existing `events` table.
- Admin APIs:
  - `GET /api/events` — list events with pagination, filter by type/seen.
  - `POST /api/events/{id}/seen` — mark event seen.
- Portal APIs:
  - `GET /api/portal/events` — list events related to the logged-in customer.
  - `POST /api/portal/events/{id}/seen` — mark customer event seen.
- Dashboard stats now include `unseen_events` count.
- Admin notification bell in topbar with unread events dropdown.
- Portal notification bell with unread customer events.
- Event auto-creation on key actions: customer created, plan applied, payment approved/rejected, ticket created/replied.

### Changed
- `/api/health` version is now `0.21.0`.

## 0.20.0 - 2026-06-14

### Added
- L2TP/IPSec Apple `.mobileconfig` profile generation via `GET /api/portal/profiles/l2tp.mobileconfig`.
- IKEv2 Apple `.mobileconfig` profile generation via `GET /api/portal/profiles/ikev2.mobileconfig`.
- Portal profiles list now includes L2TP and IKEv2 availability, remote host, and download links.
- L2TP profile embeds IPSec PSK from admin VPN settings as base64-encoded shared secret.
- IKEv2 profile uses EAP-MSCHAPv2 username authentication with AES-256-GCM + SHA2-384 parameters.
- Portal connection profiles card shows OpenVPN, L2TP, and IKEv2 rows with download buttons and hints for Apple vs manual setup.
- `safeFilename` helper for consistent attachment filenames.

### Changed
- `/api/health` version is now `0.20.0`.

## 0.19.0 - 2026-06-14

### Added
- Audit logs helper and `GET /api/audit-logs` endpoint with pagination.
- Audit logging instrumentation for key actions: customer create/update/archive/restore, plan create/update/deactivate, node create/update/token rotation, payment create/approve/reject, wallet adjust/set, VPN settings save, node task create, ticket create/reply/status change, payment method create/update/deactivate.
- CSV export endpoints:
  - `GET /api/export/customers.csv`
  - `GET /api/export/payments.csv`
  - `GET /api/export/radacct.csv`
  - `GET /api/export/wallet-transactions.csv`
- Admin Audit Logs page (`/dashboard/`) with paginated table and JSON before/after columns.
- Admin Backups/Exports page with direct CSV download buttons.
- Background worker goroutine in the panel process that runs every minute:
  - Auto-expires active subscriptions when `expires_at <= NOW()`.
  - Auto-limits active customers when `radacct` usage exceeds `Max-Data`.
  - Closes stale RADIUS sessions with no update in 5 minutes.
  - Marks stale nodes offline after 5 minutes of no push.
  - Daily SQL backup at 02:00 via `mysqldump` to `/var/backups/koris-next/db-YYYY-MM-DD.sql.gz`.

### Changed
- `/api/health` version is now `0.19.0`.

## 0.18.0 - 2026-06-14

### Added
- Payment intent columns via `007_payment_intents.sql`.
- Linked payment approval workflow:
  - `wallet_topup` credits wallet only.
  - `plan_renewal` credits wallet, then applies the selected plan and deducts plan price.
- Portal renewal requests now create pending payments linked to the selected plan.
- Admin/portal payment lists now show payment intent and target plan.

### Changed
- Payment approval is now intent-aware and idempotent for wallet top-up vs plan purchase transactions.
- `/api/health` version is now `0.18.0`.

## 0.17.0 - 2026-06-14

### Added
- Payment methods CRUD in the admin Payments page.
- Portal payment method selection.
- Portal payment instructions display from payment method configuration.
- Payment method APIs:
  - `GET /api/payment-methods`
  - `POST /api/payment-methods`
  - `PATCH /api/payment-methods/{id}`
  - `DELETE /api/payment-methods/{id}`
  - `GET /api/portal/payment-methods`

### Changed
- `/api/health` version is now `0.17.0`.

## 0.16.0 - 2026-06-14

### Added
- Admin support ticket queue.
- Admin ticket detail, reply, close, and reopen workflow.
- Portal ticket creation, ticket history, detail, reply, and close workflow.
- Ticket APIs:
  - `GET /api/tickets`
  - `POST /api/tickets`
  - `GET /api/tickets/{id}`
  - `POST /api/tickets/{id}/reply`
  - `POST /api/tickets/{id}/close`
  - `POST /api/tickets/{id}/open`
  - `GET /api/portal/tickets`
  - `POST /api/portal/tickets`
  - `GET /api/portal/tickets/{id}`
  - `POST /api/portal/tickets/{id}/reply`
  - `POST /api/portal/tickets/{id}/close`

### Changed
- `/api/health` version is now `0.16.0`.

## 0.15.0 - 2026-06-14

### Added
- Admin VPN settings page.
- Admin VPN settings API:
  - `GET /api/vpn/settings`
  - `PATCH /api/vpn/settings`
- Runtime VPN status display for OpenVPN service, active node, remote host, CA file, and tls-crypt file.
- Optional "Save & restart OpenVPN" flow that rewrites core OpenVPN server settings and restarts OpenVPN.

### Changed
- `/api/health` version is now `0.15.0`.

## 0.14.1 - 2026-06-14

### Added
- OpenVPN speed limit enforcement through Linux `tc` in `koris-client-connect.sh`.
- OpenVPN speed limit cleanup in `koris-client-disconnect.sh`.
- Speed policy reads `Mikrotik-Rate-Limit` first and falls back to plan `speed_mbps`.

### Removed
- OpenVPN connect/disconnect scripts no longer call old legacy `cc-up.sh` / `cc-down.sh`.

### Changed
- `/api/health` version is now `0.14.1`.

## 0.14.0 - 2026-06-14

### Added
- OpenVPN auth-time policy enforcement in `koris-radius-auth.sh`.
- Customer status enforcement: disabled, deleted, expired, and limited users are rejected at VPN login.
- Subscription expiry enforcement at VPN login; expired latest subscription marks customer `expired`.
- Data limit enforcement at VPN login using `Max-Data` and accumulated `radacct` usage; exhausted users are marked `limited`.

### Changed
- `/api/health` version is now `0.14.0`.

## 0.13.0 - 2026-06-14

### Added
- Admin customer usage/session API:
  - `GET /api/customers/{id}/usage`
- Portal usage/session API:
  - `GET /api/portal/usage`
- Admin customer detail usage summary and recent session table.
- Portal usage card with total, download, upload, remaining quota, active sessions, and recent sessions.

### Changed
- `/api/health` version is now `0.13.0`.

## 0.12.4 - 2026-06-14

### Changed
- Generated OpenVPN profiles now include `explicit-exit-notify 1` for cleaner UDP disconnect/accounting stop handling.
- `/api/health` version is now `0.12.4`.

## 0.12.3 - 2026-06-14

### Added
- OpenVPN auth script using `radclient` against FreeRADIUS.
- OpenVPN accounting connect/disconnect scripts writing to `radacct`.
- Installer helper `scripts/install-openvpn-scripts.sh`.

### Changed
- OpenVPN no longer depends on the old `radiusplugin.so` for auth/accounting.
- OpenVPN server hooks are standardized to Koris auth/accounting wrappers.
- `/api/health` version is now `0.12.3`.

### Fixed
- OpenVPN `AUTH_FAILED` caused by old radius plugin no-response behavior.

## 0.12.2 - 2026-06-13

### Fixed
- FreeRADIUS SQL configuration now points to the active KorisPanel database (`radius_next` on the test server).
- OpenVPN `user/123456` authentication now returns FreeRADIUS `Access-Accept` after DB config repair.
- Installer now configures FreeRADIUS SQL credentials/database during panel install.

### Changed
- `/api/health` version is now `0.12.2`.

## 0.12.1 - 2026-06-13

### Fixed
- OpenVPN generated profile now embeds CA certificate when available.
- OpenVPN generated profile now embeds `tls-crypt` key when available.
- Added `setenv CLIENT_CERT 0` so username/password profiles do not request a client certificate in OpenVPN clients.
- Added OpenVPN cipher/auth options matching the current server config.

### Changed
- `/api/health` version is now `0.12.1`.

## 0.12.0 - 2026-06-13

### Added
- Portal OpenVPN profile metadata endpoint:
  - `GET /api/portal/profiles`
- Portal OpenVPN profile download endpoint:
  - `GET /api/portal/profiles/openvpn.ovpn`
- Dynamic OpenVPN profile generation using active node/domain and core VPN settings.
- Portal download buttons for generated OpenVPN profile.

### Changed
- `/api/health` version is now `0.12.0`.

## 0.11.0 - 2026-06-13

### Added
- Node task queue migration `005_node_tasks.sql`.
- Admin node task UI with recent task list.
- Node task APIs:
  - `GET /api/node/tasks`
  - `POST /api/node/tasks`
  - `POST /api/node/tasks/poll`
  - `POST /api/node/tasks/{id}/complete`
  - `POST /api/node/tasks/{id}/cancel`
- Node agent polling and task completion.
- Safe node actions:
  - `agent.status`
  - `service.status`
  - `service.restart`
  - `service.reload`
- Admin node quick actions for ping and VPN service restarts.

### Changed
- `/api/health` version is now `0.11.0`.
- Node agent now pushes status and polls tasks in the same loop.

## 0.10.1 - 2026-06-13

### Added
- Node token modal now shows a ready install command below the one-time token.
- Install command includes `PANEL_URL`, `NODE_TOKEN`, and `NODE_NAME`.

### Changed
- `/api/health` version is now `0.10.1`.

## 0.10.0 - 2026-06-13

### Added
- Version file and changelog are now maintained for each update.
- Node management admin UI.
- Node APIs:
  - `GET /api/nodes`
  - `POST /api/nodes`
  - `GET /api/nodes/{id}`
  - `PATCH /api/nodes/{id}`
  - `POST /api/nodes/{id}/rotate-token`
  - `POST /api/nodes/{id}/enable`
  - `POST /api/nodes/{id}/disable`
- Node status push API:
  - `POST /api/node/push`
- Node token generation and SHA-256 token hash storage.
- Node status storage in `node_status`, `node_services`, and `node_usage_snapshots`.
- Node agent now pushes CPU, RAM, disk, network, and service status.

### Changed
- `/api/health` version is now `0.10.0`.
- Dashboard node count can be managed from the UI.

## 0.9.0 - 2026-06-13

### Added
- Customer soft delete/archive workflow.
- Customer restore workflow from `deleted_archive` payload.
- Admin Archive page for deleted customers.
- Archive/restore APIs:
  - `GET /api/deleted/customers`
  - `DELETE /api/customers/{id}`
  - `POST /api/customers/{id}/restore`
- Radius policy archival for deleted users:
  - archives `radcheck`
  - archives `radreply`
  - removes live Radius rows while archived
  - restores Radius rows on restore

### Changed
- Admin customer detail now includes an Archive action.

## 0.8.0 - 2026-06-13

### Added
- Portal plan selection and renewal flow.
- Portal APIs:
  - `GET /api/portal/plans`
  - `POST /api/portal/renew`
- Automatic pending payment request if portal user selects a plan but wallet balance is insufficient.
- Admin plan create/edit moved into modal popup.
- IRT suffix in money display.

## 0.7.0 - 2026-06-13

### Added
- `wallet_transactions.reference_type` and `wallet_transactions.reference_id`.
- Portal payment request form and payment history.
- Portal payment APIs:
  - `GET /api/portal/payments`
  - `POST /api/portal/payments`
- Pending payment counter for admin dashboard.

### Fixed
- Payment approval/rejection wallet sync is now reference-aware and idempotent for new transactions.

## 0.6.0 - 2026-06-13

### Added
- Customer detail wallet transaction history.
- Customer detail subscription history.
- Wallet set-balance API.

## 0.5.0 - 2026-06-13

### Added
- Customer renewal/apply-plan flow.
- `Pay as you go` default plan seed.
- Wallet deduction when paid plan is activated.

## 0.4.0 - 2026-06-13

### Added
- Payments page.
- Wallet adjustment.
- Speed limit fields using `Mikrotik-Rate-Limit`.
- Unlimited data/speed behavior via blank or `0` values.
- WebSocket realtime stats endpoint.

## 0.3.0 - 2026-06-13

### Added
- Customer detail page.
- Plans CRUD.
- Customer create with plan defaults.

## 0.2.0 - 2026-06-13

### Added
- Vue admin dashboard and customer portal.
- Setup/login UI.
- Customer list/create UI.
- Go static SPA serving for `/dashboard/` and `/portal/`.

## 0.1.0 - 2026-06-13

### Added
- Initial clean rewrite skeleton.
- Go panel service.
- Vue admin/portal skeletons.
- Clean DB migration `001_init.sql`.
- Panel/node split structure.
