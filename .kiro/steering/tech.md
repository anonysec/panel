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

## Rules

- No external router. No ORM. No heavy frameworks.
- Parameterized SQL only. Never concatenate user input.
- `<script setup lang="ts">` for all Vue components.
- Migrations: `NNN_desc.sql`, forward-only, use IF NOT EXISTS.
- Memory: GOMAXPROCS(1), GOGC=50, GOMEMLIMIT=100MB. No unbounded collections.
- Execute immediately. Don't ask for confirmation. Fix errors and continue.
