# Implementation Plan: DNS Failover Profiles

## Overview

Implement the DNS Failover system that enables transparent VPN server migration using domain names in OpenVPN profiles. The implementation covers: backend API endpoints for DNS providers, failover domains, and failover events; Cloudflare API integration; a failover orchestrator with propagation verification; an auto-failover background worker; modified OpenVPN profile generation; admin frontend views with real-time updates; and Telegram notifications.

## Tasks

- [ ] 1. Backend data models and encryption utilities
  - [ ] 1.1 Create Go structs and encryption helper for DNS failover
    - Create file `panel/internal/api/failover.go` with DNSProvider, FailoverDomain, FailoverEvent structs matching the design
    - Implement AES-256-GCM encryption/decryption functions for API token storage using `PANEL_SECRET`
    - Add `pathID`-style helper to parse `/api/failover/providers/`, `/api/failover/domains/`, `/api/failover/events/` paths
    - Add input validation helpers: FQDN validation (RFC 1035), TTL bounds (30–86400, default 60)
    - _Requirements: 1.3, 2.1, 2.3, 5_

  - [ ]* 1.2 Write property tests for encryption and validation
    - **Property 9: API Token Encryption Round-Trip** — for any token string, encrypt then decrypt must return original; ciphertext must differ from plaintext
    - **Property 5: TTL Bounds Enforcement** — for any TTL input, values outside 30–86400 are rejected; zero/missing defaults to 60
    - **Validates: Requirements 1.3, 2.3**

- [ ] 2. DNS Provider API endpoints
  - [ ] 2.1 Implement DNS provider CRUD handlers
    - Add `func (s *Server) failoverProviders(w, r)` with method switch for GET (list) and POST (create)
    - Add `func (s *Server) failoverProviderByID(w, r)` with method switch for PATCH (update) and DELETE
    - On create: validate unique name, type is "cloudflare" or "manual", require api_token+zone_id for cloudflare type
    - On delete: reject if provider is referenced by active failover_domains (Requirement 1.6)
    - Encrypt API tokens before storing, never return them in responses (tag `json:"-"`)
    - _Requirements: 1.1, 1.2, 1.4, 1.6_

  - [ ] 2.2 Implement provider test connection endpoint
    - Add `POST /api/failover/providers/{id}/test` handler
    - For Cloudflare: call `GET /zones/{zone_id}` with stored (decrypted) API token to verify access
    - Return `{ "ok": true, "message": "Connection successful" }` or error with details
    - On invalid token, mark provider `is_active = 0` (Requirement 1.7)
    - _Requirements: 1.5, 1.7_

  - [ ]* 2.3 Write property test for API token confidentiality
    - **Property 7: API Token Confidentiality** — for any API response containing a DNSProvider, the api_token field must be empty/absent
    - **Validates: Requirement 1.4**

- [ ] 3. Failover Domain API endpoints
  - [ ] 3.1 Implement failover domain CRUD handlers
    - Add `func (s *Server) failoverDomains(w, r)` with GET (list with joins for node name/IP/provider) and POST (create)
    - Add `func (s *Server) failoverDomainByID(w, r)` with PATCH (update), DELETE, and sub-path routing for `/failover` and `/status`
    - On create: validate FQDN uniqueness, node existence, TTL bounds, optional dns_provider_id reference
    - On deactivate (`is_active = 0`): domain excluded from profile gen and auto-failover (Requirement 2.4)
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 2.5_

  - [ ]* 3.2 Write property tests for domain uniqueness and inactive domain exclusion
    - **Property 3: Domain Uniqueness Invariant** — no two active failover_domains may share the same domain name
    - **Property 13: Inactive Domain Exclusion** — deactivated domains must never be returned by profile lookup or health-checked by auto-failover
    - **Validates: Requirements 2.1, 2.4**

