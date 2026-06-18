# KorisPanel — Task List

> Updated: 2026-06-18
> Completed / verified items removed. Remaining work for v1.0 release.

---

## 🔴 Critical Bugs — STILL OPEN

- [x] ~~Enable Node Not Working~~ — Fixed: toggle now shows "Enable" only for disabled nodes, "Disable" for online/offline/stale
- [x] ~~Telegram Bot Not Working~~ — Fixed: URL-encoded allowed_updates param, added API response error checking, improved logging
- [x] ~~Dashboard Data Usage Not Fixed~~ — Fixed: always show total/today download/upload stats, not just as chart fallback
- [x] ~~Two Page Titles On Each Page~~ — Fixed: removed duplicate h2 from BackupView and WireGuardPeersView
- [x] ~~Users Page Checkbox Too Big~~ — Fixed: constrained sizing with min/max-width/height, box-sizing, pixel-accurate checkmark
- [x] ~~MariaDB Connection Warning~~ — Fixed: auto-append timeout/readTimeout/writeTimeout DSN params in db.Open()

---

## 🟡 Bugs to Verify (Previously Fixed — Code Confirmed Present)

All fixes verified in codebase. Need production deployment to confirm:

- [x] ~~SQL injection in OpenVPN shell scripts~~ — numeric checks, sanitize CAUSE, escape usernames
- [x] ~~Email header injection~~ — `sanitizeHeader()` removes CRLF
- [x] ~~WebSocket concurrent write race condition~~ — `wsMu sync.Mutex` around writes
- [x] ~~Wallet balance race condition (double-spend)~~ — `SELECT ... FOR UPDATE` in tx
- [x] ~~PrevSessionBytes memory leak~~ — cleanup loop removes stale entries
- [x] ~~Vite version conflict (ERESOLVE)~~ — pinned to `vite: ^5.4.19`
- [x] ~~WireGuard AllowedIPs validation~~ — `net.ParseCIDR` loop in createWireguardPeer
- [x] ~~CertRotation DB Error (migration 024)~~ — migration 025 adds columns, PBT tests pass
- [x] ~~Russian language not in portal~~ — `i18n.ts` has `ru:` block, Shell has button
- [x] ~~Unnecessary session disconnection~~ — only disconnects when status set to non-active
- [x] ~~WireGuard config sync fix~~ — `wg-quick strip` → temp file → `wg syncconf`
- [x] ~~WireGuard remove peer trailing newline~~ — trims trailing empty lines, ensures single newline
- [x] ~~OpenVPN empty network check~~ — explicit check: `if v.OpenVPNNetwork == ""`
- [x] ~~Proxy persistence~~ — `proxy_config` JSON column in nodes, read/write on CRUD
- [x] ~~SSH status fallback~~ — reads from `node_services` table into `StatusMetrics.SSH`
- [x] ~~L2TP redundant toggles~~ — removed from codebase entirely

---

## ⭐ New Features — High Priority

- [x] ~~**WireGuard Protocol Support**~~ — FULLY IMPLEMENTED
  - Peer management, IP allocation, client config generation
  - Gaming optimize (fwmark, MTU 1280, keepalive 15)
  - Node tasks: setup, add_peer, remove_peer, sync_config, update_config
  - Admin UI: WireGuardPeersView, WireGuardConfig component
  - Portal: peer list, config download

- [x] ~~**Passwordless Configs**~~ — IMPLEMENTED (backend)
  - Migration 027 adds global setting + per-plan `allow_passwordless` column
  - `canUsePasswordless()` checks global setting + plan permission
  - Portal `/api/portal/profiles` returns `passwordless_available` flag
  - Profile download supports `?passwordless=true` query param
  - OpenVPN config omits `auth-user-pass` line when passwordless
  - Admin can toggle per plan via existing plan CRUD

- [x] ~~**Tunnel Mode (Iran Traffic)**~~ — FULLY IMPLEMENTED
  - `node/cmd/node/outbound.go` — complete outbound proxy system
  - Protocols: VLESS, VMess, Trojan, Shadowsocks, SOCKS5
  - Per-VPN routing: OpenVPN socks-proxy, IKEv2 updown scripts, SSH ProxyCommand
  - xray-core/sing-box bridge config generation
  - Admin UI has outbound config per node (type, address, UUID, TLS, path, SNI)

- [x] ~~**Backup System Upgrade**~~ — FULLY IMPLEMENTED
  - SQL dump via `mysqldump` (streaming, not buffered)
  - tar.gz archive with dump.sql + node configs + manifest.json
  - Restore with pre-restore safety backup
  - Scheduled backups, retention, admin UI (BackupView)

---

## ⭐ New Features — Medium Priority

- [ ] **Cisco IPSec Protocol**
  - Add Cisco IPSec VPN server support
  - Enterprise client compatibility

- [x] ~~**Gaming Optimize Option**~~ — implemented in WireGuard (fwmark priority routing, MTU 1280, fast keepalive)

- [ ] **Drag & Drop Reordering**
  - Remove "sort order" from all lists
  - Add drag & drop for items

