# Implementation Plan: AI Health Monitor

## Overview

This plan implements the AI Health Monitor system for KorisPanel — a comprehensive diagnostics, auto-healing, and smart alerting suite. The implementation is structured in layers: core types and interfaces first, then individual health checks, then the auto-healer worker, then the alert engine, and finally API handlers and report generation. All code is in Go 1.22+ with the `pgregory.net/rapid` library for property-based testing.

## Tasks

- [ ] 1. Database migration and core types
  - [ ] 1.1 Create migration file `panel/migrations/020_ai_health_monitor.sql`
    - Create tables: `health_scores`, `healing_rules`, `healing_actions`, `anomaly_events`
    - Seed default healing rules (stale_sessions, vpn_crash_openvpn, vpn_crash_l2tp, vpn_crash_ikev2, disk_critical, memory_critical, node_offline_failover)
    - Insert health monitor settings into `settings` table
    - _Requirements: 2.1, 3.6, 4.1, 5.3, 9.1_

  - [ ] 1.2 Create `panel/internal/health/health.go` with core types and interfaces
    - Define `Analyzer` interface with `Analyze(AnalysisInput) (AnalysisOutput, error)` method
    - Define types: `Severity`, `CheckResult`, `HealthReport`, `AnalysisInput`, `AnalysisOutput`
    - Define constants: `SeverityHealthy`, `SeverityWarning`, `SeverityCritical`
    - Implement `ComputeScore(results []CheckResult, weights map[string]float64) int` with penalty formula and clamping to [0,100]
    - Implement `ClassifySeverity(value, warningThreshold, criticalThreshold float64) Severity`
    - Implement `ComputeTrend(currentScore int, historicalScores []int) string`
    - _Requirements: 1.3, 1.6, 2.4, 10.1, 10.4_

  - [ ]* 1.3 Write property tests for core computation functions
    - **Property 1: Severity Threshold Assignment** — For any value and threshold pair, ClassifySeverity returns correct severity
    - **Validates: Requirements 1.3**
    - **Property 2: Health Score Bounded Computation** — For any check results and weights, ComputeScore returns [0,100]
    - **Validates: Requirements 1.6**
    - **Property 4: Trend Direction Calculation** — For any score and history, ComputeTrend returns correct direction
    - **Validates: Requirements 2.4**

- [ ] 2. Diagnostics Engine and health checks
  - [ ] 2.1 Create `panel/internal/health/checks.go` with health check implementations
    - Define `HealthCheck` interface with `Name() string`, `Category() string`, `Run(ctx, db) CheckResult`
    - Implement checks: DatabaseCheck, NodeOnlineCheck, VPNServiceCheck, DiskUsageCheck, MemoryUsageCheck, CPUUsageCheck, StaleSessionCheck, ExpiredSubscriptionCheck, DNSFailoverCheck
    - Each check queries the relevant existing tables (nodes, node_status, radacct, customers, failover_events)
    - Each check assigns severity via `ClassifySeverity` with configurable thresholds
    - Each warning/critical check populates `SuggestedActions` with specific remediation steps
    - _Requirements: 1.2, 1.3, 1.4_

  - [ ] 2.2 Create `panel/internal/health/diagnostics.go` with DiagnosticsEngine
    - Implement `NewDiagnosticsEngine(db, analyzer, notifier)` registering all checks
    - Implement `RunAll(ctx) (*HealthReport, error)` — runs checks with 5s per-check timeout, computes score, computes trend from DB history, invokes Analyzer for RCA
    - Implement `PersistScore(ctx, report)` to save score/trend/checks_json to `health_scores` table
    - Implement `GetHistory(ctx, from, to time.Time) ([]HealthScoreRecord, error)` for historical data
    - Handle DB failure gracefully: if ping fails, return score=0 with single critical check
    - Handle Analyzer failure: return report without `root_cause_analysis` field
    - _Requirements: 1.1, 1.5, 1.6, 1.7, 2.1, 2.3, 2.4_

  - [ ]* 2.3 Write property test for Health Score persistence round-trip
    - **Property 3: Health Score Persistence Round-Trip** — For any valid HealthReport, persist then read back produces equivalent data
    - **Validates: Requirements 2.1**

