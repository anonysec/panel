# Requirements Document

## Introduction

AI Health Monitor is a comprehensive health monitoring suite for KorisPanel that provides intelligent diagnostics, automatic healing of common issues, and smart log-based alerting. The system operates using rule-based heuristics and pattern matching (no external AI APIs required), with an optional future integration path for LLM-powered analysis. It consists of three interconnected components: a Smart Diagnostics endpoint, an Auto-Healing background worker, and a Log Analysis & Smart Alerting engine.

## Glossary

- **Health_Monitor**: The overarching system responsible for coordinating diagnostics, auto-healing, and alerting subsystems within KorisPanel.
- **Diagnostics_Engine**: The component that executes health checks across all monitored resources and produces structured health reports with severity ratings and suggested actions.
- **Auto_Healer**: The background worker that detects known failure patterns and attempts automated remediation within configurable policy rules.
- **Alert_Engine**: The component that monitors system events, detects anomalies, correlates related events, deduplicates alerts, and dispatches notifications via Telegram.
- **Health_Check**: A single probe that evaluates one aspect of system health (e.g., database connectivity, disk usage, service status).
- **Health_Score**: A numeric value (0-100) representing overall system health at a point in time, derived from individual check results.
- **Severity**: A classification of check results: `healthy` (no issues), `warning` (degraded but functional), or `critical` (requires immediate attention).
- **Healing_Rule**: A configurable policy that defines what condition triggers an auto-fix, what action to take, and whether the rule is set to auto-fix or alert-only mode.
- **Cooldown_Period**: A minimum time interval that must elapse between consecutive auto-healing actions targeting the same resource.
- **Anomaly**: A statistically significant deviation from baseline behavior detected by the Alert_Engine (e.g., spike in failed logins, mass disconnections).
- **Alert_Deduplication**: The process of suppressing repeated notifications for the same ongoing issue within a configurable time window.
- **Root_Cause_Analysis**: A heuristic process that correlates multiple concurrent events to identify the most likely underlying cause of observed symptoms.
- **Node**: A VPN server instance managed by KorisPanel, running OpenVPN/L2TP/IKEv2 services and reporting metrics via the node agent.
- **Stale_Session**: A RADIUS accounting session (radacct) with no stop time that has been inactive beyond a configurable threshold.

## Requirements

### Requirement 1: Diagnostics Endpoint

**User Story:** As a panel administrator, I want to call an API endpoint that runs comprehensive health checks and returns structured results with severity ratings and fix suggestions, so that I can quickly assess system health without manually checking each component.

#### Acceptance Criteria

1. WHEN a GET request is received at `/api/diagnostics/ai`, THE Diagnostics_Engine SHALL execute all registered Health_Checks and return a JSON response within 30 seconds.
2. THE Diagnostics_Engine SHALL include the following Health_Checks: database connectivity, node online status, VPN service health (OpenVPN, L2TP, IKEv2), disk usage, memory usage, CPU usage, Stale_Session detection, expired subscription count, and DNS failover status.
3. WHEN a Health_Check completes, THE Diagnostics_Engine SHALL assign a Severity of `healthy`, `warning`, or `critical` based on configurable thresholds.
4. WHEN any Health_Check returns a `warning` or `critical` Severity, THE Diagnostics_Engine SHALL include a `suggested_actions` array with specific remediation steps the administrator can take.
5. THE Diagnostics_Engine SHALL include a Root_Cause_Analysis section that correlates related check failures (e.g., node offline + multiple service failures on same node = node connectivity issue).
6. THE Diagnostics_Engine SHALL compute an overall Health_Score (0-100) derived from the weighted results of all individual Health_Checks.
7. WHEN the diagnostics endpoint is called, THE Diagnostics_Engine SHALL authenticate the request using the existing admin session or API key mechanism.

### Requirement 2: Health Score History

**User Story:** As a panel administrator, I want the system to track health scores over time, so that I can detect trends and identify degrading infrastructure before it causes outages.

#### Acceptance Criteria

