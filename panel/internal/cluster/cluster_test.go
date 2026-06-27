//go:build !lite

package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func setupMock(t *testing.T) (*ClusterManager, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// checkTableExists query during New()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM information_schema.tables`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	cm := New(db, "test-node-1")
	return cm, mock
}

func TestNew_DefaultNodeID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM information_schema.tables`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	cm := New(db, "")
	if cm.NodeID() == "" {
		t.Error("expected non-empty node ID when using hostname fallback")
	}
}

func TestNew_CustomNodeID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM information_schema.tables`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	cm := New(db, "my-panel-01")
	if cm.NodeID() != "my-panel-01" {
		t.Errorf("expected nodeID %q, got %q", "my-panel-01", cm.NodeID())
	}
}

func TestIsLeader_InitiallyFalse(t *testing.T) {
	cm, _ := setupMock(t)
	if cm.IsLeader() {
		t.Error("expected IsLeader to be false initially")
	}
}

func TestTryBecomeLeader_Success(t *testing.T) {
	cm, mock := setupMock(t)
	ctx := context.Background()

	// pg_try_advisory_lock returns true = acquired
	mock.ExpectQuery(`SELECT pg_try_advisory_lock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))

	// updateRole INSERT ON CONFLICT
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "leader").
		WillReturnResult(sqlmock.NewResult(0, 1))

	acquired, err := cm.TryBecomeLeader(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !acquired {
		t.Error("expected TryBecomeLeader to return true")
	}
	if !cm.IsLeader() {
		t.Error("expected IsLeader to be true after successful acquisition")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTryBecomeLeader_Failure(t *testing.T) {
	cm, mock := setupMock(t)
	ctx := context.Background()

	// pg_try_advisory_lock returns false = not acquired
	mock.ExpectQuery(`SELECT pg_try_advisory_lock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))

	// updateRole INSERT ON CONFLICT (role=follower)
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "follower").
		WillReturnResult(sqlmock.NewResult(0, 1))

	acquired, err := cm.TryBecomeLeader(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acquired {
		t.Error("expected TryBecomeLeader to return false when lock not acquired")
	}
	if cm.IsLeader() {
		t.Error("expected IsLeader to be false when lock not acquired")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestTryBecomeLeader_OnlyOneCanBeLeader(t *testing.T) {
	// Simulate two nodes trying to acquire leadership.
	// Node 1 succeeds, node 2 fails.
	db1, mock1, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db1.Close()

	db2, mock2, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	// Both nodes: table exists
	mock1.ExpectQuery(`SELECT COUNT\(\*\) FROM information_schema.tables`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock2.ExpectQuery(`SELECT COUNT\(\*\) FROM information_schema.tables`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	cm1 := New(db1, "node-1")
	cm2 := New(db2, "node-2")

	ctx := context.Background()

	// Node 1 acquires successfully
	mock1.ExpectQuery(`SELECT pg_try_advisory_lock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock1.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("node-1", "leader").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Node 2 fails (lock held by node 1)
	mock2.ExpectQuery(`SELECT pg_try_advisory_lock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(false))
	mock2.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("node-2", "follower").
		WillReturnResult(sqlmock.NewResult(0, 1))

	acquired1, err := cm1.TryBecomeLeader(ctx)
	if err != nil {
		t.Fatalf("node-1 error: %v", err)
	}
	acquired2, err := cm2.TryBecomeLeader(ctx)
	if err != nil {
		t.Fatalf("node-2 error: %v", err)
	}

	if !acquired1 {
		t.Error("expected node-1 to become leader")
	}
	if acquired2 {
		t.Error("expected node-2 to NOT become leader")
	}
	if !cm1.IsLeader() {
		t.Error("node-1 should report as leader")
	}
	if cm2.IsLeader() {
		t.Error("node-2 should report as follower")
	}

	if err := mock1.ExpectationsWereMet(); err != nil {
		t.Errorf("node-1 unmet expectations: %v", err)
	}
	if err := mock2.ExpectationsWereMet(); err != nil {
		t.Errorf("node-2 unmet expectations: %v", err)
	}
}

func TestReleaseLeadership(t *testing.T) {
	cm, mock := setupMock(t)
	ctx := context.Background()

	// First acquire leadership
	mock.ExpectQuery(`SELECT pg_try_advisory_lock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "leader").
		WillReturnResult(sqlmock.NewResult(0, 1))

	_, err := cm.TryBecomeLeader(ctx)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}

	// Now release
	mock.ExpectExec(`SELECT pg_advisory_unlock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "follower").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = cm.ReleaseLeadership(ctx)
	if err != nil {
		t.Fatalf("release failed: %v", err)
	}
	if cm.IsLeader() {
		t.Error("expected IsLeader to be false after ReleaseLeadership")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestHeartbeat(t *testing.T) {
	cm, mock := setupMock(t)
	ctx := context.Background()

	// Heartbeat as follower
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "follower").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := cm.Heartbeat(ctx)
	if err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestHeartbeat_AsLeader(t *testing.T) {
	cm, mock := setupMock(t)
	ctx := context.Background()

	// Acquire leadership first
	mock.ExpectQuery(`SELECT pg_try_advisory_lock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "leader").
		WillReturnResult(sqlmock.NewResult(0, 1))

	_, err := cm.TryBecomeLeader(ctx)
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}

	// Heartbeat as leader
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "leader").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = cm.Heartbeat(ctx)
	if err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestHeartbeat_NoTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	// Table does NOT exist — single-node mode
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM information_schema.tables`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	cm := New(db, "solo-node")
	ctx := context.Background()

	// Heartbeat should be a no-op when table doesn't exist
	err = cm.Heartbeat(ctx)
	if err != nil {
		t.Fatalf("heartbeat should be no-op but got error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestIsLeader_StateChanges(t *testing.T) {
	cm, mock := setupMock(t)
	ctx := context.Background()

	// Initially not leader
	if cm.IsLeader() {
		t.Error("should not be leader initially")
	}

	// Acquire leadership
	mock.ExpectQuery(`SELECT pg_try_advisory_lock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "leader").
		WillReturnResult(sqlmock.NewResult(0, 1))

	cm.TryBecomeLeader(ctx)
	if !cm.IsLeader() {
		t.Error("should be leader after acquire")
	}

	// Release leadership
	mock.ExpectExec(`SELECT pg_advisory_unlock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "follower").
		WillReturnResult(sqlmock.NewResult(0, 1))

	cm.ReleaseLeadership(ctx)
	if cm.IsLeader() {
		t.Error("should not be leader after release")
	}

	// Re-acquire
	mock.ExpectQuery(`SELECT pg_try_advisory_lock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "leader").
		WillReturnResult(sqlmock.NewResult(0, 1))

	cm.TryBecomeLeader(ctx)
	if !cm.IsLeader() {
		t.Error("should be leader again after re-acquire")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestRunLeaderElection_CancelStops(t *testing.T) {
	cm, mock := setupMock(t)

	// Heartbeat on start
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "follower").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Initial TryBecomeLeader
	mock.ExpectQuery(`SELECT pg_try_advisory_lock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "leader").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// ReleaseLeadership on shutdown
	mock.ExpectExec(`SELECT pg_advisory_unlock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO cluster_nodes`).
		WithArgs("test-node-1", "follower").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		cm.RunLeaderElection(ctx, 100*time.Millisecond)
		close(done)
	}()

	// Give it a moment to start, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK — RunLeaderElection returned.
	case <-time.After(2 * time.Second):
		t.Fatal("RunLeaderElection did not return after context cancel")
	}
}

func TestSingleNode_NoTable_IsLeader(t *testing.T) {
	// When the cluster_nodes table doesn't exist, TryBecomeLeader still
	// works via the advisory lock. The node becomes leader without registering.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM information_schema.tables`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	cm := New(db, "solo-node")
	ctx := context.Background()

	// Advisory lock still works (no table needed)
	mock.ExpectQuery(`SELECT pg_try_advisory_lock\(hashtext\(\$1\)\)`).
		WithArgs(LockName).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))

	acquired, err := cm.TryBecomeLeader(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !acquired {
		t.Error("single node should always become leader")
	}
	if !cm.IsLeader() {
		t.Error("single node should report as leader")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
