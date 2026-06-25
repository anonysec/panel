package grpcclient

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"KorisPanel/panel/internal/dbstore"
)

// mockPoolForSync implements Pool for testing SyncUsers fan-out logic.
type mockPoolForSync struct {
	nodes map[int64]*NodeConnection
}

func (m *mockPoolForSync) Start(ctx context.Context) error                    { return nil }
func (m *mockPoolForSync) Stop() error                                        { return nil }
func (m *mockPoolForSync) Connect(ctx context.Context, node NodeConfig) error { return nil }
func (m *mockPoolForSync) Disconnect(nodeID int64) error                      { return nil }
func (m *mockPoolForSync) Reconnect(nodeID int64) error                       { return nil }
func (m *mockPoolForSync) All() []*NodeConnection {
	var result []*NodeConnection
	for _, n := range m.nodes {
		result = append(result, n)
	}
	return result
}
func (m *mockPoolForSync) OnStatusChange(fn StatusChangeFunc) {}
func (m *mockPoolForSync) Get(nodeID int64) (*NodeConnection, error) {
	n, ok := m.nodes[nodeID]
	if !ok {
		return nil, fmt.Errorf("node %d not found", nodeID)
	}
	return n, nil
}
func (m *mockPoolForSync) Status(nodeID int64) NodeStatus {
	n, ok := m.nodes[nodeID]
	if !ok {
		return StatusOffline
	}
	return n.Status
}

