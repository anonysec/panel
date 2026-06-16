# Technical Design Document: Panel Major Update

## Overview

This design covers a broad set of enhancements to KorisPanel across three layers: the Go Panel API backend, the Vue 3 Admin Dashboard and Customer Portal frontends, and the Go Node Agent binary. The update introduces user templates, bulk actions, traffic management, connection limits, data warnings, portal UX improvements, node agent operational maturity (auto-update, hot-reload, structured logging), and cross-cutting quality improvements (null safety, stale data prevention, error handling).

## Architecture

### System Context

```
┌─────────────────────────────────────────────────────────────────┐
│                         Panel Server                             │
│  ┌───────────┐   ┌──────────────┐   ┌───────────────────────┐  │
│  │  Nginx    │──▶│  Panel API   │──▶│  MariaDB              │  │
│  │  :80/443  │   │  (Go :8080)  │   │  (radius + panel DB)  │  │
│  └───────────┘   └──────┬───────┘   └───────────────────────┘  │
│                          │                                       │
│                          ▼                                       │
│                   ┌──────────────┐                               │
│                   │ FreeRADIUS   │                               │
│                   │ (radcheck)   │                               │
│                   └──────────────┘                               │
└─────────────────────────────────────────────────────────────────┘
              │  REST + WebSocket
              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Node Server(s)                              │
│  ┌──────────────────────────────────────────────────┐           │
│  │  Node Agent (Go binary)                          │           │
│  │  - Structured JSON logging                       │           │
│  │  - Config hot-reload (SIGHUP / task)             │           │
│  │  - Auto-update with checksum verification        │           │
│  │  - Diagnostics push                              │           │
│  └──────────────────────────────────────────────────┘           │
└─────────────────────────────────────────────────────────────────┘
```

### Component Boundaries

| Component | Language | Location | Responsibility |
|-----------|----------|----------|----------------|
| Panel API | Go 1.22+ | `panel/internal/api/` | REST endpoints, business logic, DB access |
| Admin Dashboard | Vue 3 + TS | `panel/web/admin/` | Admin SPA (Pinia stores, views) |
| Customer Portal | Vue 3 + TS | `panel/web/portal/` | Customer SPA (usage, self-service) |
| Node Agent | Go 1.22+ | `node/cmd/node/` | Metrics push, task execution, self-management |


## Data Models

### New Database Tables

#### `user_templates`

```sql
CREATE TABLE IF NOT EXISTS user_templates (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL UNIQUE,
  plan_id BIGINT NULL,
  status ENUM('active','disabled') NOT NULL DEFAULT 'active',
  connection_limit INT NOT NULL DEFAULT 0,
  radius_checks JSON NULL,       -- Array of {attribute, op, value}
  radius_replies JSON NULL,      -- Array of {attribute, op, value}
  created_by VARCHAR(64) NOT NULL,
  deleted_at DATETIME NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX(deleted_at)
);
```

#### `data_warning_thresholds` (stored in `panel_settings`)

```sql
-- Uses existing panel_settings table
-- Keys: 'data_warning_thresholds' → JSON array e.g. [80, 95]
INSERT IGNORE INTO panel_settings(setting_key, setting_value)
VALUES ('data_warning_thresholds', '[80, 95]');
```

#### `node_diagnostics`

