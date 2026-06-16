# Implementation Plan: Panel Major Update

## Overview

This implementation plan covers user templates management, bulk customer actions, traffic reset, connection limit enforcement, data usage warnings, portal self-service UX, node agent operational maturity (auto-update, config hot-reload, structured logging/diagnostics), and cross-cutting bug fixes (null safety, stale data, error handling). The plan is structured to build foundational patterns first (error handling, null safety, caching middleware), then layer feature-specific backend endpoints, then frontend views, and finally the node agent enhancements.

## Tasks

- [x] 1. Database migration and core backend patterns
  - [x] 1.1 Create migration file `panel/migrations/018_major_update.sql`
    - Create `user_templates` table with columns: id, name, plan_id, status, connection_limit, radius_checks (JSON), radius_replies (JSON), created_by, deleted_at, created_at, updated_at
    - Create `node_diagnostics` table with columns: node_id, agent_version, uptime_seconds, go_version, goroutines, mem_alloc_bytes, updated_at
    - Create `agent_releases` table with columns: id, version, binary_path, checksum_sha256, released_at
    - Insert default data_warning_thresholds '[80, 95]' into panel_settings
    - _Requirements: 1.2, 5.1, 7.6, 9.6_

  - [x] 1.2 Implement `writeError` helper and error response structs in `panel/internal/api/api.go`
    - Add `ErrorResponse` struct with fields: Error, Code, Status
    - Implement `writeError(w, status, code, message)` function that sets Content-Type, Cache-Control: no-store, writes status, and encodes ErrorResponse as JSON
    - Implement `logServerError(r, err)` that logs 5xx errors with path, method, user, request_id
    - _Requirements: 12.1, 12.2_

  - [x] 1.3 Implement `noCacheMiddleware` in `panel/internal/api/api.go`
    - Add middleware that sets `Cache-Control: no-store` on all `/api/` responses
    - Wire middleware into the main HTTP handler chain
    - _Requirements: 11.5_

  - [x] 1.4 Implement null-safety scanning helpers in `panel/internal/api/api.go`
    - Add `nullStringPtr(sql.NullString) *string` helper function
    - Add `nullInt64Ptr(sql.NullInt64) *int64` helper function
    - Add `nullTimePtr(sql.NullTime) *string` helper function
    - Ensure all nullable DB columns use sql.Null* types when scanned
    - Ensure JSON serialization outputs `null` (not omitted) for nil pointer fields
    - _Requirements: 10.1, 10.2_

- [x] 2. Checkpoint - Ensure migration and core patterns compile
  - Ensure all tests pass, ask the user if questions arise.

- [x] 3. User Templates API
  - [x] 3.1 Implement `UserTemplate` struct and templates CRUD handlers in `panel/internal/api/`
    - Add `UserTemplate` Go struct with JSON tags matching the design
    - Implement `func (s *Server) templates(w, r)` with method switch for GET (list all non-deleted)
    - Implement `func (s *Server) createTemplate(w, r)` for POST — validate unique name, insert, return template
    - Implement `func (s *Server) updateTemplate(w, r)` for PATCH `/api/templates/{id}` — validate exists & not deleted, update fields
    - Implement `func (s *Server) deleteTemplate(w, r)` for DELETE `/api/templates/{id}` — set deleted_at = NOW()
    - Register routes in the main router
    - _Requirements: 1.2, 1.3, 1.4, 1.5_

  - [x] 3.2 Implement template pre-population logic in existing customer creation handler
    - When a `template_id` field is provided in the create-customer request, load the template and pre-populate plan_id, status, connection_limit, RADIUS check/reply attributes on the new customer record
    - _Requirements: 1.6_

  - [ ]* 3.3 Write property test for template CRUD round-trip
    - **Property 1: Template CRUD Round-Trip**
    - **Validates: Requirements 1.2, 1.3**

  - [ ]* 3.4 Write property test for template name uniqueness
    - **Property 3: Template Name Uniqueness**
    - **Validates: Requirements 1.5**

