package grpcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"KorisPanel/panel/internal/dbstore"
	"KorisPanel/panel/internal/knodepb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Multi-panel compatibility (Requirements 14.1, 14.2, 14.3):
// The panel uses the knode API in a stateless manner, treating knode as the source of truth
// for core states and sessions. It uses status.Code(err) to detect concurrent modification:
//   - codes.AlreadyExists on EnableCore → core was enabled by another panel
//   - codes.NotFound on DisableCore → core was already disabled by another panel
//   - codes.FailedPrecondition → resource is in a conflicting state due to another panel
//
// In all cases, handleConcurrentModification refreshes local state from knode
// via AllCoreStatuses, rather than returning an error to the user.

// RefreshNodeState fetches the live core state from a knode via AllCoreStatuses
// and synchronizes it to the local database. This is the primary mechanism for
// stateless knode API usage (Requirements 14.1, 14.3): the panel always defers
// to knode as the source of truth rather than relying on cached assumptions.
//
// This function should be called:
//   - After detecting concurrent modifications (AlreadyExists, NotFound, FailedPrecondition)
//   - During initial connection setup for state reconciliation
//   - When any operation requires up-to-date core state from knode
//
// It returns nil if the refresh succeeds. If the node is unreachable or the
// AllCoreStatuses RPC fails, the error is logged and returned.
func RefreshNodeState(ctx context.Context, pool Pool, store dbstore.Store, nodeID int64) error {
	cm := &CoreManager{pool: pool, store: store}
	_, err := cm.AllCoreStatuses(ctx, nodeID)
	if err != nil {
		log.Printf("[knode] RefreshNodeState: failed to refresh state for node %d: %v", nodeID, err)
		return err
	}
	return nil
}

// isConcurrentModification checks if a gRPC error indicates a concurrent modification
// by another panel instance. The relevant codes are:
//   - AlreadyExists: resource was already created/enabled (e.g., core already enabled)
//   - NotFound: resource was already removed/disabled (e.g., core already disabled)
//   - FailedPrecondition: resource is in a state that conflicts with the requested operation
//     (e.g., trying to disable a core that is mid-transition by another panel)
//
// Returns true if the error represents a concurrent modification that should be
// handled by refreshing local state from knode, rather than surfaced to the user.
func isConcurrentModification(err error) bool {
	if err == nil {
		return false
	}
	code := status.Code(err)
	return code == codes.AlreadyExists || code == codes.NotFound || code == codes.FailedPrecondition
}

// CoreManager handles core (VPN protocol) management operations on knode instances.
// It wraps gRPC calls to EnableCore, DisableCore, and AllCoreStatuses with
// database updates to keep the local node_services table synchronized.
type CoreManager struct {
	pool  Pool
	store dbstore.Store
}

// NewCoreManager creates a CoreManager with the given pool and database store.
func NewCoreManager(pool Pool, store dbstore.Store) *CoreManager {
	return &CoreManager{
		pool:  pool,
		store: store,
	}
}

// EnableCore calls the EnableCore RPC on the target knode, then updates the
// node_services table to reflect the running state on success.
// Parameters:
//   - nodeID: the target node
//   - coreType: VPN protocol type (e.g., "openvpn", "wireguard", "l2tp", "ikev2", "ssh", "mtproto")
//   - listenPort: the port the core should listen on
//   - extraConfig: protocol-specific configuration (JSON-encoded, may be nil)
func (cm *CoreManager) EnableCore(ctx context.Context, nodeID int64, coreType string, listenPort int, extraConfig json.RawMessage) error {
	err := cm.callEnableCore(ctx, nodeID, coreType, listenPort, extraConfig)
	if err != nil {
		log.Printf("[knode] EnableCore failed for core %q on node %d: %v", coreType, nodeID, err)
		return err
	}

	// Update node_services to reflect the enabled/running state
	if err := cm.upsertCoreStatus(ctx, nodeID, coreType, "running"); err != nil {
		log.Printf("[knode] EnableCore: failed to update node_services for core %q on node %d: %v", coreType, nodeID, err)
		// Don't return error — the RPC succeeded, the DB update is best-effort
	}

	return nil
}

