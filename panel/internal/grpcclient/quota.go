package grpcclient

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"KorisPanel/panel/internal/dbstore"
)

// QuotaEnforcer checks whether a user's cumulative traffic exceeds their
// configured quota (max_data_bytes). When a user exceeds the quota, it calls
// SyncUsers with enabled=false on all relevant nodes to cut off access.
type QuotaEnforcer struct {
	syncService *UserSyncService
	store       dbstore.Store
}

// NewQuotaEnforcer creates a QuotaEnforcer that uses the given UserSyncService
// and database store for quota checks and enforcement.
func NewQuotaEnforcer(syncService *UserSyncService, store dbstore.Store) *QuotaEnforcer {
	return &QuotaEnforcer{
		syncService: syncService,
		store:       store,
	}
}

// CheckQuota verifies whether the user identified by userID/username has exceeded
// their data quota. If max_data_bytes > 0 and cumulative usage exceeds it, the user
// is disabled on all relevant nodes via SyncUsers.
//
// Returns true if the user was disabled due to exceeding quota, false otherwise.
func (q *QuotaEnforcer) CheckQuota(ctx context.Context, userID int64, username string) (bool, error) {
	// 1. Get the user's max_data_bytes from radcheck (Max-Data attribute)
	maxDataBytes, err := q.getMaxDataBytes(ctx, username)
	if err != nil {
		return false, fmt.Errorf("quota: get max_data_bytes for %q: %w", username, err)
	}

	// If max_data_bytes is 0 or not set, quota is unlimited — skip check
	if maxDataBytes <= 0 {
		return false, nil
	}

	// 2. Query cumulative rx+tx from user_traffic_log
	totalUsage, err := q.getCumulativeUsage(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("quota: get cumulative usage for user %d: %w", userID, err)
	}

	// 3. Compare: if usage exceeds quota, disable the user on nodes
	if totalUsage > maxDataBytes {
		log.Printf("[grpc-client] Quota exceeded for user %q (id=%d): used %d / limit %d bytes — disabling",
			username, userID, totalUsage, maxDataBytes)

		// Call SyncUser which will rebuild payload from DB state.
		// The user's status should reflect the over-quota state. However, to ensure
		// the user is disabled even if status hasn't been updated yet, we push directly
		// with enabled=false using the sync service.
		if err := q.disableUserOnNodes(ctx, username); err != nil {
			log.Printf("[grpc-client] Quota enforcement: failed to disable user %q on nodes: %v", username, err)
			return true, err
		}

		return true, nil
	}

	return false, nil
}

// getMaxDataBytes queries the user's Max-Data attribute from the radcheck table.
// Returns 0 if no Max-Data attribute is set (unlimited).
func (q *QuotaEnforcer) getMaxDataBytes(ctx context.Context, username string) (int64, error) {
	db := q.store.DB()

	var maxDataStr string
	err := db.QueryRowContext(ctx,
		`SELECT value FROM radcheck WHERE username = ? AND attribute = 'Max-Data' ORDER BY id DESC LIMIT 1`,
		username,
	).Scan(&maxDataStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	var maxDataBytes int64
	fmt.Sscanf(maxDataStr, "%d", &maxDataBytes)
	return maxDataBytes, nil
}

// getCumulativeUsage queries the total rx+tx bytes from user_traffic_log for a user.
func (q *QuotaEnforcer) getCumulativeUsage(ctx context.Context, userID int64) (int64, error) {
	db := q.store.DB()

	var total int64
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(rx_bytes + tx_bytes), 0) FROM user_traffic_log WHERE user_id = ?`,
		userID,
	).Scan(&total)
	if err != nil {
		return 0, err
	}

	return total, nil
}

// disableUserOnNodes calls SyncUser which will rebuild the payload from the current
// DB state. Since the user is over quota, we need to ensure enabled=false is pushed.
// We use the SyncUser method which already handles fan-out and retries.
func (q *QuotaEnforcer) disableUserOnNodes(ctx context.Context, username string) error {
	// SyncUser rebuilds the payload. If the customer's status or quota state
	// reflects "over quota", the payload will have enabled=false.
	// However, since the existing buildPayload checks status and expiry but not
	// cumulative traffic directly, we update the customer status to "limited"
	// before syncing to ensure enabled=false is propagated.
	db := q.store.DB()

	_, err := db.ExecContext(ctx,
		`UPDATE customers SET status = 'limited' WHERE username = ? AND status NOT IN ('disabled', 'expired', 'limited') AND deleted_at IS NULL`,
		username,
	)
	if err != nil {
		return fmt.Errorf("update customer status to limited: %w", err)
	}

	// Now SyncUser will pick up the "limited" status and set enabled=false
	return q.syncService.SyncUser(ctx, username)
}
