package grpcclient

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"KorisPanel/panel/internal/dbstore"
)

// mockStore implements dbstore.Store for testing purposes.
type mockStore struct {
	db              *sql.DB
	insertedMetrics []*dbstore.MetricsRow
	insertedNodeID  int64
	insertErr       error
}

func (m *mockStore) DB() *sql.DB                                          { return m.db }
func (m *mockStore) Close() error                                         { return nil }
func (m *mockStore) Ping(_ context.Context) error                         { return nil }
func (m *mockStore) Migrate(_ context.Context, _ string) error            { return nil }
func (m *mockStore) Begin(_ context.Context) (dbstore.Tx, error)          { return nil, nil }
func (m *mockStore) AcquireLock(_ context.Context, _ int64) (bool, error) { return true, nil }
func (m *mockStore) ReleaseLock(_ context.Context, _ int64) error         { return nil }
func (m *mockStore) GetSession(_ context.Context, _ string) (*dbstore.Session, error) {
	return nil, nil
}
func (m *mockStore) SaveSession(_ context.Context, _ *dbstore.Session) error { return nil }
func (m *mockStore) DeleteSession(_ context.Context, _ string) error         { return nil }
func (m *mockStore) CleanExpiredSessions(_ context.Context) error            { return nil }
func (m *mockStore) InsertMetrics(_ context.Context, nodeID int64, row *dbstore.MetricsRow) error {
	m.insertedNodeID = nodeID
	m.insertedMetrics = append(m.insertedMetrics, row)
	return m.insertErr
}
func (m *mockStore) InsertTrafficLog(_ context.Context, _ *dbstore.TrafficLogEntry) error {
	return nil
}
func (m *mockStore) QueryMetrics(_ context.Context, _ int64, _, _ time.Time) ([]dbstore.MetricsRow, error) {
	return nil, nil
}

