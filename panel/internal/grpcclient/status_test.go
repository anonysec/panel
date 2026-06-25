package grpcclient

import (
	"context"
	"sync"
	"testing"
	"time"
)

// testPool creates a minimal connPool for status monitor testing.
func testPool(nodes ...*NodeConnection) *connPool {
	p := &connPool{
		connections: make(map[int64]*nodeEntry),
	}
	for _, n := range nodes {
		p.connections[n.NodeID] = &nodeEntry{
			conn: n,
		}
	}
	return p
}

func TestStatusMonitor_OnlineToStale(t *testing.T) {
	// Node has been online but hasn't received metrics for > 30s.
	node := &NodeConnection{
		NodeID:      1,
		NodeName:    "test-node",
		Status:      StatusOnline,
		LastMetrics: time.Now().Add(-35 * time.Second), // 35s ago
	}
	pool := testPool(node)

	var mu sync.Mutex
	var transitions []struct{ old, new NodeStatus }
	pool.OnStatusChange(func(nodeID int64, old, new NodeStatus) {
		mu.Lock()
		transitions = append(transitions, struct{ old, new NodeStatus }{old, new})
		mu.Unlock()
	})

	sm := NewStatusMonitor(pool, 0)
	sm.evaluate(time.Now())

	if node.Status != StatusStale {
		t.Errorf("expected status stale, got %s", node.Status)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].old != StatusOnline || transitions[0].new != StatusStale {
		t.Errorf("expected online→stale, got %s→%s", transitions[0].old, transitions[0].new)
	}
}

func TestStatusMonitor_StaleToOffline(t *testing.T) {
	// Node has been stale and hasn't received metrics for > 120s.
	node := &NodeConnection{
		NodeID:      2,
		NodeName:    "test-node-2",
		Status:      StatusStale,
		LastMetrics: time.Now().Add(-125 * time.Second), // 125s ago
	}
	pool := testPool(node)

	var mu sync.Mutex
	var transitions []struct{ old, new NodeStatus }
	pool.OnStatusChange(func(nodeID int64, old, new NodeStatus) {
		mu.Lock()
		transitions = append(transitions, struct{ old, new NodeStatus }{old, new})
		mu.Unlock()
	})

	sm := NewStatusMonitor(pool, 0)
	sm.evaluate(time.Now())

	if node.Status != StatusOffline {
		t.Errorf("expected status offline, got %s", node.Status)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].old != StatusStale || transitions[0].new != StatusOffline {
		t.Errorf("expected stale→offline, got %s→%s", transitions[0].old, transitions[0].new)
	}
}

func TestStatusMonitor_OnlineNoTransitionWithinThreshold(t *testing.T) {
	// Node is online and received metrics recently (within 30s) — no transition.
	node := &NodeConnection{
		NodeID:      3,
		NodeName:    "test-node-3",
		Status:      StatusOnline,
		LastMetrics: time.Now().Add(-10 * time.Second), // 10s ago
	}
	pool := testPool(node)

	var transitions int
	pool.OnStatusChange(func(nodeID int64, old, new NodeStatus) {
		transitions++
	})

	sm := NewStatusMonitor(pool, 0)
	sm.evaluate(time.Now())

	if node.Status != StatusOnline {
		t.Errorf("expected status online, got %s", node.Status)
	}
	if transitions != 0 {
		t.Errorf("expected no transitions, got %d", transitions)
	}
}

func TestStatusMonitor_StaleNoTransitionWithinThreshold(t *testing.T) {
	// Node is stale but within the 120s offline threshold — stays stale.
	node := &NodeConnection{
		NodeID:      4,
		NodeName:    "test-node-4",
		Status:      StatusStale,
		LastMetrics: time.Now().Add(-60 * time.Second), // 60s ago (> 30s but < 120s)
	}
	pool := testPool(node)

	var transitions int
	pool.OnStatusChange(func(nodeID int64, old, new NodeStatus) {
		transitions++
	})

	sm := NewStatusMonitor(pool, 0)
	sm.evaluate(time.Now())

	if node.Status != StatusStale {
		t.Errorf("expected status stale, got %s", node.Status)
	}
	if transitions != 0 {
		t.Errorf("expected no transitions, got %d", transitions)
	}
}

