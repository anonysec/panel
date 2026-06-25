package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"KorisPanel/panel/internal/dbstore"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	// Create required tables for testing.
	ctx := context.Background()
	_, err = store.DB().ExecContext(ctx, `
		CREATE TABLE panel_sessions (
			token TEXT PRIMARY KEY,
			admin_id INTEGER,
			customer_id INTEGER,
			data BLOB,
			ip_address TEXT,
			user_agent TEXT,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			last_seen TEXT NOT NULL
		);
		CREATE TABLE node_metrics_history (
			time TEXT NOT NULL,
			node_id INTEGER NOT NULL,
			cpu_percent REAL,
			ram_percent REAL,
			disk_percent REAL,
			rx_bps INTEGER,
			tx_bps INTEGER,
			active_sessions INTEGER,
			uptime_seconds INTEGER
		);
		CREATE TABLE user_traffic_log (
			time TEXT NOT NULL,
			user_id INTEGER NOT NULL,
			node_id INTEGER NOT NULL,
			rx_bytes INTEGER NOT NULL DEFAULT 0,
			tx_bytes INTEGER NOT NULL DEFAULT 0
		);
	`)
	if err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}
	return store
}

func TestNew_InMemory(t *testing.T) {
	store, err := New(":memory:")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer store.Close()

	if store.DB() == nil {
		t.Fatal("expected non-nil DB")
	}
}

func TestPing(t *testing.T) {
	store := newTestStore(t)
	if err := store.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestAcquireReleaseLock(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// First acquire should succeed.
	ok, err := store.AcquireLock(ctx, 1001)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected lock to be acquired")
	}

	// Second acquire of same lock should fail.
	ok, err = store.AcquireLock(ctx, 1001)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected lock to NOT be acquired (already held)")
	}

	// Different lock ID should succeed.
	ok, err = store.AcquireLock(ctx, 1002)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected different lock to be acquired")
	}

	// Release the first lock.
	if err := store.ReleaseLock(ctx, 1001); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	// Now we can acquire it again.
	ok, err = store.AcquireLock(ctx, 1001)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected lock to be re-acquired after release")
	}
}

func TestReleaseLock_NotHeld(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.ReleaseLock(ctx, 9999)
	if err != dbstore.ErrLockNotAcquired {
		t.Fatalf("expected ErrLockNotAcquired, got %v", err)
	}
}

func TestSessionCRUD(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	sess := &dbstore.Session{
		Token:      "test-token-123",
		AdminID:    sql.NullInt64{Int64: 1, Valid: true},
		CustomerID: sql.NullInt64{},
		Data:       []byte(`{"role":"admin"}`),
		IPAddress:  "192.168.1.1",
		UserAgent:  "TestAgent/1.0",
		CreatedAt:  now,
		ExpiresAt:  now.Add(24 * time.Hour),
		LastSeen:   now,
	}

	// Save session.
	if err := store.SaveSession(ctx, sess); err != nil {
		t.Fatalf("save session failed: %v", err)
	}

	// Get session.
	got, err := store.GetSession(ctx, "test-token-123")
	if err != nil {
		t.Fatalf("get session failed: %v", err)
	}
	if got.Token != sess.Token {
		t.Errorf("token mismatch: got %q, want %q", got.Token, sess.Token)
	}
	if got.AdminID.Int64 != 1 || !got.AdminID.Valid {
		t.Errorf("admin_id mismatch: got %v", got.AdminID)
	}
	if got.IPAddress != "192.168.1.1" {
		t.Errorf("ip mismatch: got %q", got.IPAddress)
	}

	// Delete session.
	if err := store.DeleteSession(ctx, "test-token-123"); err != nil {
		t.Fatalf("delete session failed: %v", err)
	}

	// Get should fail.
	_, err = store.GetSession(ctx, "test-token-123")
	if err != dbstore.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetSession_Expired(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	sess := &dbstore.Session{
		Token:     "expired-token",
		CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-1 * time.Hour), // already expired
		LastSeen:  now.Add(-2 * time.Hour),
	}

	if err := store.SaveSession(ctx, sess); err != nil {
		t.Fatalf("save session failed: %v", err)
	}

	_, err := store.GetSession(ctx, "expired-token")
	if err != dbstore.ErrSessionExpired {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.GetSession(ctx, "nonexistent")
	if err != dbstore.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCleanExpiredSessions(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)

	// Insert an expired session and a valid session.
	expired := &dbstore.Session{
		Token:     "expired",
		CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-1 * time.Hour),
		LastSeen:  now.Add(-2 * time.Hour),
	}
	valid := &dbstore.Session{
		Token:     "valid",
		CreatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour),
		LastSeen:  now,
	}

	store.SaveSession(ctx, expired)
	store.SaveSession(ctx, valid)

	if err := store.CleanExpiredSessions(ctx); err != nil {
		t.Fatalf("clean failed: %v", err)
	}

	// Valid session should still exist.
	_, err := store.GetSession(ctx, "valid")
	if err != nil {
		t.Fatalf("valid session should still exist: %v", err)
	}

	// Expired session should be gone.
	_, err = store.GetSession(ctx, "expired")
	if err != dbstore.ErrNotFound {
		t.Fatalf("expected expired session to be cleaned, got %v", err)
	}
}

