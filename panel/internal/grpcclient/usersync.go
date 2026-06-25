package grpcclient

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"KorisPanel/panel/internal/dbstore"
)

// UserSyncPayload represents the data pushed to a knode for a single user.
type UserSyncPayload struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	Enabled        bool   `json:"enabled"`
	MaxDataBytes   int64  `json:"max_data_bytes"`
	MaxConnections int    `json:"max_connections"`
	BandwidthBPS   int64  `json:"bandwidth_limit_bps"`
}

// UserSyncService handles fan-out of user credential/limit changes to knode instances.
// It determines which nodes serve the user's core types and calls SyncUsers on each.
type UserSyncService struct {
	pool  Pool
	store dbstore.Store
}

// NewUserSyncService creates a new UserSyncService.
func NewUserSyncService(pool Pool, store dbstore.Store) *UserSyncService {
	return &UserSyncService{
		pool:  pool,
		store: store,
	}
}

// SyncUser pushes the current state of a user to all nodes that serve the user's
// assigned core types. It builds the payload from the database and fans out to
// each relevant node. On failure, it retries once after 5 seconds. If the retry
// also fails, it records the failure in the sync_failures table.
func (s *UserSyncService) SyncUser(ctx context.Context, username string) error {
	// 1. Build the payload from database state
	payload, err := s.buildPayload(ctx, username)
	if err != nil {
		return fmt.Errorf("usersync: build payload for %q: %w", username, err)
	}

	// 2. Determine target nodes based on user's assigned core types
	coreTypes, err := s.getUserCoreTypes(ctx, username)
	if err != nil {
		return fmt.Errorf("usersync: get core types for %q: %w", username, err)
	}

	if len(coreTypes) == 0 {
		log.Printf("[knode] SyncUser: no core types assigned for user %q, skipping", username)
		return nil
	}

	targetNodes, err := s.getNodesForCoreTypes(ctx, coreTypes)
	if err != nil {
		return fmt.Errorf("usersync: get target nodes for %q: %w", username, err)
	}

	if len(targetNodes) == 0 {
		log.Printf("[knode] SyncUser: no online nodes serve core types %v for user %q", coreTypes, username)
		return nil
	}

	// 3. Fan-out: call SyncUsers on each target node
	for _, nodeID := range targetNodes {
		s.syncToNode(ctx, nodeID, coreTypes, payload)
	}

	return nil
}

// SyncUserToNodes pushes user state to a specific set of nodes (used during reconnection).
func (s *UserSyncService) SyncUserToNodes(ctx context.Context, username string, nodeIDs []int64) error {
	payload, err := s.buildPayload(ctx, username)
	if err != nil {
		return fmt.Errorf("usersync: build payload for %q: %w", username, err)
	}

	coreTypes, err := s.getUserCoreTypes(ctx, username)
	if err != nil {
		return fmt.Errorf("usersync: get core types for %q: %w", username, err)
	}

	for _, nodeID := range nodeIDs {
		s.syncToNode(ctx, nodeID, coreTypes, payload)
	}

	return nil
}

// FullSyncForNode performs a complete user sync for all cores on a given node.
// Called when a node transitions from offline to online.
func (s *UserSyncService) FullSyncForNode(ctx context.Context, nodeID int64) error {
	// Get all core types this node serves
	nodeCores, err := s.getNodeCoreTypes(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("usersync: get node %d core types: %w", nodeID, err)
	}

	if len(nodeCores) == 0 {
		log.Printf("[knode] FullSyncForNode: node %d has no enabled cores, skipping", nodeID)
		return nil
	}

	// Get all active users that have subscriptions covering any of these core types
	usernames, err := s.getUsersForCoreTypes(ctx, nodeCores)
	if err != nil {
		return fmt.Errorf("usersync: get users for node %d cores: %w", nodeID, err)
	}

	log.Printf("[knode] FullSyncForNode: syncing %d users to node %d (cores: %v)", len(usernames), nodeID, nodeCores)

	for _, username := range usernames {
		payload, err := s.buildPayload(ctx, username)
		if err != nil {
			log.Printf("[knode] FullSyncForNode: failed to build payload for %q: %v", username, err)
			continue
		}
		s.syncToNode(ctx, nodeID, nodeCores, payload)
	}

	return nil
}