func TestStatusMonitor_OfflineSkipped(t *testing.T) {
	// Offline nodes are skipped — they transition via reconnection logic.
	node := &NodeConnection{
		NodeID:      5,
		NodeName:    "test-node-5",
		Status:      StatusOffline,
		LastMetrics: time.Now().Add(-200 * time.Second), // very old, but already offline
	}
	pool := testPool(node)

	var transitions int
	pool.OnStatusChange(func(nodeID int64, old, new NodeStatus) {
		transitions++
	})

	sm := NewStatusMonitor(pool, 0)
	sm.evaluate(time.Now())

	if node.Status != StatusOffline {
		t.Errorf("expected status offline, got %s", node.Status)
	}
	if transitions != 0 {
		t.Errorf("expected no transitions, got %d", transitions)
	}
}

func TestStatusMonitor_ZeroLastMetricsSkipped(t *testing.T) {
	// Nodes with zero LastMetrics (just connected, no metrics yet) are skipped.
	node := &NodeConnection{
		NodeID:   6,
		NodeName: "test-node-6",
		Status:   StatusOnline,
		// LastMetrics is zero value
	}
	pool := testPool(node)

	var transitions int
	pool.OnStatusChange(func(nodeID int64, old, new NodeStatus) {
		transitions++
	})

	sm := NewStatusMonitor(pool, 0)
	sm.evaluate(time.Now())

	if node.Status != StatusOnline {
		t.Errorf("expected status online, got %s", node.Status)
	}
	if transitions != 0 {
		t.Errorf("expected no transitions, got %d", transitions)
	}
}

func TestStatusMonitor_StartStop(t *testing.T) {
	pool := testPool()
	sm := NewStatusMonitor(pool, 50*time.Millisecond)

	ctx := context.Background()
	sm.Start(ctx)

	// Let it tick a few times.
	time.Sleep(200 * time.Millisecond)

	sm.Stop()
	// Stopping twice should not panic.
	sm.Stop()
}

func TestStatusMonitor_MultipleNodes(t *testing.T) {
	// Mix of nodes: one should go stale, one should go offline, one stays online.
	nodeOnlineOld := &NodeConnection{
		NodeID:      10,
		NodeName:    "node-online-old",
		Status:      StatusOnline,
		LastMetrics: time.Now().Add(-40 * time.Second),
	}
	nodeStaleOld := &NodeConnection{
		NodeID:      11,
		NodeName:    "node-stale-old",
		Status:      StatusStale,
		LastMetrics: time.Now().Add(-130 * time.Second),
	}
	nodeOnlineFresh := &NodeConnection{
		NodeID:      12,
		NodeName:    "node-online-fresh",
		Status:      StatusOnline,
		LastMetrics: time.Now().Add(-5 * time.Second),
	}
	pool := testPool(nodeOnlineOld, nodeStaleOld, nodeOnlineFresh)

	var mu sync.Mutex
	transitionMap := make(map[int64]struct{ old, new NodeStatus })
	pool.OnStatusChange(func(nodeID int64, old, new NodeStatus) {
		mu.Lock()
		transitionMap[nodeID] = struct{ old, new NodeStatus }{old, new}
		mu.Unlock()
	})

	sm := NewStatusMonitor(pool, 0)
	sm.evaluate(time.Now())

	mu.Lock()
	defer mu.Unlock()

	// Node 10: online → stale
	if nodeOnlineOld.Status != StatusStale {
		t.Errorf("node 10: expected stale, got %s", nodeOnlineOld.Status)
	}
	if tr, ok := transitionMap[10]; !ok || tr.old != StatusOnline || tr.new != StatusStale {
		t.Errorf("node 10: unexpected transition %+v", transitionMap[10])
	}

	// Node 11: stale → offline
	if nodeStaleOld.Status != StatusOffline {
		t.Errorf("node 11: expected offline, got %s", nodeStaleOld.Status)
	}
	if tr, ok := transitionMap[11]; !ok || tr.old != StatusStale || tr.new != StatusOffline {
		t.Errorf("node 11: unexpected transition %+v", transitionMap[11])
	}

	// Node 12: stays online (no transition)
	if nodeOnlineFresh.Status != StatusOnline {
		t.Errorf("node 12: expected online, got %s", nodeOnlineFresh.Status)
	}
	if _, ok := transitionMap[12]; ok {
		t.Errorf("node 12: expected no transition, got %+v", transitionMap[12])
	}
}