- [ ] 3. Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 4. Auto-Healer worker
  - [ ] 4.1 Create `panel/internal/health/healer.go` with AutoHealer
    - Implement `NewAutoHealer(db, notifier, orchestrator)` with configurable interval
    - Implement `Start(ctx)` background loop with graceful shutdown on context cancel
    - Implement `Tick(ctx) error` — one cycle of detect + heal
    - Implement `DetectConditions(ctx) ([]TriggeredCondition, error)` — queries DB for stale sessions, crashed VPN services, node offline status, disk/memory thresholds against enabled rules
    - Implement `ShouldHeal(rule, resourceID, now) bool` — checks cooldown window using `healing_actions` table with `SELECT ... FOR UPDATE`
    - Implement `ExecuteAction(ctx, condition) error` with action dispatch:
      - Stale sessions: update radacct stop time and terminate cause
      - VPN crash: create node_task with `action="restart_service"`
      - Node offline: call `orchestrator.TriggerFailover()` for domains on that node
      - Disk/memory: alert only (default rules)
    - Log every action to `healing_actions` table with timing, result, and error message
    - Respect rule `action_mode`: if `alert_only`, send Telegram alert instead of remediating
    - Skip disabled rules entirely
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 4.4, 4.5_

  - [ ]* 4.2 Write property tests for Auto-Healer logic
    - **Property 5: Condition Detection Accuracy** — For any system state and rules, DetectConditions returns exactly matching conditions
    - **Validates: Requirements 3.2**
    - **Property 6: Cooldown Enforcement** — For any rule and timestamps within cooldown, ShouldHeal returns false
    - **Validates: Requirements 3.7**
    - **Property 7: Rule Mode Enforcement** — alert_only dispatches alert without remediation, disabled skips entirely
    - **Validates: Requirements 4.4, 4.5**
    - **Property 12: Healing Action Audit Completeness** — Every action record has all required non-empty fields
    - **Validates: Requirements 3.6, 9.1**

- [ ] 5. Healing rule CRUD and audit log
  - [ ] 5.1 Implement healing rule database operations in `panel/internal/health/healer.go`
    - `GetRules(ctx) ([]HealingRule, error)` — returns all rules from `healing_rules`
    - `UpdateRule(ctx, id, updates) error` — updates mode, cooldown, enabled, thresholds
    - `GetHealingLog(ctx, filters, page, pageSize) ([]HealingAction, total int, error)` — paginated query with filters for date range, rule_key, result_status, resource_type
    - _Requirements: 4.1, 4.2, 4.3, 9.2, 9.3_

  - [ ]* 5.2 Write property test for paginated filter correctness
    - **Property 13: Paginated Filter Correctness** — For any records and filter criteria, query returns only matching records with correct count
    - **Validates: Requirements 9.2**

- [ ] 6. Alert Engine — anomaly detection and correlation
  - [ ] 6.1 Create `panel/internal/health/alert.go` with AlertEngine
    - Implement `NewAlertEngine(db, notifier, analyzer)` with configurable interval
    - Implement `Start(ctx)` background loop
    - Implement `Tick(ctx) error` — one cycle of baseline computation, anomaly detection, correlation, and alerting
    - Implement `ComputeBaseline(values []float64) (mean, stddev float64)` for rolling metrics
    - Implement `IsAnomaly(value, mean, stddev, multiplier float64) bool` — returns true when value > mean + multiplier*stddev
    - Compute baselines for: failed logins/min, disconnections/min, payment failure rate, data usage rate
    - Record anomalies to `anomaly_events` table
    - Skip metric if fewer than 10 data points (insufficient baseline)
    - _Requirements: 5.1, 5.2, 5.3, 5.4_

  - [ ] 6.2 Implement event correlation in `panel/internal/health/alert.go`
    - Implement `CorrelateEvents(events []AnomalyEvent, window time.Duration) []CorrelatedIncident`
    - Apply correlation rules: node offline + disconnections = node failure, multiple service crashes = node instability, failed login spike from same IP range = brute force
    - Assign severity as max severity among contributing events
    - Generate RCA summary via Analyzer interface
    - Cap correlated incident size at 50 events
    - _Requirements: 6.1, 6.2, 6.3, 6.4_

  - [ ]* 6.3 Write property tests for anomaly detection and correlation
    - **Property 8: Anomaly Detection Correctness** — IsAnomaly returns true iff value > mean + k*stddev
    - **Validates: Requirements 5.1, 5.2**
    - **Property 9: Event Correlation** — Events matching patterns are grouped, severity equals max contributing severity
    - **Validates: Requirements 6.1, 6.2, 6.3, 6.4**

- [ ] 7. Alert deduplication and Telegram integration
  - [ ] 7.1 Create `panel/internal/health/dedup.go` with Deduplicator
    - Implement `Deduplicator` struct with mutex-protected sent map and configurable window
    - Implement `ShouldSend(alertType, resourceID string) bool` — returns true only if not sent within window
    - Implement `MarkSent(alertType, resourceID string)` — records send timestamp
    - Implement periodic cleanup of expired entries
    - _Requirements: 7.3_

  - [ ] 7.2 Implement alert formatting and dispatch in `panel/internal/health/alert.go`
    - Implement `FormatAlert(incident CorrelatedIncident, analysis *AnalysisOutput) string` — formats with severity icon, title, affected components, RCA summary, suggested actions
    - Integrate with existing `notify.Notifier` for Telegram sending
    - Implement retry logic: up to 3 retries with exponential backoff (1s, 2s, 4s) on send failure
    - Use Deduplicator before sending — only send if `ShouldSend` returns true
    - _Requirements: 7.1, 7.2, 7.3, 7.4_

  - [ ]* 7.3 Write property tests for deduplication and alert formatting
    - **Property 10: Alert Message Formatting Completeness** — FormatAlert output contains severity icon, title, components, suggested actions
    - **Validates: Requirements 7.2**
    - **Property 11: Alert Deduplication** — ShouldSend returns true only for first alert within window
    - **Validates: Requirements 7.3**