- [ ] 4. Checkpoint - Ensure provider and domain CRUD compiles and tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 5. Cloudflare DNS updater and DNSUpdater interface
  - [ ] 5.1 Implement DNSUpdater interface and Cloudflare client
    - Create file `panel/internal/api/failover_dns.go`
    - Define `DNSUpdater` interface with `UpdateARecord`, `GetCurrentIP`, `VerifyPropagation` methods
    - Implement `CloudflareUpdater` struct: PUT to `/zones/{zone}/dns_records/{record}` with `"proxied": false`
    - Implement `ManualUpdater` struct (no-op, logs instructions)
    - Handle Cloudflare errors: 401 → mark inactive; 429 → exponential backoff up to 3 retries; 5xx → retry up to 3 times
    - Implement DNS propagation check via `net.LookupHost` comparing resolved IP to expected
    - _Requirements: 9.1, 9.2, 9.3, 9.4_

  - [ ]* 5.2 Write unit tests for Cloudflare updater retry logic
    - Test 429 exponential backoff behavior with HTTP test server
    - Test 5xx retry up to 3 times with backoff
    - Test 401 marks provider inactive
    - _Requirements: 9.2, 9.3_

- [ ] 6. Failover orchestrator
  - [ ] 6.1 Implement FailoverOrchestrator with TriggerFailover, CheckPropagation, and Rollback
    - Create file `panel/internal/api/failover_orchestrator.go`
    - `TriggerFailover`: validate target != current (Req 3.1), target online (Req 3.2), no concurrent failover pending/propagating (Req 3.3)
    - Create failover_event with status "pending", call DNSUpdater, transition to "propagating" on success or "failed" on error
    - Update `failover_domains.current_node_id` and `last_failover_at` on DNS success
    - Launch background goroutine for propagation checking (poll every 10s, timeout from settings)
    - `CheckPropagation`: poll DNS, mark "completed" when IP matches, mark "failed" on timeout (keep DNS pointing to new IP)
    - `Rollback`: validate original node is online, create new failover_event with reason "auto_rollback", trigger reverse failover
    - Send Telegram notification on failover start/complete/fail (Req 11.1, 11.2)
    - Broadcast WebSocket events for status changes (Req 10.1–10.4)
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 4.1, 4.2, 4.3, 4.4, 5.1, 5.2, 5.3, 10.1, 10.2, 10.3, 10.4, 11.1, 11.2_

  - [ ]* 6.2 Write property tests for failover orchestrator
    - **Property 2: Failover State Machine Validity** — status transitions must follow only valid paths (pending→propagating→completed, pending→propagating→failed, pending→failed, completed→rolled_back)
    - **Property 4: Concurrent Failover Prevention** — at most one event in pending/propagating per domain at any time
    - **Property 10: Same-Node Failover Rejection** — failover where target == current must always be rejected
    - **Property 11: Offline Target Node Rejection** — failover to offline node must always be rejected
    - **Property 8: Failover Event Audit Completeness** — any change to current_node_id must have a corresponding FailoverEvent
    - **Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 8.1, 8.4**

- [ ] 7. Failover events and trigger API endpoints
  - [ ] 7.1 Implement failover trigger, status, events, and rollback endpoints
    - `POST /api/failover/domains/{id}/failover`: parse TriggerFailoverRequest, call orchestrator.TriggerFailover
    - `GET /api/failover/domains/{id}/status`: return current propagation status (domain, current_ip, expected_ip, propagated, last_event)
    - `GET /api/failover/events`: list events with pagination, filterable by domain_id, status, triggered_by
    - `GET /api/failover/events/{id}`: single event detail
    - `POST /api/failover/events/{id}/rollback`: call orchestrator.Rollback
    - _Requirements: 3.4, 4.1, 5.1, 8.1, 8.2, 8.3_

  - [ ]* 7.2 Write unit tests for failover trigger validation
    - Test 400 on same-node failover
    - Test 400 on offline target node
    - Test 409 on concurrent failover in progress
    - Test successful failover creates event with status "pending"
    - _Requirements: 3.1, 3.2, 3.3, 3.4_

