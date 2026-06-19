---
inclusion: always
---

# Dev Rules

## Stack

Go 1.22 (`KorisPanel` module) | net/http ServeMux | mysql driver | gorilla/websocket | bcrypt
Vue 3 Composition API | Vite 5 | Pinia | Vue Router 4 | @vueuse/core
Vitest + happy-dom + fast-check | go-sqlmock

## Commands

```bash
go build -o /usr/local/bin/panel ./panel/cmd/panel
go test ./...
cd panel/web/admin && npm run build && npm run test
./deploy.sh
```

## Deployment

- Production server: `root@91.107.168.34`
- Panel address: `127.0.0.1:8088` (behind nginx reverse proxy)
- Push to main, then deploy via: `git archive --format=tar HEAD | ssh root@91.107.168.34 "cd /opt/KorisPanel && tar xf -"` followed by `ssh root@91.107.168.34 "cd /opt/KorisPanel && go build -o /usr/local/bin/panel ./panel/cmd/panel && systemctl restart panel.service"`
- GitHub is unreachable from the server (DNS blocked) — always use git archive + SSH pipe for deploys
- Git remote: `ssh://git@ssh.github.com:443/anonysec/panel.git` (port 443 for networks that block port 22)

## Rules

- No external router. No ORM. No heavy frameworks.
- Parameterized SQL only. Never concatenate user input.
- `<script setup lang="ts">` for all Vue components.
- Migrations: `NNN_desc.sql`, forward-only, use IF NOT EXISTS.
- Memory: GOMAXPROCS(1), GOGC=50, GOMEMLIMIT=100MB. No unbounded collections.
- Execute immediately. Don't ask for confirmation. Fix errors and continue.
- Use SSH protocol for all Git operations (clone, push, pull). Never use HTTPS for Git remotes.
- Always push to main. No feature branches.
- Commit changes on each update
