package grpcclient

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	// DefaultCheckInterval is how often the status monitor checks node staleness.
	DefaultCheckInterval = 5 * time.Second

	// StaleThreshold is the duration without metrics before a node is marked stale.
	StaleThreshold = 30 * time.Second

	// OfflineThreshold is the duration without metrics before a stale node is marked offline.
	OfflineThreshold = 120 * time.Second
)

// StatusMonitor periodically evaluates node connections and transitions their
// status based on time elapsed since the last received metrics event.
//
// State machine:
//
//	offline → online:  Connection established + StreamMetrics open (handled by pool)
//	online  → stale:   No metrics for 30s
//	online  → offline: Connection lost / gRPC error (handled by pool)
//	stale   → offline: No metrics for 120s
//	stale   → online:  Metrics stream recovers (handled by UpdateLastMetrics callers)
//	offline → online:  Reconnection succeeds (handled by pool)
//
// The StatusMonitor is responsible for the time-based transitions:
//
//	online → stale (30s) and stale → offline (120s).
type StatusMonitor struct {
	pool          *connPool
	checkInterval time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewStatusMonitor creates a StatusMonitor that watches the given pool.
// The checkInterval controls how frequently node staleness is evaluated.
// Pass 0 to use the DefaultCheckInterval (5s).
func NewStatusMonitor(pool *connPool, checkInterval time.Duration) *StatusMonitor {
	if checkInterval <= 0 {
		checkInterval = DefaultCheckInterval
	}
	return &StatusMonitor{
		pool:          pool,
		checkInterval: checkInterval,
	}
}

// NewStatusMonitorFromPool creates a StatusMonitor from a Pool interface.
// It requires the underlying pool to be a *connPool (which it always is in practice).
// Pass 0 for checkInterval to use the DefaultCheckInterval (5s).
func NewStatusMonitorFromPool(pool Pool, checkInterval time.Duration) *StatusMonitor {
	cp, ok := pool.(*connPool)
	if !ok {
		log.Printf("[grpc-client] WARNING: StatusMonitor requires *connPool, got %T — status transitions will not work", pool)
		return &StatusMonitor{checkInterval: DefaultCheckInterval}
	}
	return NewStatusMonitor(cp, checkInterval)
}

// Start begins the periodic status check loop. It runs until Stop is called
// or the provided context is cancelled.
func (sm *StatusMonitor) Start(ctx context.Context) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// If already running, stop the existing loop first.
	if sm.cancel != nil {
		sm.cancel()
	}

	ctx, sm.cancel = context.WithCancel(ctx)
	go sm.loop(ctx)
	log.Printf("[grpc-client] Status monitor started (check interval: %s)", sm.checkInterval)
}

// Stop halts the periodic status check loop.
func (sm *StatusMonitor) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.cancel != nil {
		sm.cancel()
		sm.cancel = nil
	}
	log.Printf("[grpc-client] Status monitor stopped")
}

// loop runs the periodic ticker that evaluates node status transitions.
func (sm *StatusMonitor) loop(ctx context.Context) {
	ticker := time.NewTicker(sm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			sm.evaluate(now)
		}
	}
}

// evaluate checks all nodes in the pool and performs time-based status transitions.
func (sm *StatusMonitor) evaluate(now time.Time) {
	sm.pool.mu.RLock()
	// Collect nodes that need evaluation (snapshot under read lock).
	type candidate struct {
		nodeID      int64
		status      NodeStatus
		lastMetrics time.Time
	}
	candidates := make([]candidate, 0, len(sm.pool.connections))
	for _, entry := range sm.pool.connections {
		candidates = append(candidates, candidate{
			nodeID:      entry.conn.NodeID,
			status:      entry.conn.Status,
			lastMetrics: entry.conn.LastMetrics,
		})
	}
	sm.pool.mu.RUnlock()

	// Evaluate each node outside the lock (SetStatus acquires its own lock).
	for _, c := range candidates {
		sm.evaluateNode(c.nodeID, c.status, c.lastMetrics, now)
	}
}

// evaluateNode applies the time-based status transition rules for a single node.
func (sm *StatusMonitor) evaluateNode(nodeID int64, status NodeStatus, lastMetrics time.Time, now time.Time) {
	// Skip nodes that are already offline — they will transition back to online
	// via the pool's reconnection logic, not via the status monitor.
	if status == StatusOffline {
		return
	}

	// If lastMetrics is zero (never received metrics), skip evaluation.
	// The node just connected and hasn't streamed yet.
	if lastMetrics.IsZero() {
		return
	}

	elapsed := now.Sub(lastMetrics)

	switch status {
	case StatusOnline:
		if elapsed > StaleThreshold {
			log.Printf("[grpc-client] Node %d stale: no metrics for %s", nodeID, elapsed.Round(time.Second))
			sm.pool.SetStatus(nodeID, StatusStale)
		}

	case StatusStale:
		if elapsed > OfflineThreshold {
			log.Printf("[grpc-client] Node %d offline: no metrics for %s", nodeID, elapsed.Round(time.Second))
			sm.pool.SetStatus(nodeID, StatusOffline)
		}
	}
}