```sql
CREATE TABLE IF NOT EXISTS node_diagnostics (
  node_id BIGINT PRIMARY KEY,
  agent_version VARCHAR(32) NOT NULL DEFAULT '',
  uptime_seconds BIGINT NOT NULL DEFAULT 0,
  go_version VARCHAR(32) NOT NULL DEFAULT '',
  goroutines INT NOT NULL DEFAULT 0,
  mem_alloc_bytes BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

#### `agent_releases`

```sql
CREATE TABLE IF NOT EXISTS agent_releases (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  version VARCHAR(32) NOT NULL UNIQUE,
  binary_path VARCHAR(512) NOT NULL,
  checksum_sha256 VARCHAR(64) NOT NULL,
  released_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```


### Go Structs

```go
// panel/internal/api/ — new types

type UserTemplate struct {
    ID              int64            `json:"id"`
    Name            string           `json:"name"`
    PlanID          *int64           `json:"plan_id"`
    Status          string           `json:"status"`
    ConnectionLimit int              `json:"connection_limit"`
    RadiusChecks    json.RawMessage  `json:"radius_checks"`
    RadiusReplies   json.RawMessage  `json:"radius_replies"`
    CreatedBy       string           `json:"created_by"`
    DeletedAt       *string          `json:"deleted_at"`
    CreatedAt       string           `json:"created_at"`
    UpdatedAt       string           `json:"updated_at"`
}

type BulkActionRequest struct {
    CustomerIDs []int64 `json:"customer_ids"`
    Action      string  `json:"action"` // "enable", "disable", "delete", "traffic_reset"
}

type BulkActionResponse struct {
    OK        bool              `json:"ok"`
    Succeeded []int64           `json:"succeeded"`
    Failed    []BulkFailure     `json:"failed"`
}

type BulkFailure struct {
    CustomerID int64  `json:"customer_id"`
    Error      string `json:"error"`
}

type DiagnosticsReport struct {
    AgentVersion  string `json:"agent_version"`
    UptimeSeconds int64  `json:"uptime_seconds"`
    GoVersion     string `json:"go_version"`
    Goroutines    int    `json:"goroutines"`
    MemAllocBytes int64  `json:"mem_alloc_bytes"`
}

type AgentVersionResponse struct {
    OK       bool   `json:"ok"`
    Version  string `json:"version"`
    URL      string `json:"url"`
    Checksum string `json:"checksum_sha256"`
}

type ErrorResponse struct {
    Error  string `json:"error"`
    Code   string `json:"code"`
    Status int    `json:"status"`
}
```


### TypeScript Types (Frontend)

```typescript
// Admin Dashboard & Portal shared types

interface UserTemplate {
  id: number
  name: string
  plan_id: number | null
  status: string
  connection_limit: number
  radius_checks: RadiusAttribute[]
  radius_replies: RadiusAttribute[]
  created_by: string
  deleted_at: string | null
  created_at: string
  updated_at: string
}

interface RadiusAttribute {
  attribute: string
  op: string
  value: string
}

interface BulkActionRequest {
  customer_ids: number[]
  action: 'enable' | 'disable' | 'delete' | 'traffic_reset'
}

interface BulkActionResponse {
  ok: boolean
  succeeded: number[]
  failed: { customer_id: number; error: string }[]
}

interface UsageDisplayData {
  usedBytes: number
  capBytes: number
  usedPercent: number
  remainingBytes: number
  remainingFormatted: string
  capFormatted: string
  progressColor: 'green' | 'amber' | 'red'
  expiresAt: string
  daysRemaining: number
}

interface DiagnosticsReport {
  agent_version: string
  uptime_seconds: number
  go_version: string
  goroutines: number
  mem_alloc_bytes: number
}
```


## API Interfaces

### New Panel API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/api/templates` | Admin | List all non-deleted user templates |
| POST | `/api/templates` | Admin | Create user template |
| PATCH | `/api/templates/{id}` | Admin | Update user template |
| DELETE | `/api/templates/{id}` | Admin | Soft-delete user template |
| POST | `/api/customers/bulk` | Admin | Execute bulk action on customers |
| POST | `/api/customers/{id}/traffic-reset` | Admin | Reset single customer traffic |
| POST | `/api/customers/{id}/connection-limit` | Admin | Set connection limit |
| GET | `/api/node/agent/version` | Node Token | Get latest agent version info |
| GET | `/api/node/agent/download` | Node Token | Download agent binary |
| GET | `/api/portal/warnings` | Customer | Get active data warnings |

### User Templates CRUD

```go
// POST /api/templates
func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request) {
    // Validate: name unique, plan_id exists if set
    // Insert into user_templates
    // Return: {ok: true, template: UserTemplate}
}

// PATCH /api/templates/{id}
func (s *Server) updateTemplate(w http.ResponseWriter, r *http.Request) {
    // Validate: template exists and not soft-deleted
    // Update fields
    // Return: {ok: true, template: UserTemplate}
}

// DELETE /api/templates/{id}
func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
    // SET deleted_at = NOW() WHERE id = ? AND deleted_at IS NULL
    // Return: {ok: true}
}
```

### Bulk Actions

```go
// POST /api/customers/bulk
func (s *Server) customersBulk(w http.ResponseWriter, r *http.Request) {
    // 1. Decode BulkActionRequest
    // 2. Validate len(customer_ids) <= 200
    // 3. For each customer_id, apply action in a loop (not transaction per-item)
    // 4. Collect successes and failures
    // 5. Return BulkActionResponse
}
```

### Traffic Reset

```go
// POST /api/customers/{id}/traffic-reset
func (s *Server) trafficReset(w http.ResponseWriter, r *http.Request) {
    // 1. Zero radacct counters for current period:
    //    UPDATE radacct SET acctinputoctets=0, acctoutputoctets=0
    //    WHERE username=? AND acctstoptime IS NULL
    // 2. Insert wallet_transaction (type='adjustment', desc='Traffic reset')
    // 3. Insert audit_log entry
    // 4. If customer.status == 'limited', UPDATE to 'active'
    // Return: {ok: true}
}
```

### Connection Limit

```go
// POST /api/customers/{id}/connection-limit
func (s *Server) setConnectionLimit(w http.ResponseWriter, r *http.Request) {
    // Input: {limit: int}
    // If limit == 0: DELETE FROM radcheck WHERE username=? AND attribute='Simultaneous-Use'
    // If limit > 0:  REPLACE INTO radcheck (username, attribute, op, value)
    //               VALUES (?, 'Simultaneous-Use', ':=', ?)
    // Return: {ok: true, connection_limit: limit}
}
```


### Data Usage Warning Processing

```go
// Called during radacct accounting updates or via periodic job
func (s *Server) checkDataWarnings(username string) {
    // 1. Query customer's total usage (input_octets + output_octets)
    // 2. Query customer's plan data cap (plans.data_gb * 1024^3)
    // 3. Load thresholds from panel_settings ('data_warning_thresholds')
    // 4. For each threshold crossed that hasn't been warned:
    //    a. INSERT INTO events (type='data_warning', severity='warning', ...)
    //    b. Dispatch notification via Notifier
    // 5. If usage >= 100% cap:
    //    a. UPDATE customers SET status='limited' WHERE username=?
    //    b. INSERT INTO events (type='data_cap_reached', severity='error', ...)
}
```

### Agent Version Endpoint

```go
// GET /api/node/agent/version
func (s *Server) agentVersion(w http.ResponseWriter, r *http.Request) {
    // Auth: X-Node-Token header
    // Query: SELECT version, binary_path, checksum_sha256
    //        FROM agent_releases ORDER BY released_at DESC LIMIT 1
    // Return: AgentVersionResponse
}
```

### Error Response Middleware

```go
// writeError standardizes all error responses
func writeError(w http.ResponseWriter, status int, code, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Cache-Control", "no-store")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(ErrorResponse{
        Error:  message,
        Code:   code,
        Status: status,
    })
}
```

### Cache-Control Middleware

```go
// Applied to all /api/ routes
func noCacheMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if strings.HasPrefix(r.URL.Path, "/api/") {
            w.Header().Set("Cache-Control", "no-store")
        }
        next.ServeHTTP(w, r)
    })
}
```


## Node Agent Design

### Structured Logging

Replace all `log.Printf` calls with a structured logger:

```go
// node/internal/logger/logger.go
package logger

import (
    "encoding/json"
    "io"
    "os"
    "sync"
    "time"
)

type Level int

const (
    LevelDebug Level = iota
    LevelInfo
    LevelWarn
    LevelError
)

type Logger struct {
    mu     sync.Mutex
    out    io.Writer
    level  Level
    fields map[string]any
}

type LogEntry struct {
    Timestamp string         `json:"timestamp"`
    Level     string         `json:"level"`
    Message   string         `json:"message"`
    Fields    map[string]any `json:"fields,omitempty"`
}

func New(level Level) *Logger {
    return &Logger{out: os.Stdout, level: level, fields: make(map[string]any)}
}

func (l *Logger) Info(msg string, fields ...map[string]any) {
    l.log(LevelInfo, msg, fields...)
}

func (l *Logger) Warn(msg string, fields ...map[string]any) {
    l.log(LevelWarn, msg, fields...)
}

func (l *Logger) Error(msg string, fields ...map[string]any) {
    l.log(LevelError, msg, fields...)
}

func (l *Logger) Debug(msg string, fields ...map[string]any) {
    l.log(LevelDebug, msg, fields...)
}

func (l *Logger) log(level Level, msg string, fields ...map[string]any) {
    if level < l.level {
        return
    }
    entry := LogEntry{
        Timestamp: time.Now().UTC().Format(time.RFC3339),
        Level:     levelString(level),
        Message:   msg,
    }
    if len(fields) > 0 {
        entry.Fields = fields[0]
    }
    l.mu.Lock()
    defer l.mu.Unlock()
    json.NewEncoder(l.out).Encode(entry)
}

func (l *Logger) SetLevel(level Level) {
    l.mu.Lock()
    defer l.mu.Unlock()
    l.level = level
}
```


### Config Hot-Reload

```go
// node/internal/config/config.go
package config

import (
    "os"
    "strconv"
    "strings"
    "sync"
)

type Config struct {
    mu            sync.RWMutex
    PanelURL      string
    NodeToken     string // Not hot-reloadable
    Interval      int    // seconds
    AutoUpdate    bool
    LogLevel      string
}

var reloadableKeys = map[string]bool{
    "PANEL_URL":        true,
    "NODE_INTERVAL":    true,
    "NODE_AUTO_UPDATE": true,
    "LOG_LEVEL":        true,
}

func (c *Config) Reload(envFile string) (changes map[string][2]string, err error) {
    // 1. Read and parse envFile (KEY=VALUE lines)
    // 2. Validate required keys present
    // 3. For each reloadable key, compare old vs new
    // 4. Record changes as map[key] → [old, new]
    // 5. Apply new values under write lock
    // Returns changes for logging, or error if validation fails
}

func (c *Config) IsReloadable(key string) bool {
    return reloadableKeys[key]
}
```

### Auto-Update

```go
// node/internal/updater/updater.go
package updater

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "io"
    "net/http"
    "os"
)

type Updater struct {
    panelURL       string
    nodeToken      string
    currentVersion string
    client         *http.Client
}

func (u *Updater) CheckAndUpdate() error {
    // 1. GET {panelURL}/api/node/agent/version
    // 2. Compare response.Version with currentVersion
    // 3. If newer: download binary from response.URL
    // 4. Compute SHA-256 of downloaded bytes
    // 5. Compare with response.Checksum
    // 6. If mismatch: discard, log error, return
    // 7. If match: write to temp file, chmod +x, rename over current binary
    // 8. Exec "systemctl restart node-agent"
    return nil
}

func (u *Updater) VerifyChecksum(data []byte, expected string) bool {
    h := sha256.Sum256(data)
    actual := hex.EncodeToString(h[:])
    return actual == expected
}

func CompareVersions(current, remote string) bool {
    // Semantic version comparison: returns true if remote > current
    // Format: "v1.2.3" — compare major.minor.patch numerically
}
```


### Signal Handling for Hot-Reload

```go
// In node/cmd/node/main.go — add signal handler
func setupSignalHandler(cfg *config.Config, logger *logger.Logger) {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGHUP)
    go func() {
        for range sigCh {
            changes, err := cfg.Reload("/etc/panel-node/node.env")
            if err != nil {
                logger.Error("config reload failed", map[string]any{
                    "error": err.Error(),
                })
                continue
            }
            for key, vals := range changes {
                logger.Info("config reloaded", map[string]any{
                    "key":       key,
                    "old_value": vals[0],
                    "new_value": vals[1],
                })
            }
        }
    }()
}
```

### Diagnostics in Status Push

```go
// Extended Push struct for diagnostics
type Push struct {
    // ... existing fields ...
    Diagnostics *DiagnosticsReport `json:"diagnostics,omitempty"`
}

type DiagnosticsReport struct {
    AgentVersion  string `json:"agent_version"`
    UptimeSeconds int64  `json:"uptime_seconds"`
    GoVersion     string `json:"go_version"`
    Goroutines    int    `json:"goroutines"`
    MemAllocBytes int64  `json:"mem_alloc_bytes"`
}

func buildDiagnostics(startTime time.Time, version string) *DiagnosticsReport {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    return &DiagnosticsReport{
        AgentVersion:  version,
        UptimeSeconds: int64(time.Since(startTime).Seconds()),
        GoVersion:     runtime.Version(),
        Goroutines:    runtime.NumGoroutine(),
        MemAllocBytes: int64(m.Alloc),
    }
}
```


## Frontend Architecture

### Admin Dashboard — New Stores

#### `stores/templates.ts`

```typescript
// Pinia Composition API store for user templates
export const useTemplatesStore = defineStore('templates', () => {
  const list = ref<UserTemplate[]>([])
  const loading = ref(false)
  const { get, post, patch, del } = useApi()

  async function loadTemplates(): Promise<void> { /* GET /api/templates */ }
  async function createTemplate(payload: Partial<UserTemplate>): Promise<boolean> { /* POST /api/templates */ }
  async function updateTemplate(id: number, payload: Partial<UserTemplate>): Promise<boolean> { /* PATCH /api/templates/{id} */ }
  async function deleteTemplate(id: number): Promise<boolean> { /* DELETE /api/templates/{id} */ }

  return { list, loading, loadTemplates, createTemplate, updateTemplate, deleteTemplate }
})
```

#### `stores/customers.ts` — Extensions

```typescript
// Add to existing useCustomersStore
async function bulkAction(request: BulkActionRequest): Promise<BulkActionResponse | null> {
  // POST /api/customers/bulk
  // On success: refetch customer list
}

async function trafficReset(customerId: number): Promise<boolean> {
  // POST /api/customers/{id}/traffic-reset
  // On success: refetch customer detail
}

async function setConnectionLimit(customerId: number, limit: number): Promise<boolean> {
  // POST /api/customers/{id}/connection-limit
  // On success: refetch customer detail
}
```

### Stale Data Prevention — Composable

```typescript
// composables/useFreshData.ts
import { ref, onMounted, onActivated } from 'vue'

const STALE_THRESHOLD_MS = 30_000 // 30 seconds

export function useFreshData(fetcher: () => Promise<void>) {
  const lastFetchedAt = ref<number>(0)

  async function ensureFresh(): Promise<void> {
    const now = Date.now()
    if (now - lastFetchedAt.value > STALE_THRESHOLD_MS) {
      await fetcher()
      lastFetchedAt.value = now
    }
  }

  onMounted(ensureFresh)
  onActivated(ensureFresh)

  return { ensureFresh, lastFetchedAt }
}
```

### Error Handling — Global Interceptor

```typescript
// composables/useApi.ts — enhancement
export function useApi(options?: { onUnauthorized?: () => void }) {
  // Existing fetch wrapper enhanced with:
  // 1. On 401: call onUnauthorized → redirect to login, clear session
  // 2. On any error: emit toast event with error.message from response
  // 3. On success mutation: emit 'invalidate' event for resource
}
```


### Customer Portal — Usage Display Component

```typescript
// composables/useUsageDisplay.ts
export function useUsageDisplay(usedBytes: number, capBytes: number, expiresAt: string) {
  const usedPercent = capBytes > 0 ? (usedBytes / capBytes) * 100 : 0
  const remainingBytes = Math.max(0, capBytes - usedBytes)
  const remainingPercent = 100 - usedPercent

  const progressColor = computed(() => {
    if (remainingPercent <= 5) return 'red'
    if (remainingPercent <= 20) return 'amber'
    return 'green'
  })

  const daysRemaining = computed(() => {
    const now = new Date()
    const expiry = new Date(expiresAt)
    const diff = expiry.getTime() - now.getTime()
    return Math.max(0, Math.ceil(diff / (1000 * 60 * 60 * 24)))
  })

  return { usedPercent, remainingBytes, progressColor, daysRemaining }
}

// Formatting utility
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const k = 1024
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${units[i]}`
}
```

### Null-Safety Rendering Utility

```typescript
// utils/nullSafe.ts
export function displayValue(value: unknown, fallback = '—'): string {
  if (value === null || value === undefined || value === '') {
    return fallback
  }
  return String(value)
}