- [ ] 8. Checkpoint
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 9. RuleBasedAnalyzer and Analyzer factory
  - [ ] 9.1 Create `panel/internal/health/analyzer_rules.go` with RuleBasedAnalyzer
    - Implement correlation pattern matching:
      - Node offline + user disconnections → "Node failure"
      - Multiple service crashes on same node → "Node instability"
      - Failed login spike from same IP range → "Potential brute force attack"
      - High disk + service crash → "Disk space causing service failure"
      - Multiple expired subscriptions + payment failures → "Payment system issue"
    - Return `AnalysisOutput` with confidence score, suggested actions, and affected components
    - _Requirements: 10.2_

  - [ ] 9.2 Create `panel/internal/health/analyzer_llm.go` with LLMAnalyzer stub
    - Implement `LLMAnalyzer` struct with endpoint URL and http.Client
    - Implement `Analyze()` that POSTs structured input to configured endpoint, parses structured response
    - _Requirements: 10.3_

  - [ ] 9.3 Implement `NewAnalyzer()` factory function in `panel/internal/health/health.go`
    - Check `PANEL_LLM_ENDPOINT` env var: if set, return LLMAnalyzer; otherwise return RuleBasedAnalyzer
    - _Requirements: 10.1, 10.2, 10.3_

- [ ] 10. Report generation
  - [ ] 10.1 Create `panel/internal/health/reports.go` with report generation
    - Implement `GenerateDailyReport(ctx, db) (string, error)` — average score, healing action count, anomaly count, top 3 recurring issues, trend direction
    - Implement `GenerateWeeklyReport(ctx, db) (string, error)` — daily score chart data, total healing actions with success rate, anomalies by type, week-over-week comparison
    - Implement scheduled report dispatch via Telegram notifier
    - Read schedule configuration from settings (daily_report_hour, weekly_report_day)
    - _Requirements: 8.1, 8.2, 8.3, 8.4_

  - [ ]* 10.2 Write property test for report generation correctness
    - **Property 14: Report Generation Correctness** — For any historical data set, report contains correct averages, counts, and trends
    - **Validates: Requirements 8.1, 8.2**

- [ ] 11. API handlers and route registration
  - [ ] 11.1 Create API handlers in `panel/internal/api/health.go`
    - `aiDiagnostics(w, r)` — GET only, calls `healthEngine.RunAll()`, persists score, returns JSON report
    - `aiDiagnosticsHistory(w, r)` — GET with optional `from`/`to` query params, returns historical scores
    - `aiHealingRules(w, r)` — GET, returns all healing rules as JSON
    - `aiHealingRuleByID(w, r)` — PUT with rule ID from URL path, updates rule fields
    - `aiHealingLog(w, r)` — GET with pagination params (`page`, `page_size`) and filter params (`from`, `to`, `rule_key`, `status`)
    - All handlers use `requireAdmin` middleware
    - _Requirements: 1.1, 1.7, 2.3, 4.2, 4.3, 9.2_

  - [ ] 11.2 Register routes and wire dependencies in `panel/cmd/panel/main.go`
    - Add `healthEngine *health.DiagnosticsEngine` field to Server struct (or local var)
    - Add `autoHealer *health.AutoHealer` and `alertEngine *health.AlertEngine`
    - Initialize `NewAnalyzer()`, `NewDiagnosticsEngine()`, `NewAutoHealer()`, `NewAlertEngine()`
    - Start AutoHealer and AlertEngine workers with app context
    - Register all `/api/diagnostics/ai*` routes in `Routes()` function
    - Add `pgregory.net/rapid` to `go.mod` as test dependency
    - _Requirements: 1.1, 3.1, 5.4_

  - [ ]* 11.3 Write unit tests for API handlers
    - Test auth enforcement (unauthenticated returns 401)
    - Test method validation (POST to GET-only endpoint returns 405)
    - Test response structure (JSON shape matches HealthReport)
    - Test pagination parameters parsing
    - _Requirements: 1.1, 1.7, 9.2_

- [ ] 12. Data retention cleanup
  - [ ] 12.1 Add retention cleanup to existing daily worker
    - Delete `health_scores` older than configured retention (default 30 days)
    - Delete `healing_actions` older than configured retention (default 90 days)
    - Delete `anomaly_events` older than 30 days
    - Read retention settings from `settings` table
    - _Requirements: 2.2, 9.3_

- [ ] 13. Final checkpoint
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- The implementation uses Go 1.22+ with `pgregory.net/rapid` for property-based testing
- All API handlers follow existing KorisPanel patterns (requireAdmin middleware, writeJSON helpers)
- The AutoHealer integrates with existing node task system and failover orchestrator — no changes to those systems required
- The AlertEngine uses the existing Telegram notifier — no new notification infrastructure needed
- Property tests validate universal correctness properties; unit tests cover edge cases and integration points
