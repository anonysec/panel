# KorisPanel Multi-Stage Dockerfile
# Builds Go binary + frontend assets from source

# ─── Build Stage ──────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git nodejs npm

WORKDIR /build

# Go module dependencies (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Install shared dependencies (admin/portal import from shared)
WORKDIR /build/panel/web/shared
RUN npm install

# Build frontend - Admin
WORKDIR /build/panel/web/admin
RUN npm install && npm run build

# Build frontend - Portal
WORKDIR /build/panel/web/portal
RUN npm install && npm run build

# Build frontend - Landing
WORKDIR /build/panel/web/landing
RUN npm install && npm run build

# Build Go binary
WORKDIR /build
ARG BUILD_TAGS=""
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" ${BUILD_TAGS:+-tags $BUILD_TAGS} -o /koris ./panel/cmd/panel

# ─── Runtime Stage ────────────────────────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary
COPY --from=builder /koris /app/koris

# Copy migrations (PostgreSQL default + MySQL legacy)
COPY --from=builder /build/panel/migrations /app/migrations

EXPOSE 2026

HEALTHCHECK --interval=10s --timeout=5s --retries=5 --start-period=10s \
    CMD wget --no-check-certificate -q --spider https://localhost:2026/api/health || exit 1

ENTRYPOINT ["/app/koris"]