// Usage in templates:
// {{ displayValue(customer.display_name) }}
```


## Error Handling Strategy

### Panel API Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `bad_request` | 400 | Malformed input or validation failure |
| `bulk_limit_exceeded` | 400 | Bulk action exceeds 200 item limit |
| `duplicate_name` | 409 | Template name already exists |
| `not_found` | 404 | Resource does not exist |
| `unauthorized` | 401 | Missing or invalid session |
| `forbidden` | 403 | Insufficient permissions |
| `internal_error` | 500 | Unexpected server error |

### 5xx Logging Structure

```go
// All 500 errors logged with context
func (s *Server) logServerError(r *http.Request, err error) {
    username, _, _ := s.currentAdmin(r)
    requestID := r.Header.Get("X-Request-ID")
    log.Printf(`{"level":"error","path":"%s","method":"%s","user":"%s","request_id":"%s","error":"%s"}`,
        r.URL.Path, r.Method, username, requestID, err.Error())
}
```

### Node Agent Error Response Logging

```go
// In postJSON — enhanced error handling
func postJSON(client *http.Client, url, token string, v any, logger *Logger) {
    // ... existing logic ...
    if resp.StatusCode/100 != 2 {
        body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
        logger.Warn("non-2xx response from panel", map[string]any{
            "url":    url,
            "status": resp.StatusCode,
            "body":   string(body),
        })
    }
}
```

## Null-Safety Strategy (Backend)

### Scanning Pattern

```go
// Before (unsafe — panics on NULL):
var displayName string
err := row.Scan(&displayName)