func TestUserSyncService_BuildPayload_ActiveUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open mock: %v", err)
	}
	defer db.Close()

	store := &mockStoreForSync{db: db}
	pool := &mockPoolForSync{nodes: map[int64]*NodeConnection{}}
	svc := NewUserSyncService(pool, store)

	// Customer is active
	mock.ExpectQuery("SELECT status FROM customers").
		WithArgs("testuser").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))

	// Subscription not expired
	mock.ExpectQuery("SELECT expires_at FROM subscriptions").
		WithArgs("testuser").
		WillReturnRows(sqlmock.NewRows([]string{"expires_at"}).AddRow(time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)))

	// Password
	mock.ExpectQuery("SELECT value FROM radcheck").
		WithArgs("testuser").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("secret123"))

	// Max-Data
	mock.ExpectQuery("SELECT value FROM radcheck").
		WithArgs("testuser").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("10737418240"))

	// Simultaneous-Use
	mock.ExpectQuery("SELECT value FROM radcheck").
		WithArgs("testuser").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("3"))

	// Bandwidth
	mock.ExpectQuery("SELECT download_kbps FROM bandwidth_rules").
		WithArgs("testuser").
		WillReturnRows(sqlmock.NewRows([]string{"download_kbps"}).AddRow(5000))

	ctx := context.Background()
	payload, err := svc.buildPayload(ctx, "testuser")
	if err != nil {
		t.Fatalf("buildPayload: %v", err)
	}

	if payload.Username != "testuser" {
		t.Errorf("username = %q, want %q", payload.Username, "testuser")
	}
	if !payload.Enabled {
		t.Error("expected enabled=true for active user")
	}
	if payload.Password != "secret123" {
		t.Errorf("password = %q, want %q", payload.Password, "secret123")
	}
	if payload.MaxDataBytes != 10737418240 {
		t.Errorf("max_data_bytes = %d, want 10737418240", payload.MaxDataBytes)
	}
	if payload.MaxConnections != 3 {
		t.Errorf("max_connections = %d, want 3", payload.MaxConnections)
	}
	if payload.BandwidthBPS != 5000000 {
		t.Errorf("bandwidth_bps = %d, want 5000000", payload.BandwidthBPS)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestUserSyncService_BuildPayload_ExpiredUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open mock: %v", err)
	}
	defer db.Close()

	store := &mockStoreForSync{db: db}
	pool := &mockPoolForSync{nodes: map[int64]*NodeConnection{}}
	svc := NewUserSyncService(pool, store)

	// Customer status is expired
	mock.ExpectQuery("SELECT status FROM customers").
		WithArgs("expireduser").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("expired"))

	// Password (still queried)
	mock.ExpectQuery("SELECT value FROM radcheck").
		WithArgs("expireduser").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("pass"))

	// Max-Data
	mock.ExpectQuery("SELECT value FROM radcheck").
		WithArgs("expireduser").
		WillReturnRows(sqlmock.NewRows([]string{"value"}))

	// Simultaneous-Use
	mock.ExpectQuery("SELECT value FROM radcheck").
		WithArgs("expireduser").
		WillReturnRows(sqlmock.NewRows([]string{"value"}))

	// Bandwidth
	mock.ExpectQuery("SELECT download_kbps FROM bandwidth_rules").
		WithArgs("expireduser").
		WillReturnRows(sqlmock.NewRows([]string{"download_kbps"}))

	ctx := context.Background()
	payload, err := svc.buildPayload(ctx, "expireduser")
	if err != nil {
		t.Fatalf("buildPayload: %v", err)
	}

	if payload.Enabled {
		t.Error("expected enabled=false for expired user")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestUserSyncService_BuildPayload_DisabledUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open mock: %v", err)
	}
	defer db.Close()

	store := &mockStoreForSync{db: db}
	pool := &mockPoolForSync{nodes: map[int64]*NodeConnection{}}
	svc := NewUserSyncService(pool, store)

	mock.ExpectQuery("SELECT status FROM customers").
		WithArgs("disableduser").
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("disabled"))

	mock.ExpectQuery("SELECT value FROM radcheck").
		WithArgs("disableduser").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("pw"))

	mock.ExpectQuery("SELECT value FROM radcheck").
		WithArgs("disableduser").
		WillReturnRows(sqlmock.NewRows([]string{"value"}))

	mock.ExpectQuery("SELECT value FROM radcheck").
		WithArgs("disableduser").
		WillReturnRows(sqlmock.NewRows([]string{"value"}))

	mock.ExpectQuery("SELECT download_kbps FROM bandwidth_rules").
		WithArgs("disableduser").
		WillReturnRows(sqlmock.NewRows([]string{"download_kbps"}))

	ctx := context.Background()
	payload, err := svc.buildPayload(ctx, "disableduser")
	if err != nil {
		t.Fatalf("buildPayload: %v", err)
	}

	if payload.Enabled {
		t.Error("expected enabled=false for disabled user")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestUserSyncService_GetNodesForCoreTypes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open mock: %v", err)
	}
	defer db.Close()

	store := &mockStoreForSync{db: db}
	pool := &mockPoolForSync{
		nodes: map[int64]*NodeConnection{
			1: {NodeID: 1, NodeName: "node1", Status: StatusOnline},
			2: {NodeID: 2, NodeName: "node2", Status: StatusOnline},
			3: {NodeID: 3, NodeName: "node3", Status: StatusOffline},
		},
	}
	svc := NewUserSyncService(pool, store)

	// node_services query returns nodes 1, 2, and 3
	mock.ExpectQuery("SELECT DISTINCT node_id FROM node_services").
		WithArgs("openvpn", "wireguard").
		WillReturnRows(sqlmock.NewRows([]string{"node_id"}).
			AddRow(1).
			AddRow(2).
			AddRow(3))

	ctx := context.Background()
	nodes, err := svc.getNodesForCoreTypes(ctx, []string{"openvpn", "wireguard"})
	if err != nil {
		t.Fatalf("getNodesForCoreTypes: %v", err)
	}

	// Node 3 is offline, should be filtered out
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d: %v", len(nodes), nodes)
	}

	// Verify nodes 1 and 2 are included (order may vary)
	found := map[int64]bool{}
	for _, id := range nodes {
		found[id] = true
	}
	if !found[1] || !found[2] {
		t.Errorf("expected nodes [1,2], got %v", nodes)
	}
	if found[3] {
		t.Error("offline node 3 should not be included")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestUserSyncService_RecordSyncFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open mock: %v", err)
	}
	defer db.Close()

	store := &mockStoreForSync{db: db}
	pool := &mockPoolForSync{nodes: map[int64]*NodeConnection{}}
	svc := NewUserSyncService(pool, store)

	mock.ExpectExec("INSERT INTO sync_failures").
		WithArgs(int64(5), "openvpn", "connection refused", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	payload := UserSyncPayload{
		Username:       "testuser",
		Password:       "pass",
		Enabled:        true,
		MaxDataBytes:   1024,
		MaxConnections: 1,
		BandwidthBPS:   0,
	}

	svc.recordSyncFailure(5, []string{"openvpn"}, payload, "connection refused")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// mockStoreForSync is a minimal dbstore.Store mock that returns the underlying *sql.DB.
type mockStoreForSync struct {
	db *sql.DB
}

func (m *mockStoreForSync) DB() *sql.DB                                   { return m.db }
func (m *mockStoreForSync) Close() error                                  { return nil }
func (m *mockStoreForSync) Ping(ctx context.Context) error                { return nil }
func (m *mockStoreForSync) Migrate(ctx context.Context, dir string) error { return nil }
func (m *mockStoreForSync) Begin(ctx context.Context) (dbstore.Tx, error) { return nil, nil }
func (m *mockStoreForSync) AcquireLock(ctx context.Context, lockID int64) (bool, error) {
	return true, nil
}
func (m *mockStoreForSync) ReleaseLock(ctx context.Context, lockID int64) error { return nil }
func (m *mockStoreForSync) GetSession(ctx context.Context, token string) (*dbstore.Session, error) {
	return nil, nil
}
func (m *mockStoreForSync) SaveSession(ctx context.Context, s *dbstore.Session) error { return nil }
func (m *mockStoreForSync) DeleteSession(ctx context.Context, token string) error     { return nil }
func (m *mockStoreForSync) CleanExpiredSessions(ctx context.Context) error            { return nil }
func (m *mockStoreForSync) InsertMetrics(ctx context.Context, nodeID int64, m2 *dbstore.MetricsRow) error {
	return nil
}
func (m *mockStoreForSync) InsertTrafficLog(ctx context.Context, entry *dbstore.TrafficLogEntry) error {
	return nil
}
func (m *mockStoreForSync) QueryMetrics(ctx context.Context, nodeID int64, from, to time.Time) ([]dbstore.MetricsRow, error) {
	return nil, nil
}
