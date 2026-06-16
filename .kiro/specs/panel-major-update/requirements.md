# Requirements Document

## Introduction

KorisPanel Major Update delivers user management power tools (templates, bulk actions, traffic reset, connection limits, data usage warnings), enhanced customer portal self-service UX, node agent operational maturity (auto-update, config hot-reload, structured logging, diagnostics), and cross-cutting bug fixes (null-safety, stale data, error handling) across the Admin Dashboard, Customer Portal, and Node Agent.

## Glossary

- **Panel_API**: The Go HTTP backend serving REST and WebSocket endpoints at `/api/`
- **Admin_Dashboard**: The Vue 3 admin single-page application for panel administrators
- **Customer_Portal**: The Vue 3 customer-facing single-page application for VPN subscribers
- **Node_Agent**: The Go binary running on each VPN node server that pushes metrics and polls tasks
- **User_Template**: A predefined configuration set (plan, status, RADIUS attributes, connection limits) used to create customers quickly
- **Bulk_Action**: An operation applied simultaneously to multiple selected customer records
- **Connection_Limit**: The maximum number of concurrent VPN sessions allowed for a customer
- **Data_Usage_Warning**: A notification triggered when a customer's consumed traffic reaches a configurable percentage threshold of their plan's data cap
- **Traffic_Reset**: An administrative action that zeroes a customer's accumulated bandwidth counters for the current billing period
- **Config_Hot_Reload**: The ability of the Node_Agent to apply configuration changes from a new environment file without requiring a process restart
- **Auto_Update**: A mechanism by which the Node_Agent checks for and applies new binary versions from the Panel_API without manual intervention
- **Structured_Logging**: JSON-formatted log output with severity levels, timestamps, and contextual fields replacing unstructured log.Printf calls

## Requirements

### Requirement 1: User Templates Management

**User Story:** As an admin, I want to create, edit, and delete user templates so that I can provision new customers quickly with predefined configurations.

#### Acceptance Criteria

1. THE Admin_Dashboard SHALL provide a templates management view accessible from the Settings or Customers section.
2. WHEN an admin creates a user template, THE Panel_API SHALL store the template with fields: name, plan_id, status, connection_limit, RADIUS check attributes, and RADIUS reply attributes.
3. WHEN an admin edits an existing user template, THE Panel_API SHALL update the stored template and return the updated record.
4. WHEN an admin deletes a user template, THE Panel_API SHALL soft-delete the template so that customers previously created from the template remain unaffected.
5. THE Panel_API SHALL enforce unique template names within the same panel instance.
6. WHEN an admin creates a new customer and selects a user template, THE Panel_API SHALL pre-populate the customer record with all fields defined in the selected template.

### Requirement 2: Bulk Actions on Customers

**User Story:** As an admin, I want to perform bulk actions on multiple customers at once so that I can manage large user bases efficiently.

#### Acceptance Criteria

1. THE Admin_Dashboard SHALL provide multi-select capability on the customers list via checkboxes and a select-all toggle.
2. WHEN an admin selects multiple customers and triggers a bulk enable action, THE Panel_API SHALL set the status of each selected customer to "active" and return a summary of successes and failures.
3. WHEN an admin selects multiple customers and triggers a bulk disable action, THE Panel_API SHALL set the status of each selected customer to "disabled" and return a summary of successes and failures.
4. WHEN an admin selects multiple customers and triggers a bulk delete action, THE Panel_API SHALL soft-delete each selected customer by setting deleted_at and status to "deleted" and return a summary of successes and failures.
5. WHEN a bulk action request contains more than 200 customer IDs, THE Panel_API SHALL reject the request with a 400 status code and a descriptive error message.
6. THE Admin_Dashboard SHALL display a confirmation dialog before executing any bulk delete action, showing the count of affected customers.
7. WHEN a bulk action partially fails, THE Panel_API SHALL return a response containing both the list of successfully processed customer IDs and the list of failed customer IDs with individual error reasons.

### Requirement 3: Traffic Reset

**User Story:** As an admin, I want to reset a customer's accumulated traffic counters so that I can grant renewed data allowance without changing the subscription plan.

#### Acceptance Criteria

1. WHEN an admin triggers a traffic reset for a single customer, THE Panel_API SHALL zero the customer's radacct input_octets and output_octets for the current billing period and log the action in the audit_logs table.
2. WHEN an admin triggers a traffic reset, THE Panel_API SHALL record a wallet_transaction entry of type "adjustment" with a description indicating traffic reset for auditability.
3. WHEN a traffic reset is performed on a customer whose status is "limited" due to data cap exhaustion, THE Panel_API SHALL change the customer's status back to "active".
4. THE Admin_Dashboard SHALL provide a traffic reset button on the customer detail view.
5. WHEN an admin triggers a bulk traffic reset for multiple selected customers, THE Panel_API SHALL process each reset individually and return a summary of successes and failures.