- [ ] 8. Checkpoint - Ensure failover orchestrator and endpoints compile and tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 9. Auto-failover worker
  - [ ] 9.1 Implement AutoFailoverWorker background goroutine
    - Create file `panel/internal/api/failover_worker.go`
    - Read `dns_failover_enabled` and `dns_failover_check_interval` from panel_settings
    - If disabled, remain idle (Req 7.2)
    - On each tick: query active failover_domains, check `node_diagnostics.updated_at` freshness for each domain's current_node
    - Node offline condition: `time.Since(lastPush) > 2 * checkInterval` (Req 7.3)
    - Require 2 consecutive offline checks before triggering (Req 7.4)
    - `selectFallbackNode`: prefer online nodes not targeted by other active failover_domains (Req 7.5)
    - If no fallback available: log warning + Telegram notification, do not change DNS (Req 7.6)
    - Create failover_event with triggered_by="auto" and reason indicating cause (Req 7.7)
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7_

  - [ ] 9.2 Implement auto-rollback on original node recovery
    - If `dns_failover_auto_rollback` is "true", monitor original nodes of completed auto-failovers
    - When original node comes back online, trigger reverse failover with reason "auto_rollback"
    - Send Telegram notification on no-available-fallback condition (Req 11.3)
    - _Requirements: 12.1, 12.2, 12.3, 11.3_

  - [ ]* 9.3 Write property tests for auto-failover worker
    - **Property 6: Auto-Failover Safety Guards** — auto-failover must NOT trigger if: disabled, node push is fresh, failover already in progress for domain, no alternative nodes
    - **Property 12: Fallback Node Selection Preference** — selection must prefer online nodes not already targeted by another active failover_domain
    - **Validates: Requirements 7.2, 7.3, 7.5, 7.6**

- [ ] 10. Modify OpenVPN profile generation
  - [ ] 10.1 Update openVPNEndpointNode to prefer failover domains
    - Modify existing `openVPNEndpointNode` function in the profile download handler
    - Add query: `SELECT domain FROM failover_domains WHERE current_node_id = ? AND is_active = 1 LIMIT 1`
    - New priority: failover_domain > node.domain > node.public_ip > request host
    - Ensure `resolv-retry infinite` directive is present in all generated profiles (Req 6.4)
    - _Requirements: 6.1, 6.2, 6.3, 6.4_

  - [ ]* 10.2 Write property tests for profile generation
    - **Property 1: Profile Domain Resolution Priority** — node with active failover_domain must use that domain as remote host, taking priority over node domain and public IP
    - **Property 14: Resolv-Retry Directive Presence** — all generated profiles must contain "resolv-retry infinite"
    - **Validates: Requirements 6.1, 6.2, 6.4**

- [ ] 11. Register routes and wire server startup
  - [ ] 11.1 Register all failover API routes and start worker
    - Add route registrations in `api.go` or main server setup:
      - `/api/failover/providers` → `s.failoverProviders` (requireAdmin)
      - `/api/failover/providers/` → `s.failoverProviderByID` (requireAdmin)
      - `/api/failover/domains` → `s.failoverDomains` (requireAdmin)
      - `/api/failover/domains/` → `s.failoverDomainByID` (requireAdmin)
      - `/api/failover/events` → `s.failoverEvents` (requireAdmin)
      - `/api/failover/events/` → `s.failoverEventByID` (requireAdmin)
    - Initialize FailoverOrchestrator with DB, DNSUpdater (based on provider type), Notifier, settings
    - Start AutoFailoverWorker goroutine on server startup with context cancellation
    - Add WebSocket event types: "failover:started", "failover:propagating", "failover:completed", "failover:failed"
    - _Requirements: 10.1, 10.2, 10.3, 10.4_

