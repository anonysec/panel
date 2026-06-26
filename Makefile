# KorisPanel Makefile
# Build automation for frontend and backend

PANEL_BIN   := koris
WEB_DIR     := panel/web
GO_LDFLAGS  := -w -s

# ─── Frontend ─────────────────────────────────────────────────────────

.PHONY: frontend
frontend:
	cd $(WEB_DIR) && pnpm install --frozen-lockfile
	cd $(WEB_DIR) && pnpm --filter admin build
	cd $(WEB_DIR) && pnpm --filter portal build
	cd $(WEB_DIR) && pnpm --filter landing build

.PHONY: frontend-dev
frontend-dev:
	cd $(WEB_DIR) && pnpm install
	cd $(WEB_DIR) && pnpm --filter admin build
	cd $(WEB_DIR) && pnpm --filter portal build
	cd $(WEB_DIR) && pnpm --filter landing build

.PHONY: clean-frontend
clean-frontend:
	rm -rf $(WEB_DIR)/admin/www
	rm -rf $(WEB_DIR)/portal/www
	rm -rf $(WEB_DIR)/landing/www
	rm -rf $(WEB_DIR)/node_modules

# ─── Backend ──────────────────────────────────────────────────────────

.PHONY: backend
backend:
	CGO_ENABLED=0 go build -ldflags="$(GO_LDFLAGS)" -o $(PANEL_BIN) ./panel/cmd/panel

.PHONY: backend-lite
backend-lite:
	CGO_ENABLED=0 go build -tags lite -ldflags="$(GO_LDFLAGS)" -o $(PANEL_BIN) ./panel/cmd/panel

# ─── Combined ─────────────────────────────────────────────────────────

.PHONY: build
build: frontend backend

.PHONY: build-lite
build-lite: frontend backend-lite

# ─── Test ─────────────────────────────────────────────────────────────

.PHONY: test
test:
	go test ./panel/...

.PHONY: test-frontend
test-frontend:
	cd $(WEB_DIR) && pnpm test

# ─── Clean ────────────────────────────────────────────────────────────

.PHONY: clean
clean: clean-frontend
	rm -f $(PANEL_BIN)
