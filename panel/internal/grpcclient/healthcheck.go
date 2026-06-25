package grpcclient

import (
	"context"
	"log"
	"sync"
	"time"

	"KorisPanel/panel/internal/alerts"
)

// HealthStatus represents the result of a Health RPC call to a knode instance.
type HealthStatus string

const (
	HealthOK        HealthStatus = "OK"
	HealthDegraded  HealthStatus = "DEGRADED"
	HealthUnhealthy HealthStatus = "UNHEALTHY"
)

// DefaultHealthCheckInterval is the period between supplemental health checks.
const DefaultHealthCheckInterval = 60 * time.Second

// Alerter is an interface for emitting alerts from the health checker.
type Alerter interface {
	Emit(alert alerts.Alert)
}

// HealthChecker performs periodic supplemental Health RPC calls on each
// connected node, independent of the metrics-based StatusMonitor.
// If a node reports DEGRADED or UNHEALTHY, the checker emits an alert
// and updates the node's status.
type HealthChecker struct {
	pool     *connPool
	alerter  Alerter
	interval time.Duration

	// callHealthRPCFunc allows overriding the Health RPC call for testing.
	// If nil, the default callHealthRPC stub is used.
	callHealthRPCFunc func(ctx context.Context, nodeID int64) (HealthStatus, error)

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewHealthChecker creates a HealthChecker that pings nodes via Health RPC.
// Pass 0 for interval to use the default (60s).
func NewHealthChecker(pool *connPool, alerter Alerter, interval time.Duration) *HealthChecker {
	if interval <= 0 {
		interval = DefaultHealthCheckInterval
	}
	return &HealthChecker{
		pool:     pool,
		alerter:  alerter,
		interval: interval,
	}
}

// Start begins the periodic health check loop. It runs until Stop is called
// or the provided context is cancelled.
func (hc *HealthChecker) Start(ctx context.Context) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	// If already running, stop the existing loop first.
	if hc.cancel != nil {
		hc.cancel()
	}

	ctx, hc.cancel = context.WithCancel(ctx)
	go hc.loop(ctx)
	log.Printf("[node-checker] Health checker started (interval: %s)", hc.interval)
}

// Stop halts the periodic health check loop.
func (hc *HealthChecker) Stop() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if hc.cancel != nil {
		hc.cancel()
		hc.cancel = nil
	}
	log.Printf("[node-checker] Health checker stopped")
}

// loop runs the periodic ticker that calls Health RPC on each node.
func (hc *HealthChecker) loop(ctx context.Context) {
	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.checkAll(ctx)
		}
	}
}

// checkAll iterates over all connected nodes and performs a health check.
func (hc *HealthChecker) checkAll(ctx context.Context) {
	hc.pool.mu.RLock()
	nodeIDs := make([]int64, 0, len(hc.pool.connections))
	for id, entry := range hc.pool.connections {
		// Only check nodes that are online or stale — skip offline nodes
		// since their connection is down and the pool handles reconnection.
		if entry.conn.Status == StatusOnline || entry.conn.Status == StatusStale {
			nodeIDs = append(nodeIDs, id)
		}
	}
	hc.pool.mu.RUnlock()

	for _, nodeID := range nodeIDs {
		hc.checkNode(ctx, nodeID)
	}
}

// checkNode calls the Health RPC (stub) on a single node and processes the result.
// If the Health RPC returns DEGRADED: emits AlertNodeDegraded and updates status.
// If the Health RPC returns UNHEALTHY: emits AlertNodeDegraded and marks accordingly.
// If the Health RPC fails (network error): logs but does NOT change node status,
// as the metrics stream and StatusMonitor handle connectivity-based transitions.
func (hc *HealthChecker) checkNode(ctx context.Context, nodeID int64) {
	callFn := hc.callHealthRPC
	if hc.callHealthRPCFunc != nil {
		callFn = hc.callHealthRPCFunc
	}

	status, err := callFn(ctx, nodeID)
	if err != nil {
		// Health RPC failed — log but don't change node status.
		// The metrics stream handles connectivity-based transitions.
		log.Printf("[node-checker] Health RPC failed for node %d: %v", nodeID, err)
		return
	}

	switch status {
	case HealthOK:
		// Node is healthy, nothing to do.
		return

	case HealthDegraded:
		log.Printf("[node-checker] Node %d reports DEGRADED health", nodeID)
		hc.pool.SetStatus(nodeID, StatusStale)
		if hc.alerter != nil {
			hc.alerter.Emit(alerts.Alert{
				Type:      alerts.AlertNodeDegraded,
				NodeID:    nodeID,
				Message:   "Node health check reports DEGRADED status",
				Timestamp: time.Now(),
			})
		}

	case HealthUnhealthy:
		log.Printf("[node-checker] Node %d reports UNHEALTHY", nodeID)
		hc.pool.SetStatus(nodeID, StatusOffline)
		if hc.alerter != nil {
			hc.alerter.Emit(alerts.Alert{
				Type:      alerts.AlertNodeDegraded,
				NodeID:    nodeID,
				Message:   "Node health check reports UNHEALTHY status",
				Timestamp: time.Now(),
			})
		}
	}
}

// callHealthRPC is a stub that calls the Health RPC on a knode instance.
// Once proto clients are generated, this will invoke the actual Health RPC.
// For now, it always returns HealthOK (no-op placeholder).
func (hc *HealthChecker) callHealthRPC(ctx context.Context, nodeID int64) (HealthStatus, error) {
	// TODO: Replace with actual proto client call once generated:
	//
	//   conn, err := hc.pool.Get(nodeID)
	//   if err != nil {
	//       return "", err
	//   }
	//   client := knodepb.NewKnodeServiceClient(conn.Conn)
	//   resp, err := client.Health(ctx, &knodepb.HealthRequest{})
	//   if err != nil {
	//       return "", err
	//   }
	//   return HealthStatus(resp.Status), nil
	//
	_ = ctx
	_ = nodeID
	return HealthOK, nil
}
