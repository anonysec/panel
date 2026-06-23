//go:build !lite

package cluster

import (
	"context"
	"database/sql"
	"log"
	"os"
	"sync"
	"time"
)

// LockName is the MariaDB advisory lock name used for leader election.
const LockName = "korispanel_leader"

// Role represents the cluster role of a node.
type Role string

const (
	RoleLeader   Role = "leader"
	RoleFollower Role = "follower"
)

// ClusterNode represents a panel instance in the cluster.
type ClusterNode struct {
	ID            string
	Role          Role
	LastHeartbeat sql.NullTime
	StartedAt     time.Time
	Metadata      sql.NullString
}

// ClusterManager manages distributed leader election via MariaDB advisory
// locks and node registration for health monitoring.
type ClusterManager struct {
	db     *sql.DB
	nodeID string

	mu       sync.RWMutex
	isLeader bool

	// tableExists caches whether the cluster_nodes table is present.
	// If false, we assume single-node mode (always leader).
	tableExists bool
}

// New creates a ClusterManager for the given database and node ID.
// If nodeID is empty, the hostname is used.
func New(db *sql.DB, nodeID string) *ClusterManager {
	if nodeID == "" {
		h, err := os.Hostname()
		if err != nil {
			nodeID = "unknown"
		} else {
			nodeID = h
		}
	}
	cm := &ClusterManager{
		db:     db,
		nodeID: nodeID,
	}
	cm.tableExists = cm.checkTableExists()
	return cm
}

// NodeID returns the identifier of this cluster node.
func (cm *ClusterManager) NodeID() string {
	return cm.nodeID
}

// IsLeader returns whether this node currently holds the leader lock.
// Thread-safe.
func (cm *ClusterManager) IsLeader() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.isLeader
}

// TryBecomeLeader attempts to acquire the MariaDB advisory lock.
// Returns true if this node successfully became the leader.
// The lock is non-blocking (timeout=0).
func (cm *ClusterManager) TryBecomeLeader(ctx context.Context) (bool, error) {
	var result int
	err := cm.db.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", LockName).Scan(&result)
	if err != nil {
		return false, err
	}

	acquired := result == 1
	cm.mu.Lock()
	cm.isLeader = acquired
	cm.mu.Unlock()

	// Update role in cluster_nodes table if it exists.
	if cm.tableExists {
		role := RoleFollower
		if acquired {
			role = RoleLeader
		}
		cm.updateRole(ctx, role)
	}

	return acquired, nil
}

// ReleaseLeadership releases the advisory lock. After this call, IsLeader
// returns false.
func (cm *ClusterManager) ReleaseLeadership(ctx context.Context) error {
	_, err := cm.db.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", LockName)

	cm.mu.Lock()
	cm.isLeader = false
	cm.mu.Unlock()

	if cm.tableExists {
		cm.updateRole(ctx, RoleFollower)
	}

	return err
}

// Heartbeat updates the heartbeat timestamp for this node in the
// cluster_nodes table. If the table does not exist, this is a no-op.
func (cm *ClusterManager) Heartbeat(ctx context.Context) error {
	if !cm.tableExists {
		return nil
	}

	role := RoleFollower
	if cm.IsLeader() {
		role = RoleLeader
	}

	_, err := cm.db.ExecContext(ctx, `
		INSERT INTO cluster_nodes (id, role, last_heartbeat, started_at)
		VALUES (?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE role=VALUES(role), last_heartbeat=NOW()
	`, cm.nodeID, string(role))
	if err != nil {
		log.Printf("[cluster] heartbeat failed: %v", err)
	}
	return err
}

// RunLeaderElection starts a background goroutine that periodically attempts
// to acquire the leader lock. It blocks until ctx is cancelled.
// On shutdown, it releases leadership if held.
func (cm *ClusterManager) RunLeaderElection(ctx context.Context, interval time.Duration) {
	// Register this node immediately.
	cm.Heartbeat(ctx)

	// Attempt initial leadership.
	acquired, err := cm.TryBecomeLeader(ctx)
	if err != nil {
		log.Printf("[cluster] initial leader election failed: %v", err)
	} else if acquired {
		log.Printf("[cluster] node %s became leader", cm.nodeID)
	} else {
		log.Printf("[cluster] node %s is follower", cm.nodeID)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Release leadership on shutdown.
			if cm.IsLeader() {
				shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := cm.ReleaseLeadership(shutCtx); err != nil {
					log.Printf("[cluster] failed to release leadership: %v", err)
				}
				cancel()
			}
			return
		case <-ticker.C:
			// Send heartbeat.
			cm.Heartbeat(ctx)

			// If not leader, try to acquire.
			if !cm.IsLeader() {
				acquired, err := cm.TryBecomeLeader(ctx)
				if err != nil {
					log.Printf("[cluster] leader election attempt failed: %v", err)
				} else if acquired {
					log.Printf("[cluster] node %s promoted to leader", cm.nodeID)
				}
			}
		}
	}
}

// checkTableExists checks whether the cluster_nodes table exists in the DB.
// If the query fails (table doesn't exist), we assume single-node mode.
func (cm *ClusterManager) checkTableExists() bool {
	var count int
	err := cm.db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'cluster_nodes'
	`).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// updateRole updates the role for this node in the cluster_nodes table.
func (cm *ClusterManager) updateRole(ctx context.Context, role Role) {
	_, err := cm.db.ExecContext(ctx, `
		INSERT INTO cluster_nodes (id, role, last_heartbeat, started_at)
		VALUES (?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE role=VALUES(role), last_heartbeat=NOW()
	`, cm.nodeID, string(role))
	if err != nil {
		log.Printf("[cluster] failed to update role: %v", err)
	}
}