1. WHEN a diagnostics run completes, THE Health_Monitor SHALL persist the Health_Score and individual check results to the database with a timestamp.
2. THE Health_Monitor SHALL retain historical Health_Score records for a configurable duration (default: 30 days).
3. WHEN a GET request is received at `/api/diagnostics/ai/history`, THE Diagnostics_Engine SHALL return historical Health_Score data points suitable for trend visualization.
4. THE Diagnostics_Engine SHALL calculate a trend direction (improving, stable, degrading) by comparing the current Health_Score to the rolling average of the previous 24 hours.

### Requirement 3: Auto-Healing Worker

**User Story:** As a panel administrator, I want the system to automatically detect and fix common infrastructure issues, so that minor problems are resolved without manual intervention and service disruptions are minimized.

#### Acceptance Criteria

1. THE Auto_Healer SHALL run as a background worker with a configurable check interval (default: 60 seconds).
2. THE Auto_Healer SHALL detect the following conditions: Stale_Sessions older than a configurable threshold, crashed VPN services (OpenVPN/L2TP/IKEv2), disk usage above a critical threshold, memory usage above a critical threshold, and Nodes that have been offline longer than a configurable threshold.
3. WHEN a Stale_Session is detected, THE Auto_Healer SHALL close the session by updating the radacct stop time and terminate cause.
4. WHEN a crashed VPN service is detected on a Node, THE Auto_Healer SHALL create a node task to restart the affected service.
5. WHEN a Node has been offline beyond the configured threshold and DNS failover is configured for domains pointing to that Node, THE Auto_Healer SHALL trigger the DNS failover process to redirect traffic to a healthy Node.
6. WHEN the Auto_Healer performs any remediation action, THE Auto_Healer SHALL log the action with timestamp, target resource, action taken, and result (success/failure) to the `healing_actions` table.
7. THE Auto_Healer SHALL respect Cooldown_Period constraints: the same remediation action targeting the same resource SHALL NOT execute more than once within the configured cooldown window (default: 5 minutes).

### Requirement 4: Healing Rule Configuration

**User Story:** As a panel administrator, I want to configure which issues are auto-fixed versus alert-only, so that I maintain control over automated actions in my infrastructure.

#### Acceptance Criteria

1. THE Health_Monitor SHALL store Healing_Rules in the database with fields: rule identifier, condition type, action mode (`auto_fix` or `alert_only`), cooldown duration, enabled flag, and thresholds.
2. WHEN a GET request is received at `/api/diagnostics/ai/rules`, THE Health_Monitor SHALL return all configured Healing_Rules.
3. WHEN a PUT request is received at `/api/diagnostics/ai/rules/{id}`, THE Health_Monitor SHALL update the specified Healing_Rule fields (mode, cooldown, enabled, thresholds).
4. WHILE a Healing_Rule is set to `alert_only` mode, THE Auto_Healer SHALL send an alert via the Alert_Engine instead of performing the remediation action.
5. WHILE a Healing_Rule is disabled, THE Auto_Healer SHALL skip evaluation of that rule entirely.

### Requirement 5: Anomaly Detection

**User Story:** As a panel administrator, I want the system to detect unusual patterns such as spikes in failed logins, mass disconnections, or payment failures, so that I am alerted to potential security incidents or infrastructure problems early.

#### Acceptance Criteria

1. THE Alert_Engine SHALL maintain rolling baseline metrics for: failed login attempts per minute, disconnection count per minute, payment failure rate, and data usage rate per user.
2. WHEN the current value of a monitored metric exceeds the rolling baseline by more than a configurable multiplier (default: 3x standard deviation), THE Alert_Engine SHALL classify the event as an Anomaly.
3. WHEN an Anomaly is detected, THE Alert_Engine SHALL record the anomaly with type, detected value, baseline value, timestamp, and severity in the `anomaly_events` table.
4. THE Alert_Engine SHALL evaluate anomalies at a configurable interval (default: 30 seconds).

### Requirement 6: Event Correlation and Root Cause Analysis

**User Story:** As a panel administrator, I want the system to correlate related events and identify probable root causes, so that I receive actionable context instead of isolated alerts.

