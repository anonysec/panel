package grpcclient

import (
	"context"
	"log"
	"sync"
	"time"

	"KorisPanel/panel/internal/dbstore"
)

const (
	// DefaultTrafficInterval is the default period between GetTraffic calls.
	DefaultTrafficInterval = 60 * time.Second
)

// UserTraffic holds cumulative byte counters for a single user on a node.
type UserTraffic struct {
	RxBytes int64
	TxBytes int64
}

// TrafficReport represents the per-user traffic data returned by a knode's GetTraffic RPC.
type TrafficReport struct {
	Users map[string]*UserTraffic
}

// TrafficCollector periodically calls GetTraffic on all connected nodes,
// calculates per-user bandwidth deltas, and writes them to user_traffic_log
// via dbstore.InsertTrafficLog. It handles counter resets (negative deltas)
// by using the absolute value of the current reading as the delta.
type TrafficCollector struct {
	pool     Pool
	store    dbstore.Store
	interval time.Duration
	quota    *QuotaEnforcer

	mu       sync.Mutex
	lastSeen map[int64]map[string]*UserTraffic // per-node, per-user last known values
	cancel   context.CancelFunc
}

// NewTrafficCollector creates a TrafficCollector with the given pool, store, and interval.
// Pass 0 for interval to use DefaultTrafficInterval (60s).
// The quotaEnforcer may be nil if quota enforcement is not needed.
func NewTrafficCollector(pool Pool, store dbstore.Store, interval time.Duration, quotaEnforcer *QuotaEnforcer) *TrafficCollector {
	if interval <= 0 {
		interval = DefaultTrafficInterval
	}
	return &TrafficCollector{
		pool:     pool,
		store:    store,
		interval: interval,
		quota:    quotaEnforcer,
		lastSeen: make(map[int64]map[string]*UserTraffic),
	}
}

// Start begins the periodic traffic collection loop. It runs until Stop is called
// or the provided context is cancelled.
func (tc *TrafficCollector) Start(ctx context.Context) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.cancel != nil {
		tc.cancel()
	}

	ctx, tc.cancel = context.WithCancel(ctx)
	go tc.loop(ctx)
	log.Printf("[grpc-client] Traffic collector started (interval: %s)", tc.interval)
}

// Stop halts the periodic traffic collection loop.
func (tc *TrafficCollector) Stop() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.cancel != nil {
		tc.cancel()
		tc.cancel = nil
	}
	log.Printf("[grpc-client] Traffic collector stopped")
}

// loop runs the periodic ticker that collects traffic from all connected nodes.
func (tc *TrafficCollector) loop(ctx context.Context) {
	ticker := time.NewTicker(tc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tc.collectAll(ctx)
		}
	}
}

// collectAll iterates over all connected nodes and calls GetTraffic on each.
func (tc *TrafficCollector) collectAll(ctx context.Context) {
	nodes := tc.pool.All()
	for _, node := range nodes {
		if node.Status == StatusOffline {
			continue
		}
		tc.collectNode(ctx, node.NodeID)
	}
}

// collectNode calls GetTraffic on a single node and processes the report.
// Requirement 6.4: IF the GetTraffic RPC fails, THEN the panel SHALL log the error
// and retry on the next interval without marking the node as unhealthy.
func (tc *TrafficCollector) collectNode(ctx context.Context, nodeID int64) {
	report, err := tc.callGetTraffic(ctx, nodeID)
	if err != nil {
		// Req 6.4: Log and retry on next interval — don't mark node as unhealthy.
		// The node status remains unchanged; the next tick will attempt collection again.
		log.Printf("[grpc-client] GetTraffic failed for node %d: %v", nodeID, err)
		return
	}

	if report == nil || len(report.Users) == 0 {
		return
	}

	tc.processReport(ctx, nodeID, report)
}

