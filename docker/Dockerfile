# KorisPanel Multi-Stage Dockerfile
# Stage 1: Build Go binary and frontend assets
# Stage 2: Minimal Alpine runtime

# ─── Build Stage ──────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git nodejs npm

WORKDIR /build

# Go module dependencies (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Install shared dependencies first (admin/portal import from shared)
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
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /panel ./panel/cmd/panel

# ─── Runtime Stage ────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1000 panel && adduser -u 1000 -G panel -s /bin/sh -D panel

# Create required directories
RUN mkdir -p /app/migrations /app/www /var/log/panel /var/run /etc/panel && \
    chown -R panel:panel /app /var/log/panel /var/run /etc/panel

WORKDIR /app

# Copy binary
COPY --from=builder /panel /app/panel

# Copy migrations
COPY --from=builder /build/panel/migrations /app/migrations

# Switch to non-root user
USER panel

EXPOSE 2026

HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
    CMD wget -q --spider http://localhost:2026/api/health || exit 1

ENTRYPOINT ["/app/panel"]
