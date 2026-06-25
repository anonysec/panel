package grpcclient

import (
	"context"
	"sync"
	"testing"
	"time"

	"KorisPanel/panel/internal/alerts"
)

// mockAlerter collects emitted alerts for test assertions.
type mockAlerter struct {
	mu     sync.Mutex
	alerts []alerts.Alert
}

func (m *mockAlerter) Emit(a alerts.Alert) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerts = append(m.alerts, a)
}

func (m *mockAlerter) getAlerts() []alerts.Alert {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]alerts.Alert, len(m.alerts))
	copy(result, m.alerts)
	return result
}

func TestNewHealthChecker_DefaultInterval(t *testing.T) {
	pool := &connPool{connections: make(map[int64]*nodeEntry)}
	hc := NewHealthChecker(pool, nil, 0)
	if hc.interval != DefaultHealthCheckInterval {
		t.Errorf("expected interval %s, got %s", DefaultHealthCheckInterval, hc.interval)
	}
}

func TestNewHealthChecker_CustomInterval(t *testing.T) {
	pool := &connPool{connections: make(map[int64]*nodeEntry)}
	hc := NewHealthChecker(pool, nil, 30*time.Second)
	if hc.interval != 30*time.Second {
		t.Errorf("expected interval 30s, got %s", hc.interval)
	}
}

func TestHealthChecker_StartStop(t *testing.T) {
	pool := &connPool{connections: make(map[int64]*nodeEntry)}
	hc := NewHealthChecker(pool, nil, time.Second)

	ctx := context.Background()
	hc.Start(ctx)

	// Give it a moment to start the goroutine
	time.Sleep(50 * time.Millisecond)

	hc.Stop()

	// Stopping again should be safe
	hc.Stop()
}

func TestHealthChecker_CheckNodeOK(t *testing.T) {
	pool := &connPool{connections: make(map[int64]*nodeEntry)}
	pool.connections[1] = &nodeEntry{
		conn: &NodeConnection{
			NodeID:      1,
			NodeName:    "test-node",
			Status:      StatusOnline,
			LastMetrics: time.Now(),
		},
	}

	alerter := &mockAlerter{}
	hc := NewHealthChecker(pool, alerter, time.Minute)

	// The stub returns HealthOK, so no alerts should be emitted
	hc.checkNode(context.Background(), 1)

	if len(alerter.getAlerts()) != 0 {
		t.Errorf("expected no alerts, got %d", len(alerter.getAlerts()))
	}

	// Status should remain online
	pool.mu.RLock()
	status := pool.connections[1].conn.Status
	pool.mu.RUnlock()
	if status != StatusOnline {
		t.Errorf("expected status online, got %s", status)
	}
}

func TestHealthChecker_CheckAllSkipsOfflineNodes(t *testing.T) {
	pool := &connPool{connections: make(map[int64]*nodeEntry)}
	pool.connections[1] = &nodeEntry{
		conn: &NodeConnection{
			NodeID: 1,
			Status: StatusOffline,
		},
	}
	pool.connections[2] = &nodeEntry{
		conn: &NodeConnection{
			NodeID:      2,
			Status:      StatusOnline,
			LastMetrics: time.Now(),
		},
	}

	alerter := &mockAlerter{}
	hc := NewHealthChecker(pool, alerter, time.Minute)

	// checkAll should only check node 2 (online), not node 1 (offline)
	hc.checkAll(context.Background())

	// With the stub returning OK, no alerts expected
	if len(alerter.getAlerts()) != 0 {
		t.Errorf("expected no alerts, got %d", len(alerter.getAlerts()))
	}
}

func TestHealthChecker_CheckNodeDegraded(t *testing.T) {
	pool := &connPool{connections: make(map[int64]*nodeEntry)}
	pool.connections[1] = &nodeEntry{
		conn: &NodeConnection{
			NodeID:      1,
			NodeName:    "test-node",
			Status:      StatusOnline,
			LastMetrics: time.Now(),
		},
	}

	alerter := &mockAlerter{}
	hc := &HealthChecker{
		pool:     pool,
		alerter:  alerter,
		interval: time.Minute,
	}

	// Override callHealthRPC to return DEGRADED
	origCall := hc.callHealthRPC
	_ = origCall
	hc.callHealthRPCFunc = func(ctx context.Context, nodeID int64) (HealthStatus, error) {
		return HealthDegraded, nil
	}

	hc.checkNode(context.Background(), 1)

	gotAlerts := alerter.getAlerts()
	if len(gotAlerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(gotAlerts))
	}
	if gotAlerts[0].Type != alerts.AlertNodeDegraded {
		t.Errorf("expected alert type %s, got %s", alerts.AlertNodeDegraded, gotAlerts[0].Type)
	}

	// Status should now be stale
	pool.mu.RLock()
	status := pool.connections[1].conn.Status
	pool.mu.RUnlock()
	if status != StatusStale {
		t.Errorf("expected status stale, got %s", status)
	}
}

func TestHealthChecker_CheckNodeUnhealthy(t *testing.T) {
	pool := &connPool{connections: make(map[int64]*nodeEntry)}
	pool.connections[1] = &nodeEntry{
		conn: &NodeConnection{
			NodeID:      1,
			NodeName:    "test-node",
			Status:      StatusOnline,
			LastMetrics: time.Now(),
		},
	}

	alerter := &mockAlerter{}
	hc := &HealthChecker{
		pool:     pool,
		alerter:  alerter,
		interval: time.Minute,
	}

	// Override callHealthRPC to return UNHEALTHY
	hc.callHealthRPCFunc = func(ctx context.Context, nodeID int64) (HealthStatus, error) {
		return HealthUnhealthy, nil
	}

	hc.checkNode(context.Background(), 1)

	gotAlerts := alerter.getAlerts()
	if len(gotAlerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(gotAlerts))
	}
	if gotAlerts[0].Type != alerts.AlertNodeDegraded {
		t.Errorf("expected alert type %s, got %s", alerts.AlertNodeDegraded, gotAlerts[0].Type)
	}

	// Status should now be offline
	pool.mu.RLock()
	status := pool.connections[1].conn.Status
	pool.mu.RUnlock()
	if status != StatusOffline {
		t.Errorf("expected status offline, got %s", status)
	}
}