// processReport calculates deltas between the current report and last-seen values,
// handles counter resets, and writes traffic log entries to the database.
func (tc *TrafficCollector) processReport(ctx context.Context, nodeID int64, report *TrafficReport) {
	tc.mu.Lock()
	nodeLast, exists := tc.lastSeen[nodeID]
	if !exists {
		nodeLast = make(map[string]*UserTraffic)
		tc.lastSeen[nodeID] = nodeLast
	}
	tc.mu.Unlock()

	now := time.Now()

	for username, current := range report.Users {
		if current == nil {
			continue
		}

		tc.mu.Lock()
		last, hasLast := nodeLast[username]
		tc.mu.Unlock()

		var deltaRx, deltaTx int64

		if !hasLast {
			// First report for this user on this node — no delta, just record baseline.
			// We use the absolute value as the initial delta to capture traffic
			// that accumulated before the collector started.
			deltaRx = current.RxBytes
			deltaTx = current.TxBytes
		} else {
			deltaRx = current.RxBytes - last.RxBytes
			deltaTx = current.TxBytes - last.TxBytes

			// Handle counter resets: if delta is negative, the counter was reset on the node.
			// Use absolute value as delta per design spec.
			if deltaRx < 0 {
				deltaRx = current.RxBytes
			}
			if deltaTx < 0 {
				deltaTx = current.TxBytes
			}
		}

		// Update last-seen values
		tc.mu.Lock()
		nodeLast[username] = &UserTraffic{
			RxBytes: current.RxBytes,
			TxBytes: current.TxBytes,
		}
		tc.mu.Unlock()

		// Skip zero deltas — no traffic to record
		if deltaRx == 0 && deltaTx == 0 {
			continue
		}

		// Resolve username to user_id for the traffic log entry.
		userID := tc.resolveUserID(ctx, username)
		if userID == 0 {
			log.Printf("[grpc-client] Traffic: unknown user %q on node %d, skipping", username, nodeID)
			continue
		}

		// Write to user_traffic_log via dbstore
		entry := &dbstore.TrafficLogEntry{
			Time:    now,
			UserID:  userID,
			NodeID:  nodeID,
			RxBytes: deltaRx,
			TxBytes: deltaTx,
		}

		if err := tc.store.InsertTrafficLog(ctx, entry); err != nil {
			log.Printf("[grpc-client] Failed to insert traffic log for user %q (id=%d) on node %d: %v",
				username, userID, nodeID, err)
			continue
		}

		// After a successful traffic log write, check if user has exceeded their quota.
		if tc.quota != nil {
			if _, err := tc.quota.CheckQuota(ctx, userID, username); err != nil {
				log.Printf("[grpc-client] Quota check failed for user %q (id=%d): %v", username, userID, err)
			}
		}
	}
}

// callGetTraffic makes the GetTraffic RPC call to the specified node.
// This is currently a stub since generated gRPC proto clients are not yet available.
// The delta accumulation logic and periodic collection are fully implemented.
func (tc *TrafficCollector) callGetTraffic(ctx context.Context, nodeID int64) (*TrafficReport, error) {
	node, err := tc.pool.Get(nodeID)
	if err != nil {
		return nil, err
	}

	if node.Status == StatusOffline {
		return nil, nil
	}

	// TODO: Replace with actual gRPC call when proto client is generated.
	// The call would be:
	//   client := knodepb.NewKnodeServiceClient(node.Conn)
	//   resp, err := client.GetTraffic(ctx, &knodepb.GetTrafficRequest{})
	//   if err != nil { return nil, err }
	//   report := &TrafficReport{Users: make(map[string]*UserTraffic)}
	//   for _, u := range resp.Users {
	//       report.Users[u.Username] = &UserTraffic{RxBytes: u.RxBytes, TxBytes: u.TxBytes}
	//   }
	//   return report, nil

	log.Printf("[grpc-client] GetTraffic stub: called for node %q (id=%d)", node.NodeName, nodeID)
	return nil, nil
}

// resolveUserID looks up the customer ID for a given username.
func (tc *TrafficCollector) resolveUserID(ctx context.Context, username string) int64 {
	db := tc.store.DB()

	var userID int64
	err := db.QueryRowContext(ctx,
		`SELECT id FROM customers WHERE username = ? AND deleted_at IS NULL`,
		username,
	).Scan(&userID)
	if err != nil {
		return 0
	}
	return userID
}

// ProcessReportForNode is an exported helper that allows external callers (e.g., tests)
// to inject a traffic report for a specific node. This processes deltas and writes to DB.
func (tc *TrafficCollector) ProcessReportForNode(ctx context.Context, nodeID int64, report *TrafficReport) {
	if report == nil || len(report.Users) == 0 {
		return
	}
	tc.processReport(ctx, nodeID, report)
}

// ResetLastSeen clears the last-seen state for a specific node.
// Used when a node reconnects after being offline (counters may have been reset).
func (tc *TrafficCollector) ResetLastSeen(nodeID int64) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	delete(tc.lastSeen, nodeID)
}
