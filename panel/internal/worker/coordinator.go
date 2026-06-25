package worker

import (
	"context"
	"fmt"
	"os"

	"KorisPanel/panel/internal/dbstore"
)

// Advisory lock IDs for background tasks.
// Each periodic task gets a unique lock so multiple workers don't
// execute the same task simultaneously.
const (
	LockExpiry      int64 = 1001
	LockBilling     int64 = 1002
	LockNodeMonitor int64 = 1003
	LockTraffic     int64 = 1004
	LockCertRotate  int64 = 1005
)

// LeaderElectOffset is added to a node ID to produce the advisory lock ID
// for node ownership leader election.
const LeaderElectOffset int64 = 10000

// Coordinator manages multi-worker task distribution using database advisory
// locks. Each panel worker process has a unique workerID (hostname + PID).
// Background tasks (expiry, billing, node monitoring, traffic, cert rotation)
// use TryRun to ensure only one worker executes each task at a time.
// Node ownership uses LeaderElect to assign exactly one worker per node's
// metrics stream.
type Coordinator struct {
	store    dbstore.Store
	workerID string
}

// NewCoordinator creates a Coordinator with the given store and a unique
// worker ID derived from the current hostname and PID.
func NewCoordinator(store dbstore.Store) *Coordinator {
	return &Coordinator{
		store:    store,
		workerID: generateWorkerID(),
	}
}

// NewCoordinatorWithID creates a Coordinator with an explicit worker ID.
// Useful for testing or when the ID is provided via configuration.
func NewCoordinatorWithID(store dbstore.Store, id string) *Coordinator {
	return &Coordinator{
		store:    store,
		workerID: id,
	}
}

// WorkerID returns the unique identifier for this worker process.
func (c *Coordinator) WorkerID() string {
	return c.workerID
}

// TryRun attempts to acquire the advisory lock for lockID, runs fn if acquired.
// Returns (true, nil) if the task was executed successfully by this worker.
// Returns (true, err) if the lock was acquired but fn returned an error.
// Returns (false, nil) if another worker holds the lock.
// Returns (false, err) if lock acquisition itself failed.
func (c *Coordinator) TryRun(ctx context.Context, lockID int64, fn func(ctx context.Context) error) (bool, error) {
	acquired, err := c.store.AcquireLock(ctx, lockID)
	if err != nil {
		return false, err
	}
	if !acquired {
		return false, nil
	}
	defer c.store.ReleaseLock(ctx, lockID)
	return true, fn(ctx)
}

// LeaderElect attempts to claim ownership of a node's metrics stream.
// Uses an advisory lock at offset LeaderElectOffset + nodeID so each node
// is managed by exactly one worker. Returns true if this worker acquired
// ownership, false if another worker already owns it.
func (c *Coordinator) LeaderElect(ctx context.Context, nodeID int64) (bool, error) {
	return c.store.AcquireLock(ctx, LeaderElectOffset+nodeID)
}

// ReleaseNodeLock releases the advisory lock for a node, allowing another
// worker to claim ownership. Should be called during graceful shutdown.
func (c *Coordinator) ReleaseNodeLock(ctx context.Context, nodeID int64) error {
	return c.store.ReleaseLock(ctx, LeaderElectOffset+nodeID)
}

// generateWorkerID produces a unique worker identifier from hostname and PID.
// Format: "hostname-PID" (e.g. "panel-server-12345").
func generateWorkerID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}