// DisableCore calls the DisableCore RPC on the target knode, then updates the
// node_services table to reflect the stopped state on success.
func (cm *CoreManager) DisableCore(ctx context.Context, nodeID int64, coreType string) error {
	err := cm.callDisableCore(ctx, nodeID, coreType)
	if err != nil {
		log.Printf("[knode] DisableCore failed for core %q on node %d: %v", coreType, nodeID, err)
		return err
	}

	// Update node_services to reflect the stopped state
	if err := cm.upsertCoreStatus(ctx, nodeID, coreType, "stopped"); err != nil {
		log.Printf("[knode] DisableCore: failed to update node_services for core %q on node %d: %v", coreType, nodeID, err)
	}

	return nil
}

// AllCoreStatuses calls the AllCoreStatuses RPC on the target knode and returns
// the current state of all cores. It also synchronizes the results to the local
// node_services table. This is called during initial connection setup (Requirement 4.5).
func (cm *CoreManager) AllCoreStatuses(ctx context.Context, nodeID int64) ([]CoreStatus, error) {
	statuses, err := cm.callAllCoreStatuses(ctx, nodeID)
	if err != nil {
		log.Printf("[knode] AllCoreStatuses failed for node %d: %v", nodeID, err)
		return nil, err
	}

	// Sync all reported statuses to node_services
	for _, cs := range statuses {
		if err := cm.upsertCoreStatus(ctx, nodeID, cs.Type, cs.State); err != nil {
			log.Printf("[knode] AllCoreStatuses: failed to sync core %q status for node %d: %v", cs.Type, nodeID, err)
		}
	}

	return statuses, nil
}

// callEnableCore makes the actual EnableCore RPC call to a knode instance via
// the generated knodepb proto client.
//
// Multi-panel compatibility (Requirement 14.2): If the RPC returns codes.AlreadyExists
// (core already enabled by another panel instance), we treat this as success and refresh
// local state from knode via AllCoreStatuses rather than erroring to the user.
func (cm *CoreManager) callEnableCore(ctx context.Context, nodeID int64, coreType string, listenPort int, extraConfig json.RawMessage) error {
	node, err := cm.pool.Get(nodeID)
	if err != nil {
		return fmt.Errorf("node not found in pool: %w", err)
	}

	if node.Status != StatusOnline {
		return fmt.Errorf("node %q is %s, cannot enable core", node.NodeName, node.Status)
	}

	client := knodepb.NewKnodeServiceClient(node.Conn)

	req := &knodepb.EnableCoreRequest{
		Type:        coreType,
		ListenPort:  int32(listenPort),
		ExtraConfig: extraConfig,
	}

	_, rpcErr := client.EnableCore(ctx, req)
	if rpcErr != nil {
		if isConcurrentModification(rpcErr) {
			// Core already enabled (likely by another panel instance).
			// Refresh local state from knode — this is not an error.
			log.Printf("[knode] EnableCore: core %q already enabled on node %d (concurrent modification), refreshing state", coreType, nodeID)
			cm.handleConcurrentModification(ctx, nodeID)
			return nil
		}
		return rpcErr
	}

	log.Printf("[knode] EnableCore: enabled core %q on port %d for node %q (id=%d)",
		coreType, listenPort, node.NodeName, nodeID)
	return nil
}

// callDisableCore makes the actual DisableCore RPC call to a knode instance via
// the generated knodepb proto client.
//
// Multi-panel compatibility (Requirement 14.2): If the RPC returns codes.NotFound
// (core already disabled by another panel instance), we treat this as success and refresh
// local state from knode via AllCoreStatuses rather than erroring to the user.
func (cm *CoreManager) callDisableCore(ctx context.Context, nodeID int64, coreType string) error {
	node, err := cm.pool.Get(nodeID)
	if err != nil {
		return fmt.Errorf("node not found in pool: %w", err)
	}

	if node.Status != StatusOnline {
		return fmt.Errorf("node %q is %s, cannot disable core", node.NodeName, node.Status)
	}

	client := knodepb.NewKnodeServiceClient(node.Conn)

	req := &knodepb.DisableCoreRequest{
		Type: coreType,
	}

	_, rpcErr := client.DisableCore(ctx, req)
	if rpcErr != nil {
		if isConcurrentModification(rpcErr) {
			// Core already disabled (likely by another panel instance).
			// Refresh local state from knode — this is not an error.
			log.Printf("[knode] DisableCore: core %q already disabled on node %d (concurrent modification), refreshing state", coreType, nodeID)
			cm.handleConcurrentModification(ctx, nodeID)
			return nil
		}
		return rpcErr
	}

	log.Printf("[knode] DisableCore: disabled core %q for node %q (id=%d)",
		coreType, node.NodeName, nodeID)
	return nil
}