### Requirement 4: Connection Limit Enforcement

**User Story:** As an admin, I want to set and enforce concurrent connection limits per customer so that a single account cannot consume excessive resources.

#### Acceptance Criteria

1. WHEN an admin sets a connection limit for a customer, THE Panel_API SHALL store the value as a RADIUS check attribute (Simultaneous-Use) in the radcheck table.
2. WHILE a customer has an active connection limit, THE Panel_API SHALL instruct FreeRADIUS to reject new session authentication requests when the customer's concurrent session count equals the configured limit.
3. THE Admin_Dashboard SHALL display the current connection limit on the customer detail view and allow inline editing.
4. WHEN a connection limit is set to zero, THE Panel_API SHALL interpret the value as unlimited concurrent sessions and remove the Simultaneous-Use attribute from radcheck.
5. THE Customer_Portal SHALL display the customer's connection limit and current active session count on the Usage view.

### Requirement 5: Data Usage Warnings and Notifications

**User Story:** As an admin, I want to configure data usage warning thresholds so that customers receive notifications before hitting their data cap and service is degraded gracefully.

#### Acceptance Criteria

1. THE Panel_API SHALL support configurable warning thresholds as percentages of the customer's plan data cap (default thresholds: 80% and 95%).
2. WHEN a customer's accumulated traffic reaches a configured warning threshold, THE Panel_API SHALL create an event record of type "data_warning" with severity "warning".
3. WHEN a data_warning event is created, THE Panel_API SHALL dispatch a notification to the customer via the configured notification channels (Telegram bot, email if configured).
4. THE Customer_Portal SHALL display active data usage warnings on the Dashboard view with the percentage consumed and remaining data in human-readable units.
5. THE Customer_Portal SHALL display a persistent alert banner on all views when the customer's usage exceeds 95% of the plan data cap.
6. WHEN a customer's traffic exceeds 100% of the plan data cap, THE Panel_API SHALL update the customer status to "limited" and log the event.
7. THE Admin_Dashboard SHALL provide a settings interface to configure warning threshold percentages at the global panel level.

### Requirement 6: Portal Self-Service UX Enhancements

**User Story:** As a customer, I want clear visibility into my usage limits, remaining data, and connection status so that I can self-manage my VPN subscription.

#### Acceptance Criteria

1. THE Customer_Portal SHALL display remaining data allowance as both a percentage and absolute value (e.g., "2.4 GB remaining / 10 GB") on the Dashboard view.
2. THE Customer_Portal SHALL display a visual progress bar representing data consumption relative to the plan cap on the Dashboard view.
3. THE Customer_Portal SHALL display the subscription expiry date and days remaining on the Dashboard view.
4. WHEN the customer's remaining data falls below 20% of the plan cap, THE Customer_Portal SHALL render the progress bar in a warning color (amber).
5. WHEN the customer's remaining data falls below 5% of the plan cap, THE Customer_Portal SHALL render the progress bar in a critical color (red).
6. THE Customer_Portal SHALL display active session count and connection limit on the Usage view.
7. THE Customer_Portal SHALL display a notification center showing recent data_warning events and account status changes.

### Requirement 7: Node Agent Auto-Update

**User Story:** As a panel operator, I want node agents to update themselves automatically so that I do not need to SSH into each node for manual upgrades.

#### Acceptance Criteria

1. WHEN the Node_Agent starts and at each configurable check interval (default: 6 hours), THE Node_Agent SHALL query the Panel_API for the latest available agent version.
2. WHEN the Panel_API reports a newer version than the currently running binary, THE Node_Agent SHALL download the new binary from the Panel_API update endpoint.
3. WHEN the Node_Agent downloads a new binary, THE Node_Agent SHALL verify the binary integrity using a SHA-256 checksum provided by the Panel_API.
4. IF the checksum verification fails, THEN THE Node_Agent SHALL discard the downloaded binary, log an error with the expected and actual checksums, and retry at the next check interval.
5. WHEN the checksum verification succeeds, THE Node_Agent SHALL replace the running binary and restart itself via the systemd service manager.
6. THE Panel_API SHALL expose an endpoint that returns the latest agent version string and the binary download URL with checksum.
7. WHERE the operator sets the environment variable NODE_AUTO_UPDATE=false, THE Node_Agent SHALL skip all auto-update checks.

### Requirement 8: Node Agent Config Hot-Reload

**User Story:** As a panel operator, I want the node agent to reload configuration changes without restarting so that service continuity is maintained during reconfiguration.

#### Acceptance Criteria

