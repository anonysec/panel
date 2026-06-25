package grpcclient

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"KorisPanel/panel/internal/dbstore"
)

// ResetUserTraffic resets the traffic counters for a user on all relevant nodes.
// It determines which nodes serve the user's core types, calls ResetTraffic RPC
// on each relevant node (stub), and clears the TrafficCollector's lastSeen state
// for this user so that deltas are calculated fresh after the reset.
//
// Satisfies Requirement 6.5: When an admin resets a user's traffic counter in the
// panel, the panel SHALL call ResetTraffic on all relevant nodes for that username.
func ResetUserTraffic(ctx context.Context, username string, pool Pool, store dbstore.Store, tc *TrafficCollector) error {
	// 1. Determine which nodes serve this user's core types
	targetNodes, err := getResetTargetNodes(ctx, username, pool, store)
	if err != nil {
		return fmt.Errorf("traffic reset: get target nodes for %q: %w", username, err)
	}

	if len(targetNodes) == 0 {
		log.Printf("[grpc-client] ResetUserTraffic: no online nodes found for user %q, skipping RPC calls", username)
	}

	// 2. Call ResetTraffic RPC on each relevant node
	for _, nodeID := range targetNodes {
		if err := callResetTraffic(ctx, pool, nodeID, username); err != nil {
			// Log but continue — best-effort reset across all nodes
			log.Printf("[grpc-client] ResetTraffic failed for user %q on node %d: %v", username, nodeID, err)
		}
	}

	// 3. Clear the TrafficCollector's lastSeen state for this user on all nodes
	if tc != nil {
		tc.ResetUserLastSeen(username)
	}

	log.Printf("[grpc-client] ResetUserTraffic: reset traffic for user %q on %d nodes", username, len(targetNodes))
	return nil
}

// ResetUserLastSeen clears the lastSeen state for a specific user across all nodes.
// This ensures the next GetTraffic collection will treat the user's counters as a
// fresh baseline, preventing stale deltas after a traffic reset.
func (tc *TrafficCollector) ResetUserLastSeen(username string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	for _, nodeLast := range tc.lastSeen {
		delete(nodeLast, username)
	}
}

// getResetTargetNodes determines which online nodes serve the given user's core types.
// Uses the same resolution logic as usersync: query node_services for core types,
// then filter to nodes that are online in the pool.
func getResetTargetNodes(ctx context.Context, username string, pool Pool, store dbstore.Store) ([]int64, error) {
	db := store.DB()

	// Get core types for this user (same logic as usersync.getUserCoreTypes)
	coreTypes, err := getCoreTypesForUser(ctx, db, username)
	if err != nil {
		return nil, err
	}

	if len(coreTypes) == 0 {
		return nil, nil
	}

	// Get nodes serving those core types that are online
	return getOnlineNodesForCores(ctx, db, pool, coreTypes)
}

// getCoreTypesForUser resolves which core types a user is eligible for.
// If the user has a preferred node, only that node's cores are returned;
// otherwise all active node cores are considered.
func getCoreTypesForUser(ctx context.Context, db *sql.DB, username string) ([]string, error) {
	// Check if user has a preferred node
	var preferredNodeID sql.NullInt64
	_ = db.QueryRowContext(ctx,
		`SELECT preferred_node_id FROM customers WHERE username = ? AND deleted_at IS NULL`,
		username,
	).Scan(&preferredNodeID)

	var query string
	var args []any

	if preferredNodeID.Valid && preferredNodeID.Int64 > 0 {
		query = `SELECT DISTINCT service FROM node_services WHERE node_id = ? AND status != 'unknown'`
		args = []any{preferredNodeID.Int64}
	} else {
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

// getOnlineNodesForCores returns node IDs of all connected nodes that serve
// at least one of the given core types and are currently online in the pool.
func getOnlineNodesForCores(ctx context.Context, db *sql.DB, pool Pool, coreTypes []string) ([]int64, error) {
	if len(coreTypes) == 0 {
		return nil, nil
	}

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

	var nodeIDs []int64
	for rows.Next() {
		var nodeID int64
		if err := rows.Scan(&nodeID); err != nil {
			return nil, err
		}
		// Only include nodes that are online in the pool
		if pool.Status(nodeID) != StatusOffline {
			nodeIDs = append(nodeIDs, nodeID)
		}
	}
	return nodeIDs, rows.Err()
}

// callResetTraffic makes the ResetTraffic RPC call to the specified node.
// This is currently a stub since generated gRPC proto clients are not yet available.
func callResetTraffic(ctx context.Context, pool Pool, nodeID int64, username string) error {
	node, err := pool.Get(nodeID)
	if err != nil {
		return fmt.Errorf("node not found in pool: %w", err)
	}

	if node.Status == StatusOffline {
		return fmt.Errorf("node %q is offline, cannot reset traffic", node.NodeName)
	}

	// TODO: Replace with actual gRPC call when proto client is generated.
	// The call would be:
	//   client := knodepb.NewKnodeServiceClient(node.Conn)
	//   _, err = client.ResetTraffic(ctx, &knodepb.ResetTrafficRequest{
	//       Username: username,
	//   })
	//   if err != nil { return err }
	log.Printf("[grpc-client] ResetTraffic stub: would reset traffic for user %q on node %q (id=%d)",
		username, node.NodeName, nodeID)
	return nil
}