#### Acceptance Criteria

1. WHEN multiple related events occur within a configurable time window (default: 2 minutes), THE Alert_Engine SHALL group them into a single correlated incident.
2. THE Alert_Engine SHALL apply correlation rules such as: Node offline + multiple user disconnections = node failure, multiple service crashes on same Node = node instability, spike in failed logins from same IP range = potential brute force attack.
3. WHEN a correlated incident is identified, THE Alert_Engine SHALL generate a Root_Cause_Analysis summary describing the probable cause and affected components.
4. THE Alert_Engine SHALL assign a Severity to each correlated incident based on the highest-severity contributing event.

### Requirement 7: Smart Telegram Alerting

**User Story:** As a panel administrator, I want to receive context-rich Telegram alerts with suggested actions, so that I can quickly understand and respond to issues without logging into the panel.

#### Acceptance Criteria

1. WHEN an Anomaly or correlated incident reaches `warning` or `critical` Severity, THE Alert_Engine SHALL send a Telegram notification using the existing Notifier infrastructure.
2. THE Alert_Engine SHALL format alert messages to include: severity icon, incident title, affected components, Root_Cause_Analysis summary (when available), and 1-3 suggested actions.
3. THE Alert_Engine SHALL implement Alert_Deduplication: the same alert type for the same resource SHALL NOT be sent more than once within a configurable suppression window (default: 15 minutes).
4. IF the Telegram notification fails to send, THEN THE Alert_Engine SHALL log the failure and retry up to 3 times with exponential backoff.

### Requirement 8: Health Summary Reports

**User Story:** As a panel administrator, I want to receive periodic health summary reports, so that I can track infrastructure health trends without actively monitoring the panel.

#### Acceptance Criteria

1. THE Health_Monitor SHALL generate a daily health summary report containing: average Health_Score, count of healing actions taken, count of anomalies detected, top 3 recurring issues, and trend direction.
2. THE Health_Monitor SHALL generate a weekly health summary report containing: daily Health_Score chart data, total healing actions with success rate, total anomalies categorized by type, and comparison to previous week.
3. WHEN a scheduled report is generated, THE Health_Monitor SHALL send the report via Telegram to the configured admin chat.
4. THE Health_Monitor SHALL allow configuration of report schedules (daily report hour, weekly report day) via panel settings.

### Requirement 9: Healing Action Audit Log

**User Story:** As a panel administrator, I want a complete audit trail of all auto-healing actions, so that I can review what the system did, verify correctness, and troubleshoot any unintended side effects.

#### Acceptance Criteria

1. WHEN the Auto_Healer performs a remediation action, THE Auto_Healer SHALL record: timestamp, rule identifier, target resource type and ID, action performed, result status (success/partial/failure), error message (if applicable), and execution duration.
2. WHEN a GET request is received at `/api/diagnostics/ai/healing-log`, THE Health_Monitor SHALL return paginated healing action records with filtering by date range, rule type, result status, and target resource.
3. THE Health_Monitor SHALL retain healing action records for a configurable duration (default: 90 days).

### Requirement 10: Future LLM Integration Path

**User Story:** As a panel administrator, I want the system designed so that LLM-powered analysis can be optionally enabled in the future, so that I can upgrade to more sophisticated diagnostics without architectural changes.

#### Acceptance Criteria

1. THE Diagnostics_Engine SHALL define an `Analyzer` interface with methods for generating Root_Cause_Analysis summaries and suggested actions from structured health data.
2. THE Health_Monitor SHALL include a rule-based default implementation of the Analyzer interface that uses pattern matching and threshold logic.
3. WHERE an LLM API endpoint is configured via environment variable `PANEL_LLM_ENDPOINT`, THE Health_Monitor SHALL route analysis requests to the configured LLM provider instead of the rule-based analyzer.
4. THE Analyzer interface SHALL accept structured input (health check results, event history) and return structured output (analysis text, confidence score, suggested actions), ensuring the interface contract is independent of the underlying implementation.
