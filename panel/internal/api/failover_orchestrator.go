package api

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	"KorisPanel/panel/internal/notify"
)

// FailoverOrchestrator coordinates a failover event through its full lifecycle:
// trigger → DNS update → propagation verification → completion.
type FailoverOrchestrator struct {
	db                 *sql.DB
	notifier           *notify.Notifier
	propagationTimeout time.Duration
	checkInterval      time.Duration
}

// NewFailoverOrchestrator creates a new orchestrator with the given dependencies.
// propagationTimeout controls how long propagation checking waits before marking failed.
// checkInterval controls how often DNS propagation is polled (typically 10s).
func NewFailoverOrchestrator(db *sql.DB, notifier *notify.Notifier, propagationTimeout, checkInterval time.Duration) *FailoverOrchestrator {
	return &FailoverOrchestrator{
		db:                 db,
		notifier:           notifier,
		propagationTimeout: propagationTimeout,
		checkInterval:      checkInterval,
	}
}

// TriggerFailover initiates a failover for a domain to a new node.
// It validates preconditions, updates DNS, and launches background propagation checking.
func (fo *FailoverOrchestrator) TriggerFailover(ctx context.Context, domainID int64, toNodeID int64, reason string, triggeredBy string) (*FailoverEvent, error) {
	// Load the domain
	var domain FailoverDomain
	var providerID sql.NullInt64
	var lastFailover sql.NullString
	var dnsRecordID sql.NullString
	err := fo.db.QueryRowContext(ctx, `
		SELECT id, domain, current_node_id, dns_provider_id, dns_record_id, ttl, is_active, last_failover_at
		FROM failover_domains WHERE id = $1`, domainID).Scan(
		&domain.ID, &domain.Domain, &domain.CurrentNodeID, &providerID, &dnsRecordID,
		&domain.TTL, &domain.IsActive, &lastFailover,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("domain_not_found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load domain: %w", err)
	}
	if providerID.Valid {
		domain.DNSProviderID = &providerID.Int64
	}
	if dnsRecordID.Valid {
		domain.DNSRecordID = dnsRecordID.String
	}

	// Requirement 3.1: Validate target != current node
	if toNodeID == domain.CurrentNodeID {
		return nil, fmt.Errorf("same_node")
	}

	// Requirement 3.2: Validate target node is online
	online, targetNodeIP, err := fo.isNodeOnline(ctx, toNodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to check target node: %w", err)
	}
	if !online {
		return nil, fmt.Errorf("node_offline")
	}

	// Requirement 3.3: Check no concurrent failover pending/propagating for this domain
	var concurrentCount int
	err = fo.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM failover_events WHERE domain_id = $1 AND status IN ('pending','propagating')`, domainID).Scan(&concurrentCount)
	if err != nil {
		return nil, fmt.Errorf("failed to check concurrent failovers: %w", err)
	}
	if concurrentCount > 0 {
		return nil, fmt.Errorf("failover_in_progress")
	}

	// Step 4: Create failover_event with status "pending"
	fromNodeID := domain.CurrentNodeID
	res, err := fo.db.ExecContext(ctx,
		`INSERT INTO failover_events(domain_id, from_node_id, to_node_id, reason, status, triggered_by)
		 VALUES($1, $2, $3, $4, 'pending', $5)`,
		domainID, fromNodeID, toNodeID, reason, triggeredBy)
	if err != nil {
		return nil, fmt.Errorf("failed to create failover event: %w", err)
	}
	eventID, _ := res.LastInsertId()

	// Build the event struct to return
	event := &FailoverEvent{
		ID:          eventID,
		DomainID:    domainID,
		FromNodeID:  fromNodeID,
		ToNodeID:    toNodeID,
		Reason:      reason,
		Status:      "pending",
		TriggeredBy: triggeredBy,
	}

	// Step 5: Build DNSUpdater based on provider type
	updater := fo.buildDNSUpdater(ctx, domain.DNSProviderID, domain.DNSRecordID)

	// Step 6: Call updater.UpdateARecord with new node's public_ip and domain's TTL
	err = updater.UpdateARecord(ctx, domain.Domain, targetNodeIP, domain.TTL)
	if err != nil {
		// Step 8: On DNS failure: mark event "failed" with error message
		errMsg := err.Error()
		event.Status = "failed"
		event.ErrorMessage = &errMsg
		_, _ = fo.db.ExecContext(ctx,
			`UPDATE failover_events SET status = 'failed', error_message = $1 WHERE id = $2`,
			errMsg, eventID)

		// Send notification on failure
		fo.notifier.Send(fmt.Sprintf("🔴 *DNS Failover Failed*\nDomain: `%s`\nError: %s", domain.Domain, errMsg))
		return event, nil
	}

	// Step 7: On DNS success: update event to "propagating", update domain's current_node_id and last_failover_at
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, _ = fo.db.ExecContext(ctx,
		`UPDATE failover_events SET status = 'propagating', dns_propagation_started_at = $1 WHERE id = $2`,
		now, eventID)
	_, _ = fo.db.ExecContext(ctx,
		`UPDATE failover_domains SET current_node_id = $1, last_failover_at = $2 WHERE id = $3`,
		toNodeID, now, domainID)

	event.Status = "propagating"
	event.DNSPropagationStartedAt = &now

	// Step 9: Launch background goroutine for CheckPropagation
	go fo.CheckPropagation(eventID, domain.Domain, targetNodeIP, domain.DNSProviderID, domain.DNSRecordID)

	// Step 10: Send Telegram notification
	var fromNodeName, toNodeName string
	_ = fo.db.QueryRow(`SELECT COALESCE(name,'') FROM nodes WHERE id = $1`, fromNodeID).Scan(&fromNodeName)
	_ = fo.db.QueryRow(`SELECT COALESCE(name,'') FROM nodes WHERE id = $1`, toNodeID).Scan(&toNodeName)

	fo.notifier.Send(fmt.Sprintf("🔄 *DNS Failover Started*\nDomain: `%s`\nFrom: %s → To: %s\nReason: %s\nTriggered by: %s",
		domain.Domain, fromNodeName, toNodeName, reason, triggeredBy))

	// Step 11: Return the event
	return event, nil
}

// CheckPropagation polls DNS every checkInterval until the new IP is visible or timeout.
// It runs as a background goroutine launched by TriggerFailover.
func (fo *FailoverOrchestrator) CheckPropagation(eventID int64, domain string, expectedIP string, providerID *int64, recordID string) {
	ctx := context.Background()
	updater := fo.buildDNSUpdater(ctx, providerID, recordID)

	deadline := time.Now().Add(fo.propagationTimeout)
	ticker := time.NewTicker(fo.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check if we've exceeded the timeout
			if time.Now().After(deadline) {
				// Requirement 4.3/4.4: Mark event "failed" on timeout but keep DNS pointing to new IP
				errMsg := "propagation timeout: DNS did not resolve to expected IP within timeout period"
				_, _ = fo.db.Exec(
					`UPDATE failover_events SET status = 'failed', error_message = $1 WHERE id = $2`,
					errMsg, eventID)

				fo.notifier.Send(fmt.Sprintf("⚠️ *DNS Propagation Timeout*\nDomain: `%s`\nExpected IP: %s\nDNS record was updated but propagation was not confirmed within %s.",
					domain, expectedIP, fo.propagationTimeout))

				log.Printf("[failover] propagation timeout for event %d, domain %s", eventID, domain)
				return
			}

			// Poll DNS to check propagation
			propagated, err := updater.VerifyPropagation(ctx, domain, expectedIP)
			if err != nil {
				log.Printf("[failover] propagation check error for event %d: %v", eventID, err)
				continue // Keep trying on transient errors
			}

			if propagated {
				// Requirement 4.2: Mark event "completed" and record completion timestamp
				now := time.Now().UTC().Format("2006-01-02 15:04:05")
				_, _ = fo.db.Exec(
					`UPDATE failover_events SET status = 'completed', dns_propagation_completed_at = $1 WHERE id = $2`,
					now, eventID)

				fo.notifier.Send(fmt.Sprintf("✅ *DNS Failover Completed*\nDomain: `%s`\nNow resolves to: %s",
					domain, expectedIP))

				log.Printf("[failover] propagation confirmed for event %d, domain %s → %s", eventID, domain, expectedIP)
				return
			}
		}
	}
}

// Rollback reverts a completed failover by triggering a new failover back to the original node.
func (fo *FailoverOrchestrator) Rollback(ctx context.Context, eventID int64) (*FailoverEvent, error) {
	// Step 1: Load the original event
	var originalEvent FailoverEvent
	var errMsg sql.NullString
	err := fo.db.QueryRowContext(ctx,
		`SELECT id, domain_id, from_node_id, to_node_id, reason, status, triggered_by, error_message
		 FROM failover_events WHERE id = $1`, eventID).Scan(
		&originalEvent.ID, &originalEvent.DomainID, &originalEvent.FromNodeID,
		&originalEvent.ToNodeID, &originalEvent.Reason, &originalEvent.Status,
		&originalEvent.TriggeredBy, &errMsg,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("event_not_found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load event: %w", err)
	}

	// Only allow rollback of completed events
	if originalEvent.Status != "completed" {
		return nil, fmt.Errorf("invalid_status: can only rollback completed events")
	}

	// Step 2: Get the from_node (original node to rollback to)
	rollbackToNodeID := originalEvent.FromNodeID

	// Step 3: Validate from_node is online
	online, _, err := fo.isNodeOnline(ctx, rollbackToNodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to check original node: %w", err)
	}
	if !online {
		return nil, fmt.Errorf("node_offline: original node is not available for rollback")
	}

	// Mark the original event as rolled_back
	_, _ = fo.db.ExecContext(ctx,
		`UPDATE failover_events SET status = 'rolled_back' WHERE id = $1`, eventID)

	// Trigger a new failover back to the from_node with reason "rollback"
	return fo.TriggerFailover(ctx, originalEvent.DomainID, rollbackToNodeID, "rollback", originalEvent.TriggeredBy)
}

// isNodeOnline checks if a node is online by verifying:
// 1. The node exists and is not disabled
// 2. The node's last push was within 5 minutes
// Returns: online status, public IP of the node, and any error.
func (fo *FailoverOrchestrator) isNodeOnline(ctx context.Context, nodeID int64) (bool, string, error) {
	var status, publicIP string
	var lastSeenAt sql.NullTime
	err := fo.db.QueryRowContext(ctx,
		`SELECT n.status, n.public_ip, ns.updated_at
		 FROM nodes n
		 LEFT JOIN node_status ns ON ns.node_id = n.id
		 WHERE n.id = $1`, nodeID).Scan(&status, &publicIP, &lastSeenAt)
	if err == sql.ErrNoRows {
		return false, "", fmt.Errorf("node not found: %d", nodeID)
	}
	if err != nil {
		return false, "", err
	}

	// Node must not be disabled
	if status == "disabled" {
		return false, publicIP, nil
	}

	// Check last push was within 5 minutes
	if !lastSeenAt.Valid {
		return false, publicIP, nil
	}
	if time.Since(lastSeenAt.Time) > 5*time.Minute {
		return false, publicIP, nil
	}

	return true, publicIP, nil
}

// buildDNSUpdater creates the appropriate DNSUpdater based on provider configuration.
func (fo *FailoverOrchestrator) buildDNSUpdater(ctx context.Context, providerID *int64, recordID string) DNSUpdater {
	if providerID == nil || *providerID == 0 {
		return &ManualUpdater{}
	}

	var providerType, tokenEncrypted, zoneID string
	err := fo.db.QueryRow(
		`SELECT type, COALESCE(api_token_encrypted,''), COALESCE(zone_id,'') FROM dns_providers WHERE id = $1 AND is_active = TRUE`,
		*providerID).Scan(&providerType, &tokenEncrypted, &zoneID)
	if err != nil {
		log.Printf("[failover] failed to load DNS provider %d: %v", *providerID, err)
		return &ManualUpdater{}
	}

	if providerType != "cloudflare" {
		return &ManualUpdater{}
	}

	// Decrypt the API token
	apiToken, err := decryptToken(tokenEncrypted)
	if err != nil {
		log.Printf("[failover] failed to decrypt API token for provider %d: %v", *providerID, err)
		return &ManualUpdater{}
	}

	return NewCloudflareUpdater(apiToken, zoneID, recordID)
}

// GetPropagationTimeout returns the propagation timeout from panel_settings.
func GetPropagationTimeoutFromDB(db *sql.DB) time.Duration {
	var val string
	err := db.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = 'dns_failover_propagation_timeout'`).Scan(&val)
	if err != nil {
		return 300 * time.Second // default 5 minutes
	}
	seconds, err := strconv.Atoi(val)
	if err != nil || seconds <= 0 {
		return 300 * time.Second
	}
	return time.Duration(seconds) * time.Second
}
