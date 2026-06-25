package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"KorisPanel/panel/internal/dbstore"
)

// mockCoordStore implements dbstore.Store for testing the Coordinator.
// Only AcquireLock and ReleaseLock are exercised by Coordinator.
type mockCoordStore struct {
	locks      map[int64]bool
	acquireErr error
	releaseErr error
}

func newMockCoordStore() *mockCoordStore {
	return &mockCoordStore{locks: make(map[int64]bool)}
}

func (m *mockCoordStore) DB() *sql.DB                                 { return nil }
func (m *mockCoordStore) Close() error                                { return nil }
func (m *mockCoordStore) Ping(_ context.Context) error                { return nil }
func (m *mockCoordStore) Migrate(_ context.Context, _ string) error   { return nil }
func (m *mockCoordStore) Begin(_ context.Context) (dbstore.Tx, error) { return nil, nil }
func (m *mockCoordStore) GetSession(_ context.Context, _ string) (*dbstore.Session, error) {
	return nil, nil
}
func (m *mockCoordStore) SaveSession(_ context.Context, _ *dbstore.Session) error { return nil }
func (m *mockCoordStore) DeleteSession(_ context.Context, _ string) error         { return nil }
func (m *mockCoordStore) CleanExpiredSessions(_ context.Context) error            { return nil }
func (m *mockCoordStore) InsertMetrics(_ context.Context, _ int64, _ *dbstore.MetricsRow) error {
	return nil
}
func (m *mockCoordStore) InsertTrafficLog(_ context.Context, _ *dbstore.TrafficLogEntry) error {
	return nil
}
func (m *mockCoordStore) QueryMetrics(_ context.Context, _ int64, _, _ time.Time) ([]dbstore.MetricsRow, error) {
	return nil, nil
}

func (m *mockCoordStore) AcquireLock(_ context.Context, lockID int64) (bool, error) {
	if m.acquireErr != nil {
		return false, m.acquireErr
	}
	if m.locks[lockID] {
		return false, nil
	}
	m.locks[lockID] = true
	return true, nil
}

func (m *mockCoordStore) ReleaseLock(_ context.Context, lockID int64) error {
	if m.releaseErr != nil {
		return m.releaseErr
	}
	delete(m.locks, lockID)
	return nil
}

// Compile-time check that mockCoordStore satisfies dbstore.Store.
var _ dbstore.Store = (*mockCoordStore)(nil)

func TestNewCoordinator_WorkerID(t *testing.T) {
	store := newMockCoordStore()
	coord := NewCoordinator(store)

	hostname, _ := os.Hostname()
	expected := fmt.Sprintf("%s-%d", hostname, os.Getpid())

	if coord.WorkerID() != expected {
		t.Errorf("expected workerID %q, got %q", expected, coord.WorkerID())
	}
}

func TestNewCoordinatorWithID(t *testing.T) {
	store := newMockCoordStore()
	coord := NewCoordinatorWithID(store, "test-worker-42")

	if coord.WorkerID() != "test-worker-42" {
		t.Errorf("expected workerID %q, got %q", "test-worker-42", coord.WorkerID())
	}
}