- [x] 4. Bulk Actions API
  - [x] 4.1 Implement `BulkActionRequest`, `BulkActionResponse`, `BulkFailure` structs and bulk handler
    - Add Go structs for request/response
    - Implement `func (s *Server) customersBulk(w, r)` for POST `/api/customers/bulk`
    - Validate len(customer_ids) <= 200, return 400 with code `bulk_limit_exceeded` if exceeded
    - Iterate customer_ids applying action ("enable" → status='active', "disable" → status='disabled', "delete" → deleted_at + status='deleted')
    - Collect successes and failures, return BulkActionResponse
    - Register route
    - _Requirements: 2.2, 2.3, 2.4, 2.5, 2.7_

  - [ ]* 4.2 Write property test for bulk action size limit enforcement
    - **Property 6: Bulk Action Size Limit Enforcement**
    - **Validates: Requirements 2.5**

  - [ ]* 4.3 Write property test for bulk action partial failure reporting
    - **Property 7: Bulk Action Partial Failure Reporting**
    - **Validates: Requirements 2.7**

- [x] 5. Traffic Reset and Connection Limit APIs
  - [x] 5.1 Implement traffic reset handler at POST `/api/customers/{id}/traffic-reset`
    - Zero radacct input_octets and output_octets for the customer's current period (WHERE acctstoptime IS NULL)
    - Insert wallet_transaction of type "adjustment" with description containing "Traffic reset"
    - Insert audit_log entry
    - If customer status == 'limited', update to 'active'
    - Return {ok: true}
    - _Requirements: 3.1, 3.2, 3.3_

  - [x] 5.2 Add "traffic_reset" action to the bulk handler
    - Extend the customersBulk handler to support action "traffic_reset" which calls the traffic reset logic per-customer
    - _Requirements: 3.5_

  - [x] 5.3 Implement connection limit handler at POST `/api/customers/{id}/connection-limit`
    - Accept JSON body `{limit: int}`
    - If limit == 0: DELETE Simultaneous-Use from radcheck for the customer
    - If limit > 0: REPLACE INTO radcheck (username, attribute='Simultaneous-Use', op=':=', value=limit)
    - Return {ok: true, connection_limit: limit}
    - _Requirements: 4.1, 4.4_

  - [ ]* 5.4 Write property test for connection limit RADIUS attribute mapping
    - **Property 10: Connection Limit RADIUS Attribute Mapping**
    - **Validates: Requirements 4.1, 4.4**

  - [ ]* 5.5 Write property test for traffic reset zeroes counters and creates audit trail
    - **Property 8: Traffic Reset Zeroes Counters and Creates Audit Trail**
    - **Validates: Requirements 3.1, 3.2**

- [x] 6. Data Usage Warnings API
  - [x] 6.1 Implement `checkDataWarnings(username)` function in `panel/internal/api/`
    - Query total usage (SUM of input_octets + output_octets) from radacct for the customer
    - Query the customer's plan data cap (plans.data_gb * 1024^3)
    - Load thresholds from panel_settings key 'data_warning_thresholds'
    - For each threshold crossed that hasn't already triggered a warning: INSERT event (type='data_warning', severity='warning'), dispatch notification via existing Notifier
    - If usage >= 100% cap: UPDATE customer status to 'limited', INSERT event (type='data_cap_reached', severity='error')
    - _Requirements: 5.1, 5.2, 5.3, 5.6_

  - [x] 6.2 Implement GET `/api/portal/warnings` endpoint for customer portal
    - Return active data_warning events for the authenticated customer
    - _Requirements: 5.4_

  - [x] 6.3 Implement admin settings endpoint for warning threshold configuration
    - Add PUT `/api/settings/data-warning-thresholds` to update the panel_settings value
    - Validate thresholds are valid percentages (0-100)
    - _Requirements: 5.7_

  - [ ]* 6.4 Write property test for data threshold event creation
    - **Property 11: Data Threshold Event Creation**
    - **Validates: Requirements 5.2, 5.6**

- [x] 7. Node Agent Version and Diagnostics API
  - [x] 7.1 Implement GET `/api/node/agent/version` endpoint
    - Authenticate via X-Node-Token header
    - Query latest agent_releases record (ORDER BY released_at DESC LIMIT 1)
    - Return AgentVersionResponse with version, download URL, and checksum_sha256
    - _Requirements: 7.6_

  - [x] 7.2 Implement GET `/api/node/agent/download` endpoint
    - Authenticate via X-Node-Token header
    - Serve the binary file referenced in the latest agent_releases record
    - _Requirements: 7.2_

  - [x] 7.3 Implement diagnostics storage in the node status push handler
    - Extend the existing node push handler to accept optional `diagnostics` field
    - Upsert into node_diagnostics table on each push
    - Expose diagnostics data via the admin node detail endpoint
    - _Requirements: 9.4, 9.6_