- [ ] 12. Checkpoint - Ensure full backend compiles and all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 13. Admin frontend - Pinia store
  - [ ] 13.1 Create failover Pinia store
    - Create file `panel/web/admin/src/stores/failover.ts`
    - Define interfaces: DNSProvider, FailoverDomain, FailoverEvent matching backend JSON
    - Implement actions: fetchProviders, createProvider, updateProvider, deleteProvider, testProvider
    - Implement actions: fetchDomains, createDomain, updateDomain, deleteDomain, triggerFailover, getDomainStatus
    - Implement actions: fetchEvents, rollbackEvent
    - Handle WebSocket messages for real-time status updates (failover:started, failover:completed, failover:failed)
    - _Requirements: 1.1–1.7, 2.1–2.5, 3.1–3.6, 8.3, 10.1–10.4_

- [ ] 14. Admin frontend - Failover domains view
  - [ ] 14.1 Create FailoverView.vue main page
    - Create file `panel/web/admin/src/views/FailoverView.vue`
    - Display table: Domain, Current Node (name+IP), Provider, TTL, Status badge (active/inactive), Last Failover timestamp, Actions
    - "Add Domain" button → modal with form: domain FQDN, select node, select provider (optional), DNS record ID, TTL
    - Edit and Delete action buttons per row
    - One-click "Failover" button → modal: shows current node, dropdown of online target nodes, optional reason, confirm button
    - After failover trigger: show live propagation status with progress indicator (via WebSocket events from store)
    - _Requirements: 2.1–2.5, 3.1–3.6, 10.1–10.4_

  - [ ] 14.2 Create FailoverProvidersView.vue page
    - Create file `panel/web/admin/src/views/FailoverProvidersView.vue`
    - List configured DNS providers with name, type, zone_id, status badge
    - "Add Provider" form: Name, Type (Cloudflare/Manual), API Token (password input), Zone ID, Account ID (optional)
    - "Test Connection" button per provider showing success/failure
    - Active/Inactive toggle per provider
    - Edit and Delete actions
    - _Requirements: 1.1–1.7_

  - [ ] 14.3 Create FailoverEventsView.vue page
    - Create file `panel/web/admin/src/views/FailoverEventsView.vue`
    - Chronological list of failover events with: timestamp, domain, from→to nodes, status badge, triggered_by
    - Filter controls: by domain, status, trigger type (admin/auto)
    - Expandable detail row showing error messages, propagation timestamps, duration
    - Rollback button for completed events
    - _Requirements: 8.1–8.4, 5.1_

- [ ] 15. Admin frontend - Router and navigation
  - [ ] 15.1 Register failover routes in router
    - Add routes to `panel/web/admin/src/router/index.ts`:
      - `/failover` → FailoverView.vue (name: 'failover')
      - `/failover/providers` → FailoverProvidersView.vue (name: 'failover-providers')
      - `/failover/events` → FailoverEventsView.vue (name: 'failover-events')
    - Add navigation items to sidebar/menu in AppShell layout
    - _Requirements: 8.3_

- [ ] 16. Final checkpoint - Full integration verification
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document
- Unit tests validate specific examples and edge cases
- The migration `019_dns_failover.sql` already exists and does not need to be created
- Backend follows existing patterns: `func (s *Server) handler(w, r)` with method switch, `requireAdmin` middleware
- Frontend follows existing patterns: Pinia Composition API stores, Vue 3 SFC views, lazy-loaded routes

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2", "5.1"] },
    { "id": 2, "tasks": ["2.1", "3.1", "5.2"] },
    { "id": 3, "tasks": ["2.2", "2.3", "3.2"] },
    { "id": 4, "tasks": ["6.1"] },
    { "id": 5, "tasks": ["6.2", "7.1", "10.1"] },
    { "id": 6, "tasks": ["7.2", "10.2", "9.1"] },
    { "id": 7, "tasks": ["9.2", "9.3"] },
    { "id": 8, "tasks": ["11.1"] },
    { "id": 9, "tasks": ["13.1"] },
    { "id": 10, "tasks": ["14.1", "14.2", "14.3"] },
    { "id": 11, "tasks": ["15.1"] }
  ]
}
```
