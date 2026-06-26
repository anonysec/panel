package grpcclient

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRegisterReconnectSync_TriggersOnOfflineToOnline(t *testing.T) {
	pool := &connPool{
		connections: make(map[int64]*nodeEntry),
	}

	pool.connections[1] = &nodeEntry{
		conn: &NodeConnection{
			NodeID:   1,
			NodeName: "test-node",
			Status:   StatusOffline,
		},
	}

	// Track whether the offline→online transition callback fires.
	var mu sync.Mutex
	var syncCalled bool
	var syncNodeID int64

	pool.OnStatusChange(func(nodeID int64, old, new NodeStatus) {
		if old == StatusOffline && new == StatusOnline {
			mu.Lock()
			syncCalled = true
			syncNodeID = nodeID
			mu.Unlock()
		}
	})

	// Simulate offline → online transition
	pool.SetStatus(1, StatusOnline)

	// Give callback time to fire
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !syncCalled {
		t.Error("expected sync callback to be called on offline→online transition")
	}
	if syncNodeID != 1 {
		t.Errorf("expected syncNodeID=1, got %d", syncNodeID)
	}
}

func TestRegisterReconnectSync_DoesNotTriggerOnOtherTransitions(t *testing.T) {
	pool := &connPool{
		connections: make(map[int64]*nodeEntry),
	}

	pool.connections[1] = &nodeEntry{
		conn: &NodeConnection{
			NodeID:   1,
			NodeName: "test-node",
			Status:   StatusOnline,
		},
	}

	var mu sync.Mutex
	var syncCalled bool

	pool.OnStatusChange(func(nodeID int64, old, new NodeStatus) {
		if old == StatusOffline && new == StatusOnline {
			mu.Lock()
			syncCalled = true
			mu.Unlock()
		}
	})

	// online → stale should NOT trigger the full sync callback
	pool.SetStatus(1, StatusStale)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if syncCalled {
		t.Error("sync callback should NOT be called on online→stale transition")
	}
}

func TestSetStatus_OfflineTriggersReconnection(t *testing.T) {
	// When a node transitions to offline, startReconnect should be spawned.
	// We verify the cancelReco function is set, indicating reconnection was started.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := &connPool{
		connections: make(map[int64]*nodeEntry),
		config:      DefaultPoolConfig(),
		ctx:         ctx,
		cancel:      cancel,
	}
	pool.connections[1] = &nodeEntry{
		conn: &NodeConnection{
			NodeID:   1,
			NodeName: "test-node",
			Status:   StatusOnline,
		},
		nodeConfig: NodeConfig{
			NodeID:  1,
			Name:    "test-node",
			Address: "192.0.2.1", // non-routable TEST-NET address
			Port:    2083,
		},
	}

	pool.SetStatus(1, StatusOffline)

	// Give the goroutine time to start
	time.Sleep(50 * time.Millisecond)

	pool.mu.RLock()
	entry := pool.connections[1]
	hasRecoCancel := entry.cancelReco != nil
	pool.mu.RUnlock()

	if !hasRecoCancel {
		t.Error("expected reconnection goroutine to be started (cancelReco should be set)")
	}
}

func TestSetStatus_SameStatusNoOp(t *testing.T) {
	pool := &connPool{
		connections: make(map[int64]*nodeEntry),
	}

	pool.connections[1] = &nodeEntry{
		conn: &NodeConnection{
			NodeID:   1,
			NodeName: "test-node",
			Status:   StatusOnline,
		},
	}

	var transitions int
	pool.OnStatusChange(func(nodeID int64, old, new NodeStatus) {
		transitions++
	})

	// Setting same status should be a no-op
	pool.SetStatus(1, StatusOnline)

	if transitions != 0 {
		t.Errorf("expected no transitions for same status, got %d", transitions)
	}
}