func TestMetricsConsumer_ProcessEvent_InsertMetrics(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	store := &mockStore{db: db}

	pool := &connPool{
		connections: map[int64]*nodeEntry{
			1: {
				conn: &NodeConnection{
					NodeID:   1,
					NodeName: "test-node",
					Status:   StatusOnline,
				},
			},
		},
	}

	consumer := NewMetricsConsumer(store, pool)

	event := MetricsEvent{
		CPUPercent:     45.5,
		RAMPercent:     62.3,
		DiskPercent:    28.7,
		RxBPS:          1024000,
		TxBPS:          512000,
		ActiveSessions: 12,
		UptimeSeconds:  86400,
		Cores: []CoreStatus{
			{Type: "openvpn", State: "running", ActiveSessions: 8, PID: 1234},
			{Type: "wireguard", State: "running", ActiveSessions: 4, PID: 5678},
		},
	}

	// Expect node_status upsert
	mock.ExpectExec(`INSERT INTO node_status`).
		WithArgs(int64(1), 45.5, 62.3, 28.7, int64(1024000), int64(512000), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect gRPC fields update
	mock.ExpectExec(`UPDATE node_status`).
		WithArgs(sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect node_services upsert for openvpn
	mock.ExpectExec(`INSERT INTO node_services`).
		WithArgs(int64(1), "openvpn", "running").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect node_services upsert for wireguard
	mock.ExpectExec(`INSERT INTO node_services`).
		WithArgs(int64(1), "wireguard", "running").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = consumer.ProcessEvent(1, event)
	if err != nil {
		t.Fatalf("ProcessEvent returned error: %v", err)
	}

	// Verify InsertMetrics was called with correct data
	if store.insertedNodeID != 1 {
		t.Errorf("expected node ID 1, got %d", store.insertedNodeID)
	}
	if len(store.insertedMetrics) != 1 {
		t.Fatalf("expected 1 inserted metrics row, got %d", len(store.insertedMetrics))
	}

	row := store.insertedMetrics[0]
	if row.CPUPercent != 45.5 {
		t.Errorf("CPUPercent: expected 45.5, got %f", row.CPUPercent)
	}
	if row.RAMPercent != 62.3 {
		t.Errorf("RAMPercent: expected 62.3, got %f", row.RAMPercent)
	}
	if row.DiskPercent != 28.7 {
		t.Errorf("DiskPercent: expected 28.7, got %f", row.DiskPercent)
	}
	if row.RxBPS != 1024000 {
		t.Errorf("RxBPS: expected 1024000, got %d", row.RxBPS)
	}
	if row.TxBPS != 512000 {
		t.Errorf("TxBPS: expected 512000, got %d", row.TxBPS)
	}
	if row.ActiveSessions != 12 {
		t.Errorf("ActiveSessions: expected 12, got %d", row.ActiveSessions)
	}
	if row.UptimeSeconds != 86400 {
		t.Errorf("UptimeSeconds: expected 86400, got %d", row.UptimeSeconds)
	}

	// Verify pool's LastMetrics was updated
	pool.mu.RLock()
	entry := pool.connections[1]
	pool.mu.RUnlock()
	if entry.conn.LastMetrics.IsZero() {
		t.Error("expected LastMetrics to be updated, but it's still zero")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestMetricsConsumer_ProcessEvent_NoCores(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	store := &mockStore{db: db}
	pool := &connPool{
		connections: map[int64]*nodeEntry{
			5: {
				conn: &NodeConnection{
					NodeID:   5,
					NodeName: "no-cores-node",
					Status:   StatusOnline,
				},
			},
		},
	}

	consumer := NewMetricsConsumer(store, pool)

	event := MetricsEvent{
		CPUPercent:     10.0,
		RAMPercent:     20.0,
		DiskPercent:    30.0,
		RxBPS:          100,
		TxBPS:          200,
		ActiveSessions: 0,
		UptimeSeconds:  3600,
		Cores:          nil,
	}

	// Expect node_status upsert
	mock.ExpectExec(`INSERT INTO node_status`).
		WithArgs(int64(5), 10.0, 20.0, 30.0, int64(100), int64(200), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Expect gRPC fields update
	mock.ExpectExec(`UPDATE node_status`).
		WithArgs(sqlmock.AnyArg(), int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// No node_services calls expected since Cores is nil

	err = consumer.ProcessEvent(5, event)
	if err != nil {
		t.Fatalf("ProcessEvent returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestMetricsConsumer_ProcessEvent_UpdatesPoolLastMetrics(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	store := &mockStore{db: db}
	before := time.Now().Add(-time.Hour)
	pool := &connPool{
		connections: map[int64]*nodeEntry{
			3: {
				conn: &NodeConnection{
					NodeID:      3,
					NodeName:    "stale-node",
					Status:      StatusStale,
					LastMetrics: before,
				},
			},
		},
	}

	consumer := NewMetricsConsumer(store, pool)

	event := MetricsEvent{
		CPUPercent: 5.0,
		RAMPercent: 10.0,
	}

	mock.ExpectExec(`INSERT INTO node_status`).
		WithArgs(int64(3), 5.0, 10.0, 0.0, int64(0), int64(0), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE node_status`).
		WithArgs(sqlmock.AnyArg(), int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	_ = consumer.ProcessEvent(3, event)

	pool.mu.RLock()
	entry := pool.connections[3]
	pool.mu.RUnlock()
	if !entry.conn.LastMetrics.After(before) {
		t.Error("expected LastMetrics to be updated to a time after 'before'")
	}
}

func TestNodeStatusFromEvent(t *testing.T) {
	event := MetricsEvent{
		CPUPercent:     75.0,
		RAMPercent:     80.0,
		DiskPercent:    50.0,
		RxBPS:          2048,
		TxBPS:          1024,
		ActiveSessions: 5,
		UptimeSeconds:  7200,
		Cores: []CoreStatus{
			{Type: "openvpn", State: "running"},
			{Type: "wireguard", State: "stopped"},
		},
	}

	result := NodeStatusFromEvent(event)
	if result["cpu"] != 75.0 {
		t.Errorf("expected cpu 75.0, got %v", result["cpu"])
	}
	if result["cores"] != 2 {
		t.Errorf("expected 2 cores, got %v", result["cores"])
	}
}