- [x] 8. Checkpoint - Ensure all backend API handlers compile and tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 9. Admin Dashboard — Templates Management View
  - [x] 9.1 Create `stores/templates.ts` Pinia store in `panel/web/admin/src/stores/`
    - Define `useTemplatesStore` with: list ref, loading ref
    - Implement loadTemplates() → GET /api/templates
    - Implement createTemplate(payload) → POST /api/templates
    - Implement updateTemplate(id, payload) → PATCH /api/templates/{id}
    - Implement deleteTemplate(id) → DELETE /api/templates/{id}
    - On mutation success, re-fetch template list
    - _Requirements: 1.2, 1.3, 1.4_

  - [x] 9.2 Create `TemplatesView.vue` in `panel/web/admin/src/views/`
    - Add a table listing templates with columns: name, plan, status, connection_limit, created_by, actions
    - Add "Create Template" button opening a form dialog/modal with fields: name, plan_id (dropdown), status, connection_limit, RADIUS checks (JSON editor or structured list), RADIUS replies
    - Add edit and delete action buttons per row
    - Show confirmation dialog before delete
    - Register route in admin router
    - _Requirements: 1.1, 1.2, 1.3, 1.4_

  - [x] 9.3 Add template selection dropdown to the existing customer creation form
    - When a template is selected, auto-fill plan, status, connection_limit, and RADIUS attributes
    - _Requirements: 1.6_

- [x] 10. Admin Dashboard — Bulk Actions and Customer Enhancements
  - [x] 10.1 Extend `CustomersView.vue` with multi-select and bulk action toolbar
    - Add checkbox column to customer table rows
    - Add select-all toggle in header
    - Add bulk action toolbar (visible when selection > 0) with buttons: Enable, Disable, Delete, Traffic Reset
    - Show confirmation dialog before bulk delete (display count of affected customers)
    - Call bulkAction store method on confirmation
    - Display toast with success/failure summary from BulkActionResponse
    - _Requirements: 2.1, 2.6_

  - [x] 10.2 Extend `stores/customers.ts` with bulkAction, trafficReset, and setConnectionLimit methods
    - Add `bulkAction(request: BulkActionRequest)` → POST /api/customers/bulk, refetch list on success
    - Add `trafficReset(customerId)` → POST /api/customers/{id}/traffic-reset, refetch detail on success
    - Add `setConnectionLimit(customerId, limit)` → POST /api/customers/{id}/connection-limit, refetch detail
    - _Requirements: 2.2, 3.4, 4.3_

  - [x] 10.3 Extend `CustomerDetailView.vue` with traffic reset button and connection limit inline editor
    - Add "Reset Traffic" button that calls trafficReset and shows toast on result
    - Display current connection_limit with inline edit control
    - On edit, call setConnectionLimit
    - _Requirements: 3.4, 4.3_

- [x] 11. Admin Dashboard — Data Warning Settings and Null Safety
  - [x] 11.1 Add data warning threshold configuration to `SettingsView.vue`
    - Add a section for "Data Usage Warnings" with editable threshold percentage inputs
    - Save via PUT /api/settings/data-warning-thresholds
    - _Requirements: 5.7_

  - [x] 11.2 Create `utils/nullSafe.ts` in `panel/web/admin/src/`
    - Implement `displayValue(value: unknown, fallback = '—'): string` that returns fallback for null/undefined/empty
    - Apply to all nullable field renderings across admin views (CustomerDetailView, NodesView, etc.)
    - _Requirements: 10.3_

  - [x] 11.3 Create `composables/useFreshData.ts` stale data prevention composable
    - Implement with 30-second staleness threshold
    - Call fetcher on mount and on activated if data is stale
    - Apply to key views: CustomersView, CustomerDetailView, NodesView, DashboardView
    - _Requirements: 11.1, 11.3_