// callAllCoreStatuses makes the AllCoreStatuses RPC call to a knode instance via
// the generated knodepb proto client.
// This always fetches live state from knode, treating knode as the source of truth
// (Requirement 14.1, 14.3). It never returns cached data.
func (cm *CoreManager) callAllCoreStatuses(ctx context.Context, nodeID int64) ([]CoreStatus, error) {
	node, err := cm.pool.Get(nodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found in pool: %w", err)
	}

	if node.Status != StatusOnline {
		return nil, fmt.Errorf("node %q is %s, cannot query core statuses", node.NodeName, node.Status)
	}

	client := knodepb.NewKnodeServiceClient(node.Conn)

	resp, err := client.AllCoreStatuses(ctx, &knodepb.AllCoreStatusesRequest{})
	if err != nil {
		return nil, err
	}

	var statuses []CoreStatus
	for _, cs := range resp.GetCores() {
		statuses = append(statuses, CoreStatus{
			Type:           cs.GetType(),
			State:          coreStateToString(cs.GetState()),
			ActiveSessions: int(cs.GetActiveSessions()),
			PID:            int(cs.GetPid()),
		})
	}

	log.Printf("[knode] AllCoreStatuses: fetched %d core statuses from node %q (id=%d)",
		len(statuses), node.NodeName, nodeID)
	return statuses, nil
}

// handleConcurrentModification reconciles local core state with knode after detecting
// a concurrent modification (e.g., another panel instance already enabled/disabled a core).
//
// This implements the stateless API usage pattern (Requirements 14.1, 14.2, 14.3):
// instead of assuming local state is correct, we always defer to knode as the source
// of truth by calling AllCoreStatuses and synchronizing the results to node_services.
//
// Called when isConcurrentModification(err) returns true for a gRPC error, indicating
// that another panel instance has already made the change we attempted. The relevant
// gRPC codes that trigger this are: AlreadyExists, NotFound, and FailedPrecondition.
func (cm *CoreManager) handleConcurrentModification(ctx context.Context, nodeID int64) {
	statuses, err := cm.callAllCoreStatuses(ctx, nodeID)
	if err != nil {
		log.Printf("[knode] handleConcurrentModification: failed to refresh core statuses for node %d: %v", nodeID, err)
		return
	}

	for _, cs := range statuses {
		if err := cm.upsertCoreStatus(ctx, nodeID, cs.Type, cs.State); err != nil {
			log.Printf("[knode] handleConcurrentModification: failed to sync core %q for node %d: %v", cs.Type, nodeID, err)
		}
	}

	log.Printf("[knode] handleConcurrentModification: refreshed %d core statuses for node %d", len(statuses), nodeID)
}

// upsertCoreStatus inserts or updates a core's status in the node_services table.
func (cm *CoreManager) upsertCoreStatus(ctx context.Context, nodeID int64, coreType, status string) error {
	db := cm.store.DB()

	_, err := db.ExecContext(ctx, `
		INSERT INTO node_services (node_id, service, status, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (node_id, service) DO UPDATE SET status = EXCLUDED.status, updated_at = NOW()`,
		nodeID, coreType, status,
	)
	return err
}

// SyncCoreStatuses is a convenience method that calls AllCoreStatuses and also
// removes any stale entries from node_services that are no longer reported by the node.
// This provides a full reconciliation during initial connection setup.
func (cm *CoreManager) SyncCoreStatuses(ctx context.Context, nodeID int64) error {
	statuses, err := cm.AllCoreStatuses(ctx, nodeID)
	if err != nil {
		return err
	}

	if len(statuses) == 0 {
		return nil
	}

	// Mark any cores in node_services that weren't reported as "unknown"
	db := cm.store.DB()
	reportedTypes := make(map[string]bool)
	for _, cs := range statuses {
		reportedTypes[cs.Type] = true
	}

	rows, err := db.QueryContext(ctx,
		`SELECT service FROM node_services WHERE node_id = $1`, nodeID)
	if err != nil {
		return fmt.Errorf("query existing services: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var svc string
		if err := rows.Scan(&svc); err != nil {
			continue
		}
		if !reportedTypes[svc] {
			_, _ = db.ExecContext(ctx,
				`UPDATE node_services SET status = 'unknown', updated_at = NOW() WHERE node_id = $1 AND service = $2`,
				nodeID, svc)
		}
	}

	return rows.Err()
}