// syncToNode sends user data to a single node with retry-once-after-5s logic.
func (s *UserSyncService) syncToNode(ctx context.Context, nodeID int64, coreTypes []string, payload UserSyncPayload) {
	err := s.callSyncUsers(ctx, nodeID, payload)
	if err == nil {
		return
	}

	log.Printf("[knode] SyncUsers failed for user %q on node %d: %v — retrying in 5s", payload.Username, nodeID, err)

	// Retry once after 5 seconds
	select {
	case <-ctx.Done():
		s.recordSyncFailure(nodeID, coreTypes, payload, fmt.Sprintf("context cancelled before retry: %v", err))
		return
	case <-time.After(5 * time.Second):
	}

	retryErr := s.callSyncUsers(ctx, nodeID, payload)
	if retryErr == nil {
		log.Printf("[knode] SyncUsers retry succeeded for user %q on node %d", payload.Username, nodeID)
		return
	}

	// Both attempts failed — record in sync_failures table
	log.Printf("[knode] SyncUsers retry also failed for user %q on node %d: %v — recording failure", payload.Username, nodeID, retryErr)
	s.recordSyncFailure(nodeID, coreTypes, payload, retryErr.Error())
}

// callSyncUsers makes the actual RPC call to a knode instance.
// This is currently a stub since generated gRPC proto clients are not yet available.
// The fan-out logic, retry mechanism, and failure recording are fully implemented.
func (s *UserSyncService) callSyncUsers(ctx context.Context, nodeID int64, payload UserSyncPayload) error {
	node, err := s.pool.Get(nodeID)
	if err != nil {
		return fmt.Errorf("node not found in pool: %w", err)
	}

	if node.Status != StatusOnline {
		return fmt.Errorf("node %q is %s, cannot sync", node.NodeName, node.Status)
	}

	// TODO: Replace with actual gRPC call when proto client is generated.
	// The call would be:
	//   client := knodepb.NewKnodeServiceClient(node.Conn)
	//   _, err = client.SyncUsers(ctx, &knodepb.SyncUsersRequest{
	//       Users: []*knodepb.UserEntry{{
	//           Username:       payload.Username,
	//           Password:       payload.Password,
	//           Enabled:        payload.Enabled,
	//           MaxDataBytes:   payload.MaxDataBytes,
	//           MaxConnections: int32(payload.MaxConnections),
	//           BandwidthBps:   payload.BandwidthBPS,
	//       }},
	//   })
	log.Printf("[knode] SyncUsers stub: would push user %q (enabled=%t) to node %q (id=%d)",
		payload.Username, payload.Enabled, node.NodeName, nodeID)
	return nil
}

// buildPayload constructs a UserSyncPayload from the database for the given username.
func (s *UserSyncService) buildPayload(ctx context.Context, username string) (UserSyncPayload, error) {
	db := s.store.DB()

	payload := UserSyncPayload{
		Username: username,
		Enabled:  true,
	}

	// Get customer status and check for expired/suspended
	var status string
	err := db.QueryRowContext(ctx,
		`SELECT status FROM customers WHERE username = ? AND deleted_at IS NULL`,
		username,
	).Scan(&status)
	if err != nil {
		if err == sql.ErrNoRows {
			// User deleted — send with enabled=false so nodes remove access
			payload.Enabled = false
			return payload, nil
		}
		return payload, fmt.Errorf("query customer status: %w", err)
	}

	// Disabled, expired, or limited users should have enabled=false
	if status == "disabled" || status == "expired" || status == "limited" {
		payload.Enabled = false
	}

	// Check subscription expiry
	if payload.Enabled {
		var expiresAt sql.NullTime
		err = db.QueryRowContext(ctx,
			`SELECT expires_at FROM subscriptions
			 WHERE username = ? AND status = 'active'
			 ORDER BY id DESC LIMIT 1`,
			username,
		).Scan(&expiresAt)
		if err != nil && err != sql.ErrNoRows {
			return payload, fmt.Errorf("query subscription expiry: %w", err)
		}
		if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
			payload.Enabled = false
		}
	}

	// Get password from radcheck (Cleartext-Password)
	_ = db.QueryRowContext(ctx,
		`SELECT value FROM radcheck
		 WHERE username = ? AND attribute IN ('Cleartext-Password', 'User-Password')
		 ORDER BY id DESC LIMIT 1`,
		username,
	).Scan(&payload.Password)

	// Get max_data_bytes from radcheck (Max-Data attribute, stored as bytes string)
	var maxDataStr string
	err = db.QueryRowContext(ctx,
		`SELECT value FROM radcheck
		 WHERE username = ? AND attribute = 'Max-Data'
		 ORDER BY id DESC LIMIT 1`,
		username,
	).Scan(&maxDataStr)
	if err == nil && maxDataStr != "" {
		fmt.Sscanf(maxDataStr, "%d", &payload.MaxDataBytes)
	}

	// Get max_connections from radcheck (Simultaneous-Use attribute)
	var maxConnStr string
	err = db.QueryRowContext(ctx,
		`SELECT value FROM radcheck
		 WHERE username = ? AND attribute = 'Simultaneous-Use'
		 ORDER BY id DESC LIMIT 1`,
		username,
	).Scan(&maxConnStr)
	if err == nil && maxConnStr != "" {
		fmt.Sscanf(maxConnStr, "%d", &payload.MaxConnections)
	}

	// Get bandwidth limit from bandwidth_rules table (download_kbps → convert to bps)
	var downloadKbps int64
	err = db.QueryRowContext(ctx,
		`SELECT download_kbps FROM bandwidth_rules
		 WHERE username = ? AND is_active = 1`,
		username,
	).Scan(&downloadKbps)
	if err == nil && downloadKbps > 0 {
		payload.BandwidthBPS = downloadKbps * 1000 // kbps → bps
	}

	return payload, nil
}