- [x] 12. Admin Dashboard — Error Handling and 401 Redirect
  - [x] 12.1 Enhance API composable with global error interceptor
    - On any error response: extract `error` field from body and emit toast notification
    - On 401 response: redirect to LoginView and clear auth store session state
    - On mutation success: trigger refetch of affected resource
    - _Requirements: 12.3, 12.5_

- [x] 13. Customer Portal — Usage Display and Warnings
  - [x] 13.1 Create `composables/useUsageDisplay.ts` in `panel/web/portal/src/`
    - Implement usage calculations: usedPercent, remainingBytes, progressColor (green/amber/red)
    - Implement daysRemaining computed from expiresAt
    - Implement `formatBytes(bytes)` utility
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5_

  - [x] 13.2 Enhance `DashboardView.vue` in portal with usage display
    - Show remaining data as both percentage and absolute value (e.g., "2.4 GB remaining / 10 GB")
    - Add visual progress bar with dynamic color based on useUsageDisplay composable
    - Show subscription expiry date and days remaining
    - Show persistent alert banner when usage exceeds 95%
    - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5_

  - [x] 13.3 Enhance `UsageView.vue` in portal with connection limit and session info
    - Display active session count and connection limit
    - _Requirements: 4.5, 6.6_

  - [x] 13.4 Add notification center component to portal
    - Create a notification center accessible from portal layout showing recent data_warning events and account status changes
    - Fetch from GET /api/portal/warnings
    - _Requirements: 5.4, 6.7_

  - [x] 13.5 Create `utils/nullSafe.ts` in `panel/web/portal/src/`
    - Same `displayValue` utility as admin, applied to all nullable fields in portal views
    - _Requirements: 10.4_

  - [ ]* 13.6 Write unit tests for useUsageDisplay composable
    - Test progress bar color thresholds (green > 20%, amber 5-20%, red <= 5%)
    - Test formatBytes at various magnitudes
    - Test daysRemaining calculation
    - **Property 12: Usage Display Calculations**
    - **Property 13: Usage Progress Bar Color Classification**
    - **Validates: Requirements 6.1, 6.2, 6.3, 6.4, 6.5**

- [x] 14. Customer Portal — Error Handling and Stale Data
  - [x] 14.1 Add error interceptor and 401 redirect to portal API composable
    - On error response: display toast with error message
    - On 401: redirect to login, clear session
    - _Requirements: 12.4, 12.6_

  - [x] 14.2 Add stale data composable to portal
    - Create `composables/useFreshData.ts` with 30-second threshold (same pattern as admin)
    - Apply to DashboardView, UsageView, BillingView
    - On mutation success: immediately invalidate and refetch affected data
    - _Requirements: 11.2, 11.4_

- [x] 15. Checkpoint - Ensure all frontend compiles and tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 16. Node Agent — Structured Logging
  - [x] 16.1 Create `node/internal/logger/logger.go` structured logger package
    - Implement Level type (Debug, Info, Warn, Error)
    - Implement Logger struct with mutex-protected io.Writer, level, and log method
    - Implement LogEntry struct with timestamp (ISO 8601), level, message, fields
    - Implement Info, Warn, Error, Debug methods accepting message + optional fields map
    - Implement SetLevel for dynamic level changes
    - Implement ParseLevel(string) to convert LOG_LEVEL env to Level
    - _Requirements: 9.1, 9.2_

  - [x] 16.2 Replace all `log.Printf` calls in `node/cmd/node/main.go` with structured logger
    - Instantiate logger from LOG_LEVEL env var
    - Replace every log.Printf/log.Fatalf with appropriate logger method
    - At debug level, include detailed request/response info for Panel API calls
    - _Requirements: 9.1, 9.3_

  - [x] 16.3 Implement non-2xx response logging in the node's HTTP post helper
    - On non-2xx responses: read body (limit 512 bytes), log at warn level with url, status, and truncated body
    - _Requirements: 12.7_

  - [x] 16.4 Implement consecutive failure tracking
    - Track consecutive push failures; after 3 failures, log at error level with cumulative disconnection duration and last error
    - _Requirements: 9.5_

  - [ ]* 16.5 Write unit tests for structured logger
    - **Property 17: Structured Log Format**
    - **Validates: Requirements 9.1**

