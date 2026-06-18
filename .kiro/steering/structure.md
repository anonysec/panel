# Project Structure

```
KorisPanel/
├── panel/                      # Main panel application
│   ├── cmd/panel/main.go       # Entrypoint: starts HTTP server, workers, bot
│   ├── internal/               # Internal packages (not importable externally)
│   │   ├── api/                # HTTP handlers, routes, middleware, WebSocket
│   │   ├── auth/               # Admin/customer authentication, sessions
│   │   ├── bot/                # Telegram bot integration
│   │   ├── certrotation/       # Certificate rotation worker
│   │   ├── config/             # Configuration loading from env
│   │   ├── csrf/               # CSRF protection middleware
│   │   ├── db/                 # Database connection + migration runner
│   │   ├── email/              # Email notifications
│   │   ├── health/             # Health checks, diagnostics engine, analyzer rules
│   │   ├── notify/             # Telegram notification helper
│   │   ├── ratelimit/          # Per-IP rate limiting middleware
│   │   ├── sessions/           # VPN session enforcement (connection limits)
│   │   ├── templates/          # User template logic
│   │   └── wireguard/          # WireGuard key generation helpers
│   ├── migrations/             # Numbered SQL migration files (001_init.sql, ...)
│   ├── web/
│   │   ├── admin/              # Admin dashboard SPA (Vue 3 + Vite)
│   │   ├── portal/             # Customer portal SPA (Vue 3 + Vite)
│   │   └── shared/             # Shared Vue components, composables, types
│   └── systemd/                # panel.service unit file
│
├── node/                       # Node agent (deployed on VPN servers)
│   ├── cmd/node/main.go        # Agent entrypoint: heartbeat loop, task executor
│   ├── internal/
│   │   ├── config/             # Agent config (env file based)
│   │   ├── logger/             # Structured JSON logger
│   │   └── updater/            # Self-update mechanism
│   ├── node.sh                 # Node setup script
│   └── systemd/                # node-agent.service unit file
│
├── deploy.sh                   # Full deployment script
├── install.sh                  # Initial installation script
├── node-install.sh             # Node agent installation script
├── go.mod / go.sum             # Go module definition
└── CHANGELOG.md                # Release changelog
```

## Key Patterns

- **Monorepo**: Panel server, node agent, and frontends live in one repository
- **Go `internal/` convention**: All business logic is unexported under `internal/`
- **SQL migrations**: Sequential numbered files applied on startup by `db.Migrate()`
- **SPA hosting**: Go serves pre-built Vue apps from `web/*/www/` directories
- **Agent-panel communication**: Node agent POSTs status to `/api/node/push`, polls tasks from `/api/node/tasks/poll`
- **Background workers**: Started in `main.go` as goroutines (expiry checker, billing, cert rotation, session enforcer)
- **Auth pattern**: Cookie-based sessions with HMAC-signed tokens; separate cookies for admin vs customer
- **API style**: JSON REST endpoints under `/api/`, prefixed by role (`/api/admin/`, `/api/portal/`)
