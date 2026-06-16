# Requirements Document

## Introduction

The DNS Failover Profiles feature enables transparent VPN server migration by replacing direct IP addresses with domain names in OpenVPN configuration files. When a VPN node becomes blocked or unreachable, an administrator (or automated health check worker) updates the DNS A record to point to a different node. Existing clients reconnect automatically via DNS re-resolution without needing to re-download their profiles. The system supports manual failover, optional Cloudflare API integration for programmatic DNS updates, and an auto-failover worker that detects offline nodes.

## Glossary

- **Panel**: The admin web application that manages VPN nodes, users, and configurations
- **Node**: A VPN server instance running OpenVPN, registered in the panel database
- **Failover_Domain**: A DNS domain name mapped to a target node for use in VPN profiles, stored in the `failover_domains` table
- **DNS_Provider**: A configured DNS service integration (Cloudflare or manual) used to update A records programmatically
- **Failover_Event**: An audit record tracking a single failover action through its lifecycle (pending → propagating → completed/failed)
- **Failover_Orchestrator**: The component that coordinates the full failover lifecycle from trigger to DNS propagation verification
- **Auto_Failover_Worker**: A background process that monitors node health and triggers automatic failover when nodes go offline
- **Profile_Generator**: The component that produces `.ovpn` configuration files for VPN clients
- **TTL**: Time To Live — the number of seconds DNS resolvers cache a record before re-querying
- **Propagation**: The process of DNS changes becoming visible across all DNS resolvers
- **DNSUpdater**: The interface abstracting DNS record management across providers (Cloudflare API or manual)
- **Node_Push**: A periodic heartbeat sent from the node agent to the panel API indicating the node is alive

## Requirements

### Requirement 1: DNS Provider Management

**User Story:** As an administrator, I want to configure DNS providers with API credentials, so that the panel can programmatically update DNS records during failover.

#### Acceptance Criteria

1. WHEN an administrator creates a DNS provider, THE Panel SHALL validate that the name is unique and the type is either "cloudflare" or "manual"
2. WHEN a DNS provider of type "cloudflare" is created, THE Panel SHALL require an API token and zone ID
3. THE Panel SHALL encrypt DNS provider API tokens at rest using AES-256-GCM before storing them in the database
4. WHEN any API response includes a DNS_Provider object, THE Panel SHALL omit the API token field from the response
5. WHEN an administrator tests a DNS provider connection, THE Panel SHALL verify API credentials by calling the provider API and return a success or failure status
6. WHEN an administrator deletes a DNS provider that is referenced by active Failover_Domains, THE Panel SHALL reject the deletion with an appropriate error
7. WHEN a Cloudflare API token is invalid or lacks permissions, THE Panel SHALL mark the provider as inactive and log the error

### Requirement 2: Failover Domain Management

**User Story:** As an administrator, I want to create and manage failover domains mapped to VPN nodes, so that client profiles use domain names enabling transparent server migration.

#### Acceptance Criteria

1. WHEN an administrator creates a Failover_Domain, THE Panel SHALL validate that the domain is a valid FQDN conforming to RFC 1035 and is unique among active Failover_Domains
2. WHEN an administrator creates a Failover_Domain, THE Panel SHALL validate that the referenced current_node_id exists in the nodes table
3. THE Panel SHALL enforce a TTL minimum of 30 seconds and maximum of 86400 seconds for all Failover_Domains, defaulting to 60 seconds
4. WHEN an administrator deactivates a Failover_Domain, THE Panel SHALL stop using that domain in profile generation and exclude it from auto-failover health checks
5. WHEN an administrator deletes a Failover_Domain, THE Panel SHALL remove the domain record and disassociate it from the node

### Requirement 3: Manual Failover Trigger

**User Story:** As an administrator, I want to trigger a failover to move a domain from one node to another, so that I can respond to node blocking or maintenance needs.

#### Acceptance Criteria