func TestInsertAndQueryMetrics(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	m := &dbstore.MetricsRow{
		Time:           now,
		CPUPercent:     65.5,
		RAMPercent:     72.3,
		DiskPercent:    45.0,
		RxBPS:          1000000,
		TxBPS:          500000,
		ActiveSessions: 42,
		UptimeSeconds:  86400,
	}

	if err := store.InsertMetrics(ctx, 1, m); err != nil {
		t.Fatalf("insert metrics failed: %v", err)
	}

	results, err := store.QueryMetrics(ctx, 1, now.Add(-1*time.Minute), now.Add(1*time.Minute))
	if err != nil {
		t.Fatalf("query metrics failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 row, got %d", len(results))
	}

	got := results[0]
	if got.CPUPercent != 65.5 {
		t.Errorf("cpu mismatch: got %v", got.CPUPercent)
	}
	if got.RAMPercent != 72.3 {
		t.Errorf("ram mismatch: got %v", got.RAMPercent)
	}
	if got.ActiveSessions != 42 {
		t.Errorf("sessions mismatch: got %v", got.ActiveSessions)
	}
	if got.UptimeSeconds != 86400 {
		t.Errorf("uptime mismatch: got %v", got.UptimeSeconds)
	}
}

func TestInsertTrafficLog(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	entry := &dbstore.TrafficLogEntry{
		Time:    now,
		UserID:  10,
		NodeID:  1,
		RxBytes: 1048576,
		TxBytes: 524288,
	}

	if err := store.InsertTrafficLog(ctx, entry); err != nil {
		t.Fatalf("insert traffic log failed: %v", err)
	}

	// Verify via raw query.
	var rxBytes, txBytes int64
	err := store.DB().QueryRowContext(ctx, "SELECT rx_bytes, tx_bytes FROM user_traffic_log WHERE user_id = ?", 10).Scan(&rxBytes, &txBytes)
	if err != nil {
		t.Fatalf("query traffic log failed: %v", err)
	}
	if rxBytes != 1048576 || txBytes != 524288 {
		t.Errorf("traffic mismatch: rx=%d tx=%d", rxBytes, txBytes)
	}
}

func TestTransaction(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tx, err := store.Begin(ctx)
	if err != nil {
		t.Fatalf("begin failed: %v", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO user_traffic_log (time, user_id, node_id, rx_bytes, tx_bytes) VALUES (?, ?, ?, ?, ?)`,
		time.Now().Format(time.RFC3339), 1, 1, 100, 200)
	if err != nil {
		t.Fatalf("exec in tx failed: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	// Verify committed.
	var count int
	store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM user_traffic_log").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}
}

func TestTransaction_Rollback(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tx, err := store.Begin(ctx)
	if err != nil {
		t.Fatalf("begin failed: %v", err)
	}

	_, err = tx.Exec(ctx, `INSERT INTO user_traffic_log (time, user_id, node_id, rx_bytes, tx_bytes) VALUES (?, ?, ?, ?, ?)`,
		time.Now().Format(time.RFC3339), 1, 1, 100, 200)
	if err != nil {
		t.Fatalf("exec in tx failed: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	// Verify nothing committed.
	var count int
	store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM user_traffic_log").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d", count)
	}
}

func TestMigrate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create a temporary migrations directory.
	dir := t.TempDir()

	// Write a migration file.
	migration := `CREATE TABLE test_items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL);`
	if err := os.WriteFile(filepath.Join(dir, "001_create_test.sql"), []byte(migration), 0644); err != nil {
		t.Fatalf("write migration file: %v", err)
	}

	// Run migrations.
	if err := store.Migrate(ctx, dir); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	// Table should exist now.
	_, err := store.DB().ExecContext(ctx, "INSERT INTO test_items (name) VALUES ('hello')")
	if err != nil {
		t.Fatalf("insert into migrated table failed: %v", err)
	}

	// Running again should be idempotent (skip already applied).
	if err := store.Migrate(ctx, dir); err != nil {
		t.Fatalf("second migrate failed: %v", err)
	}
}

func TestDeleteSession_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	err := store.DeleteSession(ctx, "nonexistent-token")
	if err != dbstore.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