- [x] 17. Node Agent — Config Hot-Reload
  - [x] 17.1 Create `node/internal/config/config.go` with hot-reload support
    - Implement Config struct with RWMutex-protected fields: PanelURL, NodeToken, Interval, AutoUpdate, LogLevel
    - Implement Reload(envFile) method that re-reads env file, validates, applies reloadable keys
    - Return map of changes (key → [old, new]) for logging
    - On validation error: retain current config, return error
    - Define reloadableKeys: PANEL_URL, NODE_INTERVAL, NODE_AUTO_UPDATE, LOG_LEVEL
    - _Requirements: 8.1, 8.3, 8.4, 8.5_

  - [x] 17.2 Add SIGHUP signal handler in `node/cmd/node/main.go`
    - Listen for SIGHUP, call config.Reload on signal
    - Log each changed key with old and new values at info level
    - On error: log error and continue with existing config
    - _Requirements: 8.1, 8.3, 8.4_

  - [x] 17.3 Add task-based reload handler for action "agent.reload_config"
    - In the task processing loop, handle action "agent.reload_config" by calling config.Reload
    - Log changes or errors same as SIGHUP handler
    - _Requirements: 8.2_

  - [ ]* 17.4 Write unit tests for config hot-reload
    - **Property 16: Config Hot-Reload With Validation**
    - **Validates: Requirements 8.3, 8.4**

- [x] 18. Node Agent — Auto-Update
  - [x] 18.1 Create `node/internal/updater/updater.go` with auto-update logic
    - Implement Updater struct with panelURL, nodeToken, currentVersion, http.Client
    - Implement CheckAndUpdate() that: queries version endpoint, compares versions, downloads if newer, verifies SHA-256 checksum, writes to temp file, renames over binary, restarts via systemctl
    - Implement VerifyChecksum(data, expected) → bool
    - Implement CompareVersions(current, remote) → bool for semver comparison
    - If NODE_AUTO_UPDATE=false, skip all checks
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.7_

  - [x] 18.2 Integrate auto-update into the main agent loop
    - On startup and at configurable interval (default 6h), call updater.CheckAndUpdate()
    - Use config's AutoUpdate flag to gate checks
    - _Requirements: 7.1, 7.7_

  - [ ]* 18.3 Write unit tests for version comparison and checksum verification
    - **Property 14: Version Comparison Triggers Download**
    - **Property 15: Binary Checksum Verification**
    - **Validates: Requirements 7.2, 7.3, 7.4**

- [x] 19. Node Agent — Diagnostics Push
  - [x] 19.1 Implement diagnostics report builder and integrate into status push
    - Implement `buildDiagnostics(startTime, version)` using runtime.MemStats and runtime.NumGoroutine
    - Include DiagnosticsReport in every status push payload as optional `diagnostics` field
    - _Requirements: 9.4_

  - [ ]* 19.2 Write unit test for diagnostics report completeness
    - **Property 18: Diagnostics Report Completeness**
    - **Validates: Requirements 9.4**

- [x] 20. Final Checkpoint - Ensure full build and all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document
- Unit tests validate specific examples and edge cases
- The implementation language is Go for backend/node agent and TypeScript/Vue 3 for frontends, matching the existing codebase
- Existing handler patterns use `func (s *Server) handler(w http.ResponseWriter, r *http.Request)` with method switch
- Admin frontend uses Pinia Composition API stores and Vue 3 SFCs
- Migrations are numbered sequentially in `panel/migrations/`

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2", "1.3", "1.4"] },
    { "id": 1, "tasks": ["3.1", "4.1", "5.1", "5.3", "6.1", "7.1", "7.2", "7.3", "16.1"] },
    { "id": 2, "tasks": ["3.2", "3.3", "3.4", "4.2", "4.3", "5.2", "5.4", "5.5", "6.2", "6.3", "6.4", "16.2", "16.3", "16.4", "17.1"] },
    { "id": 3, "tasks": ["9.1", "10.2", "13.1", "13.5", "16.5", "17.2", "17.3", "18.1"] },
    { "id": 4, "tasks": ["9.2", "9.3", "10.1", "10.3", "11.1", "11.2", "11.3", "12.1", "13.2", "13.3", "13.4", "14.1", "14.2", "17.4", "18.2"] },
    { "id": 5, "tasks": ["13.6", "18.3", "19.1"] },
    { "id": 6, "tasks": ["19.2"] }
  ]
}
```