1. WHEN an administrator triggers a failover for a Failover_Domain, THE Failover_Orchestrator SHALL validate that the target node is different from the current node
2. WHEN an administrator triggers a failover to a node that is offline, THE Failover_Orchestrator SHALL reject the request with an error indicating the target node is not available
3. WHEN a failover is triggered while another failover is pending or propagating for the same domain, THE Failover_Orchestrator SHALL reject the request with a 409 conflict status
4. WHEN a valid failover is triggered, THE Failover_Orchestrator SHALL create a Failover_Event record with status "pending" and proceed to update the DNS record
5. WHEN the DNS update succeeds, THE Failover_Orchestrator SHALL update the Failover_Event status to "propagating" and update the Failover_Domain current_node_id to the target node
6. WHEN the DNS update fails, THE Failover_Orchestrator SHALL mark the Failover_Event status as "failed" and store the error message

### Requirement 4: DNS Propagation Verification

**User Story:** As an administrator, I want the system to verify that DNS changes have propagated, so that I have confidence the failover is complete.

#### Acceptance Criteria

1. WHEN a Failover_Event enters "propagating" status, THE Failover_Orchestrator SHALL poll DNS resolvers every 10 seconds to check if the domain resolves to the expected new IP
2. WHEN DNS propagation is confirmed (domain resolves to the new IP), THE Failover_Orchestrator SHALL mark the Failover_Event status as "completed" and record the propagation completion timestamp
3. WHEN DNS propagation is not confirmed within the configured propagation timeout, THE Failover_Orchestrator SHALL mark the Failover_Event status as "failed"
4. WHEN propagation times out, THE Failover_Orchestrator SHALL keep the DNS record pointing to the new IP and not revert the change

### Requirement 5: Failover Rollback

**User Story:** As an administrator, I want to rollback a completed failover, so that I can revert to the previous node if the failover was unnecessary or the original node is restored.

#### Acceptance Criteria

1. WHEN an administrator triggers a rollback for a completed Failover_Event, THE Failover_Orchestrator SHALL initiate a new failover back to the original source node
2. WHEN a rollback is triggered, THE Failover_Orchestrator SHALL create a new Failover_Event with reason "auto_rollback" and triggered_by matching the initiator
3. IF the original source node is offline at rollback time, THEN THE Failover_Orchestrator SHALL reject the rollback with an error

### Requirement 6: OpenVPN Profile Generation with Failover Domains

**User Story:** As a VPN customer, I want my profile to use a domain name so that I automatically reconnect to a new server when failover occurs without re-downloading the profile.

#### Acceptance Criteria

1. WHEN generating an OpenVPN profile for a node that has an active Failover_Domain, THE Profile_Generator SHALL use the Failover_Domain name as the remote host
2. WHEN generating an OpenVPN profile for a node with both an active Failover_Domain and a node-level domain field, THE Profile_Generator SHALL prioritize the Failover_Domain over the node domain
3. WHEN generating an OpenVPN profile for a node without an active Failover_Domain, THE Profile_Generator SHALL fall back to the node domain field or public IP (existing behavior)
4. THE Profile_Generator SHALL include the "resolv-retry infinite" directive in all generated profiles to enable automatic DNS re-resolution on connection loss

### Requirement 7: Auto-Failover Worker

**User Story:** As an administrator, I want the system to automatically detect offline nodes and trigger failover, so that VPN service continuity is maintained without manual intervention.

#### Acceptance Criteria

1. WHILE the dns_failover_enabled setting is "true", THE Auto_Failover_Worker SHALL run periodic health checks at the interval specified by dns_failover_check_interval
2. WHILE the dns_failover_enabled setting is "false", THE Auto_Failover_Worker SHALL remain idle and not perform health checks
3. WHEN a node with an active Failover_Domain has not sent a Node_Push within twice the check interval, THE Auto_Failover_Worker SHALL consider that node offline
4. WHEN a node is detected as offline for two consecutive health check cycles, THE Auto_Failover_Worker SHALL trigger an automatic failover for domains pointing to that node
5. WHEN selecting a fallback node, THE Auto_Failover_Worker SHALL prefer online nodes that are not already targeted by another active Failover_Domain
6. IF no alternative online nodes are available for failover, THEN THE Auto_Failover_Worker SHALL log a warning, send a notification, and not change the DNS record
7. WHEN an auto-failover is triggered, THE Auto_Failover_Worker SHALL create a Failover_Event with triggered_by set to "auto" and reason indicating the detection cause