func TestCoordinator_TryRun_Acquired(t *testing.T) {
	store := newMockCoordStore()
	coord := NewCoordinatorWithID(store, "w1")
	ctx := context.Background()

	executed := false
	ran, err := coord.TryRun(ctx, LockExpiry, func(ctx context.Context) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran {
		t.Error("expected TryRun to report task was executed")
	}
	if !executed {
		t.Error("expected fn to be called")
	}
}

func TestCoordinator_TryRun_NotAcquired(t *testing.T) {
	store := newMockCoordStore()
	// Simulate another worker holding the lock.
	store.locks[LockExpiry] = true

	coord := NewCoordinatorWithID(store, "w2")
	ctx := context.Background()

	executed := false
	ran, err := coord.TryRun(ctx, LockExpiry, func(ctx context.Context) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ran {
		t.Error("expected TryRun to report task was NOT executed")
	}
	if executed {
		t.Error("expected fn NOT to be called when lock not acquired")
	}
}

func TestCoordinator_TryRun_FnError(t *testing.T) {
	store := newMockCoordStore()
	coord := NewCoordinatorWithID(store, "w1")
	ctx := context.Background()

	taskErr := errors.New("task failed")
	ran, err := coord.TryRun(ctx, LockBilling, func(ctx context.Context) error {
		return taskErr
	})

	if !ran {
		t.Error("expected TryRun to report task was executed (lock acquired)")
	}
	if !errors.Is(err, taskErr) {
		t.Errorf("expected error %v, got %v", taskErr, err)
	}
}

func TestCoordinator_TryRun_AcquireError(t *testing.T) {
	store := newMockCoordStore()
	store.acquireErr = errors.New("db connection lost")

	coord := NewCoordinatorWithID(store, "w1")
	ctx := context.Background()

	ran, err := coord.TryRun(ctx, LockExpiry, func(ctx context.Context) error {
		t.Fatal("fn should not be called when acquire fails")
		return nil
	})

	if ran {
		t.Error("expected ran=false when acquire errors")
	}
	if err == nil {
		t.Error("expected error from acquire failure")
	}
}

func TestCoordinator_TryRun_ReleasesLock(t *testing.T) {
	store := newMockCoordStore()
	coord := NewCoordinatorWithID(store, "w1")
	ctx := context.Background()

	_, _ = coord.TryRun(ctx, LockTraffic, func(ctx context.Context) error {
		return nil
	})

	// After TryRun completes, the lock should be released.
	if store.locks[LockTraffic] {
		t.Error("expected lock to be released after TryRun")
	}
}

func TestCoordinator_LeaderElect_Success(t *testing.T) {
	store := newMockCoordStore()
	coord := NewCoordinatorWithID(store, "w1")
	ctx := context.Background()

	acquired, err := coord.LeaderElect(ctx, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !acquired {
		t.Error("expected LeaderElect to succeed")
	}

	// Verify the lock ID is offset correctly.
	expectedLockID := LeaderElectOffset + 5
	if !store.locks[expectedLockID] {
		t.Errorf("expected lock %d to be held", expectedLockID)
	}
}

func TestCoordinator_LeaderElect_AlreadyHeld(t *testing.T) {
	store := newMockCoordStore()
	// Simulate another worker holding node 7's lock.
	store.locks[LeaderElectOffset+7] = true

	coord := NewCoordinatorWithID(store, "w2")
	ctx := context.Background()

	acquired, err := coord.LeaderElect(ctx, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acquired {
		t.Error("expected LeaderElect to fail when lock already held")
	}
}

func TestCoordinator_ReleaseNodeLock(t *testing.T) {
	store := newMockCoordStore()
	coord := NewCoordinatorWithID(store, "w1")
	ctx := context.Background()

	// Acquire first.
	_, _ = coord.LeaderElect(ctx, 3)
	if !store.locks[LeaderElectOffset+3] {
		t.Fatal("precondition: lock should be held")
	}

	// Release.
	err := coord.ReleaseNodeLock(ctx, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.locks[LeaderElectOffset+3] {
		t.Error("expected node lock to be released")
	}
}

func TestCoordinator_LockConstants(t *testing.T) {
	// Verify lock constants don't overlap with leader election range.
	taskLocks := []int64{LockExpiry, LockBilling, LockNodeMonitor, LockTraffic, LockCertRotate}
	for _, id := range taskLocks {
		if id >= LeaderElectOffset {
			t.Errorf("task lock ID %d overlaps with leader election range (>= %d)", id, LeaderElectOffset)
		}
	}

	// Verify all task lock IDs are unique.
	seen := make(map[int64]bool)
	for _, id := range taskLocks {
		if seen[id] {
			t.Errorf("duplicate task lock ID: %d", id)
		}
		seen[id] = true
	}
}

func TestGenerateWorkerID(t *testing.T) {
	id := generateWorkerID()
	if id == "" {
		t.Error("generateWorkerID should not return empty string")
	}

	hostname, _ := os.Hostname()
	expected := fmt.Sprintf("%s-%d", hostname, os.Getpid())
	if id != expected {
		t.Errorf("expected %q, got %q", expected, id)
	}
}
