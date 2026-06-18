---
inclusion: always
---

# KorisPanel

VPN management panel (Go 1.22 + Vue 3). Iran market. FreeRADIUS auth. MariaDB. 1GB RAM target.

## Layout

- `panel/cmd/panel/` — server entrypoint
- `panel/internal/` — api, auth, backup, bot, certrotation, config, db, health, notify, ratelimit, sessions, templates, wireguard
- `panel/migrations/` — sequential SQL (`NNN_desc.sql`)
- `panel/web/admin/` — admin SPA (Vue 3 + Vite, base: `/dashboard/`)
- `panel/web/portal/` — customer SPA (base: `/portal/`)
- `panel/web/shared/` — shared components/composables/types
- `node/cmd/node/` — node agent entrypoint
- `node/internal/` — config, logger, updater

## Domain

Entities: Customer, Plan, Subscription, Wallet, Node, Ticket, Event, VPN Profile
Protocols: OpenVPN, L2TP, IKEv2, SSH Tunnel, WireGuard
Billing: subscription (fixed) or PAYG (wallet-based)
Statuses — customer: active/disabled/expired/limited/deleted | node: online/offline/stale/disabled
Admin roles: owner > admin > support

## Conventions

- JSON responses: `{"ok": true/false, ...}`
- SQL: parameterized queries only, `DECIMAL(12,2)` for money
- Soft-delete: `deleted_at` + `deleted_archive` table
- Node comms: pull-based (agent polls tasks, pushes status)
- i18n: EN, FA, ZH, RU for all UI strings
- Update CHANGELOG.md on user-visible changes