### Requirement 8: Failover Event Audit Trail

**User Story:** As an administrator, I want a complete audit log of all failover events, so that I can review history and troubleshoot issues.

#### Acceptance Criteria

1. WHEN any change occurs to a Failover_Domain current_node_id, THE Panel SHALL have a corresponding Failover_Event record documenting the change
2. THE Panel SHALL record the source node, target node, reason, trigger source, and timestamps for every Failover_Event
3. WHEN an administrator views failover events, THE Panel SHALL support filtering by domain, status, and trigger type
4. THE Failover_Event status SHALL only transition through valid states: pending to propagating to completed, pending to propagating to failed, pending to failed, or completed to rolled_back

### Requirement 9: Cloudflare API Integration

**User Story:** As an administrator, I want the system to update Cloudflare DNS records automatically during failover, so that I do not need to manually change records in the Cloudflare dashboard.

#### Acceptance Criteria

1. WHEN a failover is triggered for a domain with a Cloudflare DNS_Provider, THE DNSUpdater SHALL call the Cloudflare API to update the A record to the target node IP with the configured TTL and proxied set to false
2. WHEN the Cloudflare API returns a 429 rate limit response, THE DNSUpdater SHALL apply exponential backoff and retry up to 3 times
3. WHEN the Cloudflare API returns a 5xx server error, THE DNSUpdater SHALL retry up to 3 times with exponential backoff
4. WHEN a failover is triggered for a domain with a manual DNS_Provider (or no provider), THE Panel SHALL log instructions for manual DNS update and mark the event as propagating without making API calls

### Requirement 10: Real-time Failover Status Updates

**User Story:** As an administrator, I want real-time updates on failover progress, so that I can monitor the status without refreshing the page.

#### Acceptance Criteria

1. WHEN a failover event changes status, THE Panel SHALL broadcast a WebSocket event to connected admin clients with the event type and relevant details
2. WHEN a failover starts, THE Panel SHALL send a "failover:started" event containing the domain ID, source node, and target node
3. WHEN a failover completes, THE Panel SHALL send a "failover:completed" event containing the domain ID, event ID, and duration in seconds
4. WHEN a failover fails, THE Panel SHALL send a "failover:failed" event containing the domain ID, event ID, and error description

### Requirement 11: Notification on Failover Events

**User Story:** As an administrator, I want to receive notifications when failover events occur, so that I am aware of service changes even when not actively monitoring the dashboard.

#### Acceptance Criteria

1. WHEN a failover event is triggered (manual or automatic), THE Panel SHALL send a Telegram notification to the configured admin channel
2. WHEN a failover event fails, THE Panel SHALL send a Telegram notification with the error details
3. WHEN the Auto_Failover_Worker detects no available fallback nodes, THE Panel SHALL send a Telegram notification alerting the administrator

### Requirement 12: Auto-Rollback on Original Node Recovery

**User Story:** As an administrator, I want the option for automatic rollback when the original node comes back online, so that traffic returns to the preferred node without manual intervention.

#### Acceptance Criteria

1. WHILE the dns_failover_auto_rollback setting is "true", THE Auto_Failover_Worker SHALL monitor the original node of completed auto-failovers
2. WHEN the original node that triggered an auto-failover comes back online and dns_failover_auto_rollback is enabled, THE Auto_Failover_Worker SHALL trigger a reverse failover back to the original node
3. WHEN an auto-rollback is triggered, THE Auto_Failover_Worker SHALL create a Failover_Event with reason "auto_rollback"