// getUserCoreTypes returns the core types (service names) assigned to a user.
// In KorisPanel, all active users are eligible for all enabled cores on their
// assigned nodes. The core types are determined by the node_services table.
// If a user has a preferred node, only that node's cores are returned;
// otherwise all active node cores are considered.
func (s *UserSyncService) getUserCoreTypes(ctx context.Context, username string) ([]string, error) {
	db := s.store.DB()

	// Check if user has a preferred node
	var preferredNodeID sql.NullInt64
	_ = db.QueryRowContext(ctx,
		`SELECT preferred_node_id FROM customers WHERE username = ? AND deleted_at IS NULL`,
		username,
	).Scan(&preferredNodeID)

	var query string
	var args []any

	if preferredNodeID.Valid && preferredNodeID.Int64 > 0 {
		// Only cores on the preferred node
		query = `SELECT DISTINCT service FROM node_services WHERE node_id = ? AND status != 'unknown'`
		args = []any{preferredNodeID.Int64}
	} else {
		// All active cores across all nodes
		query = `SELECT DISTINCT service FROM node_services WHERE status != 'unknown'`
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var coreTypes []string
	for rows.Next() {
		var ct string
		if err := rows.Scan(&ct); err != nil {
			return nil, err
		}
		coreTypes = append(coreTypes, ct)
	}
	return coreTypes, rows.Err()
}

// getNodesForCoreTypes returns node IDs of all connected nodes that serve
// at least one of the given core types.
func (s *UserSyncService) getNodesForCoreTypes(ctx context.Context, coreTypes []string) ([]int64, error) {
	if len(coreTypes) == 0 {
		return nil, nil
	}

	db := s.store.DB()

	// Build IN clause
	placeholders := ""
	args := make([]any, len(coreTypes))
	for i, ct := range coreTypes {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args[i] = ct
	}

	query := fmt.Sprintf(
		`SELECT DISTINCT node_id FROM node_services WHERE service IN (%s) AND status != 'unknown'`,
		placeholders,
	)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Filter to only nodes that are online in the pool
	var nodeIDs []int64
	for rows.Next() {
		var nodeID int64
		if err := rows.Scan(&nodeID); err != nil {
			return nil, err
		}
		// Only include nodes that are in the pool (connected)
		if s.pool.Status(nodeID) != StatusOffline {
			nodeIDs = append(nodeIDs, nodeID)
		}
	}
	return nodeIDs, rows.Err()
}

// getNodeCoreTypes returns the core types served by a specific node.
func (s *UserSyncService) getNodeCoreTypes(ctx context.Context, nodeID int64) ([]string, error) {
	db := s.store.DB()

	rows, err := db.QueryContext(ctx,
		`SELECT service FROM node_services WHERE node_id = ? AND status != 'unknown'`,
		nodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cores []string
	for rows.Next() {
		var core string
		if err := rows.Scan(&core); err != nil {
			return nil, err
		}
		cores = append(cores, core)
	}
	return cores, rows.Err()
}

// getUsersForCoreTypes returns all active usernames that should be synced
// to nodes serving the given core types.
func (s *UserSyncService) getUsersForCoreTypes(ctx context.Context, coreTypes []string) ([]string, error) {
	db := s.store.DB()

	// All non-deleted customers (including expired/disabled — we sync them with enabled=false)
	rows, err := db.QueryContext(ctx,
		`SELECT username FROM customers WHERE deleted_at IS NULL`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usernames []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		usernames = append(usernames, u)
	}
	return usernames, rows.Err()
}

// recordSyncFailure inserts a record into the sync_failures table for manual review.
func (s *UserSyncService) recordSyncFailure(nodeID int64, coreTypes []string, payload UserSyncPayload, errMsg string) {
	db := s.store.DB()

	payloadJSON, _ := json.Marshal(payload)
	coreType := "multiple"
	if len(coreTypes) == 1 {
		coreType = coreTypes[0]
	}

	_, err := db.Exec(
		`INSERT INTO sync_failures (node_id, core_type, error_msg, payload, attempts, resolved, created_at)
		 VALUES (?, ?, ?, ?, 2, FALSE, NOW())`,
		nodeID, coreType, errMsg, payloadJSON,
	)
	if err != nil {
		log.Printf("[knode] Failed to record sync failure for node %d: %v", nodeID, err)
	}
}