// After (safe):
var displayName sql.NullString
err := row.Scan(&displayName)

// JSON serialization with null preservation:
type Customer struct {
    DisplayName *string `json:"display_name"` // JSON null if nil
}

func nullStringPtr(ns sql.NullString) *string {
    if !ns.Valid {
        return nil
    }
    return &ns.String
}
```


## Migration Plan

New migration file: `panel/migrations/018_major_update.sql`

```sql
-- 018_major_update.sql

CREATE TABLE IF NOT EXISTS user_templates (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL UNIQUE,
  plan_id BIGINT NULL,
  status ENUM('active','disabled') NOT NULL DEFAULT 'active',
  connection_limit INT NOT NULL DEFAULT 0,
  radius_checks JSON NULL,
  radius_replies JSON NULL,
  created_by VARCHAR(64) NOT NULL,
  deleted_at DATETIME NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX(deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS node_diagnostics (
  node_id BIGINT PRIMARY KEY,
  agent_version VARCHAR(32) NOT NULL DEFAULT '',
  uptime_seconds BIGINT NOT NULL DEFAULT 0,
  go_version VARCHAR(32) NOT NULL DEFAULT '',
  goroutines INT NOT NULL DEFAULT 0,
  mem_alloc_bytes BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS agent_releases (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  version VARCHAR(32) NOT NULL UNIQUE,
  binary_path VARCHAR(512) NOT NULL,
  checksum_sha256 VARCHAR(64) NOT NULL,
  released_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Add data warning thresholds to panel_settings
INSERT IGNORE INTO panel_settings(setting_key, setting_value)
VALUES ('data_warning_thresholds', '[80, 95]');

-- Add speed_mbps to plans if not present (for Requirement 1 template pre-population)
ALTER TABLE plans ADD COLUMN IF NOT EXISTS speed_mbps DECIMAL(8,2) NOT NULL DEFAULT 0 AFTER data_gb;
```


## Sequence Diagrams

### Bulk Action Flow

```
Admin Dashboard                Panel API                    MariaDB
     │                            │                            │
     │  POST /api/customers/bulk  │                            │
     │  {customer_ids, action}    │                            │
     │───────────────────────────▶│                            │
     │                            │  Validate len <= 200       │
     │                            │                            │
     │                            │  FOR each customer_id:     │
     │                            │    UPDATE customers...     │
     │                            │───────────────────────────▶│
     │                            │◀───────────────────────────│
     │                            │    Record success/failure  │
     │                            │                            │
     │  {ok, succeeded, failed}   │                            │
     │◀───────────────────────────│                            │
     │                            │                            │
     │  GET /api/customers        │  (refetch list)            │
     │───────────────────────────▶│                            │
```

### Node Agent Auto-Update Flow

```
Node Agent                   Panel API                    Filesystem
     │                            │                            │
     │  GET /api/node/agent/ver   │                            │
     │───────────────────────────▶│                            │
     │  {version, url, checksum}  │                            │
     │◀───────────────────────────│                            │
     │                            │                            │
     │  [if remote > local]       │                            │
     │  GET /api/node/agent/dl    │                            │
     │───────────────────────────▶│                            │
     │  [binary bytes]            │                            │
     │◀───────────────────────────│                            │
     │                            │                            │
     │  SHA-256(binary) == chk?   │                            │
     │  [if yes]                  │                            │
     │                            │      Write temp binary     │
     │────────────────────────────┼───────────────────────────▶│
     │                            │      Rename over self      │
     │────────────────────────────┼───────────────────────────▶│
     │                            │                            │
     │  systemctl restart node-agent                           │
```

### Data Usage Warning Flow

```
FreeRADIUS (Accounting)      Panel API                    MariaDB
     │                            │                            │
     │  POST /api/radius/acct     │                            │
     │───────────────────────────▶│                            │
     │                            │  checkDataWarnings(user)   │
     │                            │                            │
     │                            │  SELECT SUM(octets)        │
     │                            │───────────────────────────▶│
     │                            │◀───────────────────────────│
     │                            │                            │
     │                            │  [if usage >= threshold]   │
     │                            │  INSERT events (warning)   │
     │                            │───────────────────────────▶│
     │                            │                            │
     │                            │  Notify (Telegram/Email)   │
     │                            │                            │
     │                            │  [if usage >= 100%]        │
     │                            │  UPDATE status='limited'   │
     │                            │───────────────────────────▶│
```


## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system—essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Template CRUD Round-Trip

*For any* valid user template payload (with a unique name, optional plan_id, status, connection_limit, and RADIUS attributes), creating the template and then fetching it by ID SHALL return a record with all fields matching the original payload.

**Validates: Requirements 1.2, 1.3**

### Property 2: Template Soft-Delete Isolation

*For any* user template that has been used to create customers, soft-deleting the template SHALL NOT modify any field of the customer records that were previously created from that template.

**Validates: Requirements 1.4**

### Property 3: Template Name Uniqueness

*For any* template name, attempting to create a second template with the same name (case-insensitive) SHALL be rejected with an error, regardless of whether the first template is active or soft-deleted.

**Validates: Requirements 1.5**

### Property 4: Template Pre-Population Completeness

*For any* user template, creating a customer using that template SHALL produce a customer record where every non-null field defined in the template (plan_id, status, connection_limit, RADIUS checks, RADIUS replies) appears identically in the resulting customer's configuration.

**Validates: Requirements 1.6**

### Property 5: Bulk Action Status Application

*For any* set of up to 200 valid customer IDs and any bulk action ("enable", "disable", or "delete"), after the action completes, every customer in the succeeded list SHALL have the target status ("active", "disabled", or "deleted" with deleted_at set, respectively).

**Validates: Requirements 2.2, 2.3, 2.4**

### Property 6: Bulk Action Size Limit Enforcement

*For any* bulk action request containing more than 200 customer IDs, the Panel API SHALL reject the request with HTTP 400 status code without modifying any customer record.

**Validates: Requirements 2.5**

### Property 7: Bulk Action Partial Failure Reporting

*For any* bulk action request containing a mix of valid and invalid customer IDs, the response SHALL contain a succeeded list and a failed list where: (a) every valid ID appears in exactly one list, (b) every failed entry includes a non-empty error reason, and (c) the union of succeeded and failed equals the input set.

**Validates: Requirements 2.7**


### Property 8: Traffic Reset Zeroes Counters and Creates Audit Trail

*For any* customer with non-zero accumulated traffic, performing a traffic reset SHALL result in the customer's current-period input_octets and output_octets both being zero, AND a wallet_transaction of type "adjustment" with description containing "traffic reset" SHALL exist, AND an audit_log entry for the action SHALL exist.

**Validates: Requirements 3.1, 3.2**

### Property 9: Traffic Reset Restores Limited Status

*For any* customer whose status is "limited" (due to data cap exhaustion), performing a traffic reset SHALL change the customer's status to "active".

**Validates: Requirements 3.3**

### Property 10: Connection Limit RADIUS Attribute Mapping

*For any* positive integer connection limit value set on a customer, the radcheck table SHALL contain exactly one Simultaneous-Use entry for that customer with operator ":=" and the value matching the limit. Conversely, *for any* customer whose connection limit is set to zero, no Simultaneous-Use entry SHALL exist in the radcheck table for that customer.

**Validates: Requirements 4.1, 4.4**

### Property 11: Data Threshold Event Creation

*For any* customer whose accumulated traffic crosses a configured warning threshold percentage of their plan's data cap, an event record of type "data_warning" with severity "warning" SHALL be created. Furthermore, *for any* customer whose traffic reaches or exceeds 100% of their data cap, the customer's status SHALL be set to "limited" and an event SHALL be logged.

**Validates: Requirements 5.2, 5.6**

### Property 12: Usage Display Calculations

*For any* pair of (usedBytes, capBytes) where capBytes > 0, the computed usedPercent SHALL equal (usedBytes / capBytes) × 100, the remainingBytes SHALL equal max(0, capBytes − usedBytes), and the formatted remaining string SHALL represent remainingBytes in the appropriate human-readable unit (B, KB, MB, GB, TB). Additionally, *for any* expiresAt date in the future, daysRemaining SHALL equal the ceiling of the difference in days between expiresAt and the current time.

**Validates: Requirements 6.1, 6.2, 6.3**

### Property 13: Usage Progress Bar Color Classification

*For any* usage state where remaining data is less than or equal to 5% of the plan cap, the progress bar color SHALL be "red". *For any* usage state where remaining data is greater than 5% but less than or equal to 20% of the plan cap, the progress bar color SHALL be "amber". *For any* usage state where remaining data is greater than 20%, the progress bar color SHALL be "green".

**Validates: Requirements 6.4, 6.5**


### Property 14: Version Comparison Triggers Download

*For any* pair of semantic version strings (current, remote) where remote is numerically greater than current (comparing major, then minor, then patch), the auto-update logic SHALL determine that an update is available. Conversely, *for any* pair where remote is less than or equal to current, no download SHALL be triggered.

**Validates: Requirements 7.2**

### Property 15: Binary Checksum Verification

*For any* byte sequence and its correct SHA-256 checksum, the verification function SHALL return true. *For any* byte sequence paired with an incorrect checksum (differing by at least one character), the verification function SHALL return false and the binary SHALL be discarded.

**Validates: Requirements 7.3, 7.4**

### Property 16: Config Hot-Reload With Validation

*For any* valid configuration file change affecting reloadable keys (PANEL_URL, NODE_INTERVAL, NODE_AUTO_UPDATE, LOG_LEVEL), reloading SHALL update the running configuration to the new values and log entries SHALL contain both the previous and new value for each changed key. *For any* malformed configuration file (missing required keys or unparseable values), reloading SHALL retain all previous configuration values unchanged and log an error.

**Validates: Requirements 8.3, 8.4**

### Property 17: Structured Log Format

*For any* log message emitted by the Node Agent, the output SHALL be a valid JSON object containing the fields "timestamp" (ISO 8601 format), "level" (one of "debug", "info", "warn", "error"), and "message" (non-empty string).

**Validates: Requirements 9.1**

### Property 18: Diagnostics Report Completeness

*For any* status push sent by the Node Agent, the diagnostics field SHALL contain: agent_version (non-empty string), uptime_seconds (non-negative integer), go_version (non-empty string matching "go1.x.y" pattern), goroutines (positive integer), and mem_alloc_bytes (non-negative integer).

**Validates: Requirements 9.4**

### Property 19: Nullable JSON Serialization

*For any* database record containing NULL values in nullable columns, the Panel API JSON response SHALL include those fields with the JSON value `null` (not omitted from the response object), ensuring the response schema is consistent regardless of data presence.

**Validates: Requirements 10.2**

### Property 20: Null Field Fallback Rendering

*For any* API response field that arrives as JSON `null`, the Admin Dashboard and Customer Portal SHALL render a fallback display value (e.g., "—" or "N/A") rather than displaying "null", "undefined", or an empty string.

**Validates: Requirements 10.3, 10.4**


### Property 21: Stale Cache Invalidation

*For any* navigation event in the Admin Dashboard or Customer Portal where the cached data for the target view is older than 30 seconds, the application SHALL issue a fresh API request to the server rather than displaying the cached data.

**Validates: Requirements 11.1, 11.2**

### Property 22: Mutation-Triggered Refetch

*For any* successful mutation API call (create, update, or delete) in the Admin Dashboard or Customer Portal, the application SHALL immediately invalidate the cache for the affected resource and issue a fresh fetch request, such that subsequent reads reflect the mutation.

**Validates: Requirements 11.3, 11.4**

### Property 23: Cache-Control Header Presence

*For any* HTTP response from the Panel API with Content-Type "application/json", the response headers SHALL include "Cache-Control: no-store".

**Validates: Requirements 11.5**

### Property 24: Error Response Schema

*For any* error response returned by the Panel API (4xx or 5xx status codes), the response body SHALL be valid JSON containing all three fields: "error" (non-empty string with human-readable message), "code" (non-empty string with machine-readable code), and "status" (integer matching the HTTP status code of the response).

**Validates: Requirements 12.1**

### Property 25: Error Toast Display

*For any* failed API request in the Admin Dashboard or Customer Portal, the application SHALL display a toast notification containing the error message extracted from the response body's "error" field.

**Validates: Requirements 12.3, 12.4**

### Property 26: 401 Session Invalidation

*For any* API response with HTTP status 401 received by the Admin Dashboard or Customer Portal, the application SHALL redirect the user to the login view AND clear all stored session state (cookies, Pinia auth store).

**Validates: Requirements 12.5, 12.6**

### Property 27: Non-2xx Response Logging With Truncation

*For any* non-2xx HTTP response received by the Node Agent from the Panel API, the agent SHALL log at "warn" level a JSON entry containing the request URL, the response status code, and the response body truncated to at most 512 bytes.

**Validates: Requirements 12.7**