1. WHEN the Node_Agent receives a SIGHUP signal, THE Node_Agent SHALL re-read the environment configuration file and apply changed values without terminating the process.
2. WHEN the Panel_API dispatches a task with action "agent.reload_config", THE Node_Agent SHALL re-read the environment configuration file and apply changed values.
3. WHEN configuration is reloaded, THE Node_Agent SHALL log the previous and new values for each changed configuration key at info severity level.
4. IF the reloaded configuration file is malformed or missing required keys, THEN THE Node_Agent SHALL retain the current configuration, log an error with the specific validation failure, and continue operating.
5. THE Node_Agent SHALL support hot-reload for the following configuration keys: PANEL_URL, NODE_INTERVAL, NODE_AUTO_UPDATE, and LOG_LEVEL.

### Requirement 9: Node Agent Structured Logging and Diagnostics

**User Story:** As a panel operator, I want the node agent to produce structured JSON logs with severity levels so that I can integrate with log aggregation systems and diagnose issues efficiently.

#### Acceptance Criteria

1. THE Node_Agent SHALL output all log messages in JSON format with fields: timestamp (ISO 8601), level (debug, info, warn, error), message, and contextual key-value pairs.
2. THE Node_Agent SHALL support a configurable log level via the LOG_LEVEL environment variable (default: info).
3. WHEN the log level is set to "debug", THE Node_Agent SHALL include detailed request/response information for Panel_API communications.
4. THE Node_Agent SHALL include a diagnostics report in each status push containing: agent version, uptime in seconds, Go runtime version, number of goroutines, and memory allocation in bytes.
5. WHEN the Node_Agent fails to reach the Panel_API for 3 consecutive push intervals, THE Node_Agent SHALL log an error at "error" level with the cumulative disconnection duration and last error message.
6. THE Panel_API SHALL store the latest diagnostics report from each node and expose the data via the admin nodes detail endpoint.

### Requirement 10: Cross-Cutting Bug Fixes — Null Safety

**User Story:** As a developer, I want all nullable database fields to be handled safely throughout the codebase so that null pointer dereferences and unexpected nil values do not cause runtime panics or frontend rendering errors.

#### Acceptance Criteria

1. THE Panel_API SHALL use sql.NullString, sql.NullInt64, sql.NullFloat64, and sql.NullTime for all nullable database columns when scanning query results.
2. THE Panel_API SHALL serialize nullable fields as JSON null (not omitted) when the value is not present, ensuring the frontend receives a consistent schema.
3. THE Admin_Dashboard SHALL render fallback values (e.g., "—" or "N/A") for any nullable field that arrives as null in API responses.
4. THE Customer_Portal SHALL render fallback values for any nullable field that arrives as null in API responses.

### Requirement 11: Cross-Cutting Bug Fixes — Stale Data

**User Story:** As a user of the admin or customer portal, I want data displays to reflect the current server state so that I do not make decisions based on outdated information.

#### Acceptance Criteria

1. WHEN navigating to a view that fetches data from the Panel_API, THE Admin_Dashboard SHALL invalidate cached data older than 30 seconds and re-fetch from the server.
2. WHEN navigating to a view that fetches data from the Panel_API, THE Customer_Portal SHALL invalidate cached data older than 30 seconds and re-fetch from the server.
3. WHEN a mutation (create, update, delete) API call succeeds, THE Admin_Dashboard SHALL immediately invalidate and refetch the affected resource list.
4. WHEN a mutation API call succeeds, THE Customer_Portal SHALL immediately invalidate and refetch the affected resource data.
5. THE Panel_API SHALL include a Cache-Control: no-store header on all JSON API responses to prevent browser-level caching of dynamic data.

### Requirement 12: Cross-Cutting Bug Fixes — Error Handling

**User Story:** As a user, I want clear error feedback when operations fail so that I understand what went wrong and can take corrective action.

#### Acceptance Criteria

1. WHEN the Panel_API encounters an internal error, THE Panel_API SHALL return a JSON error response with fields: error (human-readable message), code (machine-readable error code), and status (HTTP status code).
2. THE Panel_API SHALL log all 5xx errors with full stack context including request path, method, authenticated user, and request ID.
3. WHEN an API request fails, THE Admin_Dashboard SHALL display a toast notification with the error message from the response body.
4. WHEN an API request fails, THE Customer_Portal SHALL display a toast notification with the error message from the response body.
5. IF the Panel_API returns a 401 status code, THEN THE Admin_Dashboard SHALL redirect the user to the login view and clear session state.
6. IF the Panel_API returns a 401 status code, THEN THE Customer_Portal SHALL redirect the user to the login view and clear session state.
7. WHEN the Node_Agent receives a non-2xx response from the Panel_API, THE Node_Agent SHALL log the response status code, response body (truncated to 512 bytes), and the request URL at "warn" severity level.
