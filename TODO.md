# KorisPanel — Task List

> Updated: 2026-06-19
> Completed / verified items removed. Remaining work for v1.0 release.

---

## ✅ Completed This Session

- All 6 critical bugs fixed
- All 16 verification bugs confirmed
- SSL/HTTPS auto-detection + nginx management API + TLS fallback
- Portal login error display fixed
- Cores tab redesigned (WireGuard uniform, SSH status fixed, auto-start/stop)
- Backup moved to Settings sub-tab
- Node agent: cross-platform build, config management from panel
- Custom config filenames (emoji/UTF-8, per-user vs global logic)
- Dual OpenVPN profiles (UDP + TCP) with failover
- Per-user preferred node selection (portal API + UI)
- Static configs with global vpn_domain + backup domains
- Performance: indexes, /20 subnets, 2-core/4GB tuning, MariaDB optimization
- Promo codes (backend + admin UI + portal UI)
- Grace period enforcement
- Multi-currency support (DB schema)
- Passwordless configs
- WireGuard peers automated (customer detail view)

---

## ⭐ New Features — Medium Priority

- [ ] **Cisco IPSec Protocol**
  - Add Cisco IPSec VPN server support
  - Enterprise client compatibility

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

- [x] ~~Ticket System Improvements~~ — Telegram notifications on new/reply
- [ ] **Balance System Enhancement** — (wallet already works, need UI improvements)
- [ ] **Changelog System** — auto from git commits (CHANGELOG.md already exists)
- [ ] **Drag & Drop Reordering** — UI only

---

## 🔵 Infrastructure — Network Features

- [ ] **QoS / Priority System**
  - tc + fwmark for traffic shaping
  - Per-user bandwidth priority

- [ ] **Firewall Module**
  - iptables/nftables integration
  - Country blocking, rate limits

- [ ] **Layer 7 Filtering**
  - P2P detection/blocking

- [ ] **Load Balancing**
  - Multi-WAN, automatic failover

- [ ] **Bandwidth Control**
  - Per-user PCQ/HTB shaping via tc

---

## 💰 Business & Billing

- [x] ~~Promo Codes~~ — backend + admin UI + portal UI
- [x] ~~Grace Period~~ — per-plan grace_days, worker enforcement
- [x] ~~Multi-Currency~~ — DB schema (plans.currency, toman_rate)
- [x] ~~Auto-Renewal~~ — from wallet, worker logic, Telegram notification
- [x] ~~Referral System~~ — DB schema (referrals table, referral_code, referred_by)
- [x] ~~Connection Limit Per User~~ — conn_limit_override column
- [x] ~~Plan Upgrade/Downgrade~~ — plan_changes tracking table
- [ ] **Multi-Config Purchase** — buy extra connection slots (future)

---

## 🔐 Security Features

- [x] ~~Admin Roles / Permissions~~ — schema (permissions column, role-based access)
- [x] ~~Activity Log / Audit Trail~~ — audit_logs table, logAudit() on all actions
- [x] ~~Session Management~~ — admin_sessions table (IP, user agent, expiry)

---

## 📱 Client Portal Enhancements

- [x] ~~Node selection~~ — dropdown in portal, preferred node saved
- [x] ~~Promo code input~~ — apply codes in portal
- [x] ~~Usage Notifications~~ — 80%/95% alerts via Telegram + events
- [x] ~~Auto-Renewal~~ — charge from wallet when subscription expires
- [x] ~~Timezone Per User~~ — DB column ready, frontend auto-detects
- [ ] **Mobile Responsive** — better mobile UI (cosmetic)

---

## 📊 Reporting & Analytics

- [x] ~~Revenue Reports~~ — GET /api/reports/revenue (daily/weekly/monthly, by plan)
- [x] ~~User Reports~~ — GET /api/reports/users (registrations, status breakdown)
- [x] ~~Bandwidth Reports~~ — GET /api/reports/bandwidth (per-node, top users)
- [ ] **Export PDF / Excel** — downloadable reports (future)

---

## 🛠️ Technical Features

- [x] ~~Webhooks~~ — schema (webhooks + webhook_logs tables)
- [x] ~~Trial Period~~ — schema (trial_enabled, trial_days, trial_used)
- [x] ~~Time-Based Plans~~ — duration_hours column, hourly/daily support
- [x] ~~Data Packs~~ — data_packs + customer_data_packs tables
- [ ] **Plan Upgrade / Downgrade UI** — frontend for pro-rated changes

---

## 🚨 Monitoring & Alerts

- [x] ~~Alert System~~ — schema (alert_rules table, default rules)
- [x] ~~Ticket Notifications~~ — Telegram on new ticket + reply
- [x] ~~Maintenance Mode~~ — settings for maintenance_mode, message, ends_at
- [ ] **Uptime Monitoring UI** — dashboard widget for node health

---

## 🧪 Testing & Docs

- [ ] Test all protocols
- [ ] Performance testing (500+ users)
- [ ] Security audit
- [ ] Admin documentation
- [ ] API documentation
- [ ] Installation guide

---

## 🔮 Future (Not Priority)

- Xray/VLESS protocol (smart TCP proxy per-user routing)
- HAProxy integration for TCP OpenVPN node selection
- Anti-DPI Integration
- LDAP/AD Integration
- Server Clustering
- RTL Support
- Package distribution (.deb/.rpm)
