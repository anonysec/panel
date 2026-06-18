---
inclusion: always
---

# Tech Stack & Development Guide

## Backend (Go)

- **Go 1.22** | Module: `KorisPanel`
- **HTTP**: `net/http` with `http.ServeMux` — no external router
- **Database**: MySQL via `github.com/go-sql-driver/mysql`
- **WebSocket**: `github.com/gorilla/websocket`
- **Crypto**: `golang.org/x/crypto` (bcrypt for passwords)
- **Testing**: `testing` + `github.com/DATA-DOG/go-sqlmock`

## Frontend (TypeScript / Vue 3)

- **Framework**: Vue 3 Composition API (`<script setup>`)
- **Build**: Vite 5
- **State**: Pinia
- **Router**: Vue Router 4
- **Utilities**: @vueuse/core
- **Testing**: Vitest + @vue/test-utils + happy-dom
- **Property-Based Testing**: fast-check

## Infrastructure

- **Database**: MySQL / MariaDB (FreeRADIUS schema)
- **Process Manager**: systemd
- **Target**: Single-core Linux VPS, 1 GB RAM
- **CI/CD**: GitHub Actions (`.github/workflows/deploy.yml`)
- **Migrations**: Sequential numbered SQL in `panel/migrations/`

## Commands

```bash
# Backend
go build -o /usr/local/bin/panel ./panel/cmd/panel
go build -o /usr/local/bin/node-agent ./node/cmd/node
go test ./...
go test ./panel/internal/<pkg>/...
go mod tidy

# Frontend (admin | portal | shared)
cd panel/web/<app>
npm install
npm run build     # Production build
npm run test      # vitest run

# Deploy
./deploy.sh       # Pull, build all, restart service
```

## Memory Constraints

The panel targets 1 GB RAM hosts. Respect these runtime settings:

- `GOMAXPROCS(1)` — single OS thread
- `GOGC=50` — aggressive GC to keep heap small
- `GOMEMLIMIT=100MB` — soft memory ceiling

Avoid unbounded in-memory collections. Prefer streaming or pagination for large result sets.

## Code Style & Patterns

### Go

- Use `http.ServeMux` for routing — do not add a third-party router.
- All SQL must use prepared statements or parameterized queries. Never concatenate user input into SQL strings.
- Error handling: return errors up the call stack; log once at the boundary (handler or worker).
- Background workers are launched as goroutines from `main.go`. Each must be stoppable via context cancellation.
- Exported functions in `internal/` packages get doc comments.

### Vue / TypeScript

- Use `<script setup lang="ts">` for all components.
- All user-facing strings must go through i18n. Supported locales: EN, FA, ZH, RU.
- Composables live in `composables/` directories; shared ones in `panel/web/shared`.
- Prefer `ref`/`computed` over `reactive` for simple state.

### SQL Migrations

- File naming: `NNN_short_description.sql` (zero-padded three digits).
- Each file is applied once on startup by `db.Migrate()`. Migrations are forward-only (no down files).
- Use `IF NOT EXISTS` / `IF EXISTS` guards for idempotency where practical.

## Mandatory Practices

1. **CHANGELOG** — Update `CHANGELOG.md` on every user-visible change.
2. **i18n** — All UI text uses translation keys (EN, FA, ZH, RU).
3. **Tests** — Write unit tests for new backend functions. Use `go-sqlmock` for DB-dependent code.
4. **Security** — Flag potential vulnerabilities immediately. Never store secrets in code.
5. **Existing patterns** — Match the style and structure of surrounding code rather than introducing new conventions.

## Agent Behavior

- Execute tasks immediately without asking for confirmation or clarification.
- Use best judgment when information is incomplete; document assumptions made.
- If an error occurs during execution, fix it and continue — do not stop to ask.
- Report results after completion, not before.