- [ ] **Package Distribution**
  - Build .deb package for panel
  - Build .deb/.rpm package for node

- [ ] **Windows Node Support**
  - Run node agent on Windows Server

---

## 🟢 Enhancements

- [ ] **Ticket System Improvements**
  - Show "Create Ticket" button by default
  - Add notification on new/updated tickets
  - Add delete/archive action

- [ ] **Balance System Enhancement**
  - Major improvements to wallet/credit system
  - Better flexibility and limits
  - Improved transaction tracking

- [ ] **Settings Section Redesign**
  - Complete redesign of settings page
  - Better organization, improved UX/UI

- [ ] **Cores Settings Enhancement**
  - Current implementation inadequate
  - More control options

- [ ] **Changelog System**
  - Full changelog with every change logged
  - Auto-update on each PR/commit, version history

---

## 🔵 Infrastructure — Network Features

- [ ] **QoS / Priority System**
  - tc + fwmark for traffic shaping
  - Per-user bandwidth priority
  - Gaming/VoIP optimization
  - Deps: iproute2, iptables

- [ ] **Firewall Module**
  - iptables/nftables integration
  - Advanced filtering, country blocking, rate limits

- [ ] **Layer 7 Filtering**
  - Protocol-based traffic filtering
  - P2P detection/blocking
  - Deps: iptables-mod-layer7

- [ ] **Load Balancing**
  - Multi-WAN support, PCC-like config
  - Automatic failover, round-robin / sticky sessions

- [ ] **Bandwidth Control**
  - Per-user PCQ/HTB shaping via tc
  - MikroTik-like queue system
  - Real-time speed management

---

## 💰 Business & Billing

- [ ] **Payment Gateway**
  - Stripe, PayPal, Crypto (USDT, BTC)
  - Automatic payment verification

- [ ] **Local Iran Payments**
  - Shetab card, Mellat gateway, ZarinPal

- [ ] **Promo Codes / Coupons**
  - Percentage or fixed amount, usage limits, expiry dates

- [ ] **Referral System**
  - Credit on referral signup, commission percentage, tracking

- [ ] **Multi-Currency**
  - Toman, USD, EUR
  - Automatic conversion, per-plan currency

- [ ] **Grace Period**
  - Extended access after expiry
  - Configurable days, partial access, notification

- [ ] **Multi-Config Purchase (Multi-Login)**
  - Buy additional connection slots
  - Default connections per plan, price per extra
  - Admin override per user

- [ ] **Connection Limit Per User**
  - Admin custom limit override
  - Temporary increase/decrease, audit trail

---

## 🔐 Security Features

- [ ] **Admin Roles / Permissions**
  - Multi-admin, RBAC, granular permissions

- [ ] **Activity Log / Audit Trail**
  - Log all admin actions, searchable, exportable

- [ ] **Session Management**
  - Active sessions list, force logout, timeout settings, IP tracking

---

## 📱 Client Portal Enhancements

- [ ] **Usage Notifications**
  - Alert at 80% / 90% / 100% data limit
  - Email / SMS / Telegram alerts

- [ ] **Auto-Renewal**
  - Charge from wallet, email confirmation, configurable

- [ ] **Mobile Responsive**
  - Better mobile UI/UX, touch-friendly, mobile-optimized tables

- [ ] **Timezone Per User**
  - Auto-detect, local time display

---

## 📊 Reporting & Analytics

- [ ] **Revenue Reports** — daily/weekly/monthly, by plan, by method
- [ ] **User Reports** — registrations, active vs churned, retention
- [ ] **Bandwidth Reports** — per-node, per-user, peak times
- [ ] **Profit / Loss** — costs vs revenue, margin per plan
- [ ] **Export PDF / Excel** — downloadable, scheduled
- [ ] **Custom Dashboard** — drag & drop widgets, role-specific

---

## 🛠️ Technical Features

- [ ] **Webhooks** — event notifications (user created, payment, expiry)
- [ ] **Auto Backup** — scheduled, cloud (S3), local, rotation
- [ ] **Trial Period** — configurable days, limited features, auto-convert
- [ ] **Time-Based Plans** — hourly, daily, weekly, pay-per-use
- [ ] **Data Packs** — buy extra data, add-on packages, stack with plan
- [ ] **Plan Upgrade / Downgrade** — mid-cycle, pro-rated, change history

---

## 🚨 Monitoring & Alerts

- [ ] **Alert System** — email, Telegram, SMS, custom rules
- [ ] **Uptime Monitoring** — health checks, response time, alert on downtime
- [ ] **Server Maintenance** — scheduled mode, user notification, countdown

---

## 🧪 Testing

- [ ] Test all protocols (OpenVPN, L2TP, IKEv2, SSH, WireGuard)
- [ ] Performance testing (500+ users)
- [ ] Security audit
- [ ] Migration guide

---

## 📖 Documentation

- [ ] Complete admin documentation
- [ ] Complete API documentation
- [ ] Complete user guide
- [ ] Complete installation guide

---

## 🔮 Future (Not Priority)

- Anti-DPI Integration
- LDAP/AD Integration
- Server Clustering
- Auto Server Switch
- RTL Support
