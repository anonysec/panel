package worker

import (
	"runtime"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.NumWorkers != 0 {
		t.Errorf("expected NumWorkers=0 (auto), got %d", cfg.NumWorkers)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("expected Addr=:8080, got %q", cfg.Addr)
	}
	if cfg.GracefulWait != 30*time.Second {
		t.Errorf("expected GracefulWait=30s, got %v", cfg.GracefulWait)
	}
	if cfg.MaxRestarts != 5 {
		t.Errorf("expected MaxRestarts=5, got %d", cfg.MaxRestarts)
	}
}

func TestConfig_ResolvedWorkers_ExplicitCount(t *testing.T) {
	tests := []struct {
		name       string
		numWorkers int
		expected   int
	}{
		{"explicit 1", 1, 1},
		{"explicit 2", 2, 2},
		{"explicit 4", 4, 4},
		{"explicit 8", 8, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{NumWorkers: tt.numWorkers}
			got := cfg.ResolvedWorkers()
			if got != tt.expected {
				t.Errorf("ResolvedWorkers()=%d, want %d", got, tt.expected)
			}
		})
	}
}

func TestConfig_ResolvedWorkers_AutoDetect(t *testing.T) {
	cfg := Config{NumWorkers: 0}
	got := cfg.ResolvedWorkers()

	cpus := runtime.NumCPU()
	expected := cpus
	if expected > 4 {
		expected = 4
	}
	if expected < 1 {
		expected = 1
	}

	if got != expected {
		t.Errorf("ResolvedWorkers() auto-detect=%d, expected min(NumCPU=%d, 4)=%d", got, cpus, expected)
	}
}

func TestConfig_ResolvedWorkers_CappedAt4(t *testing.T) {
	cfg := Config{NumWorkers: 0}
	got := cfg.ResolvedWorkers()

	if got > 4 {
		t.Errorf("ResolvedWorkers() auto-detect should be capped at 4, got %d", got)
	}
	if got < 1 {
		t.Errorf("ResolvedWorkers() auto-detect should be at least 1, got %d", got)
	}
}

func TestWorkerStatus_String(t *testing.T) {
	tests := []struct {
		status   WorkerStatus
		expected string
	}{
		{StatusStarting, "starting"},
		{StatusRunning, "running"},
		{StatusStopping, "stopping"},
		{StatusDead, "dead"},
		{WorkerStatus(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.status.String()
			if got != tt.expected {
				t.Errorf("WorkerStatus(%d).String()=%q, want %q", tt.status, got, tt.expected)
			}
		})
	}
}

func TestNewManager_ExplicitWorkerCount(t *testing.T) {
	cfg := Config{
		NumWorkers:   3,
		Addr:         ":9090",
		GracefulWait: 10 * time.Second,
		MaxRestarts:  2,
	}
	mgr := NewManager(cfg)

	if mgr.numWorkers != 3 {
		t.Errorf("expected numWorkers=3, got %d", mgr.numWorkers)
	}
	if mgr.addr != ":9090" {
		t.Errorf("expected addr=:9090, got %q", mgr.addr)
	}
	if mgr.cfg.MaxRestarts != 2 {
		t.Errorf("expected cfg.MaxRestarts=2, got %d", mgr.cfg.MaxRestarts)
	}
}

func TestNewManager_AutoWorkerCount(t *testing.T) {
	cfg := Config{
		NumWorkers: 0,
		Addr:       ":8080",
	}
	mgr := NewManager(cfg)

	expected := cfg.ResolvedWorkers()
	if mgr.numWorkers != expected {
		t.Errorf("expected numWorkers=%d (auto), got %d", expected, mgr.numWorkers)
	}
}

func TestNewManager_WorkersSlicePreAllocated(t *testing.T) {
	cfg := Config{
		NumWorkers: 4,
		Addr:       ":8080",
	}
	mgr := NewManager(cfg)

	if len(mgr.workers) != 0 {
		t.Errorf("expected workers slice length=0 initially, got %d", len(mgr.workers))
	}
	if cap(mgr.workers) != 4 {
		t.Errorf("expected workers slice capacity=4, got %d", cap(mgr.workers))
	}
}

func TestManager_Status_Empty(t *testing.T) {
	mgr := NewManager(Config{NumWorkers: 2, Addr: ":8080"})
	status := mgr.Status()

	if len(status) != 0 {
		t.Errorf("expected 0 status entries before Start, got %d", len(status))
	}
}

func TestManager_Status_WithWorkers(t *testing.T) {
	mgr := &Manager{
		numWorkers: 2,
		addr:       ":8080",
		workers:    make([]*WorkerProcess, 0, 2),
		cfg:        Config{NumWorkers: 2},
	}

	// Simulate workers being added (as Start would do).
	now := time.Now()
	mgr.workers = append(mgr.workers, &WorkerProcess{
		ID:      0,
		PID:     1234,
		Status:  StatusRunning,
		StartAt: now,
	})
	mgr.workers = append(mgr.workers, &WorkerProcess{
		ID:      1,
		PID:     1235,
		Status:  StatusRunning,
		StartAt: now,
	})

	status := mgr.Status()
	if len(status) != 2 {
		t.Fatalf("expected 2 status entries, got %d", len(status))
	}
	if status[0].ID != 0 || status[0].PID != 1234 {
		t.Errorf("worker 0: got ID=%d PID=%d, want ID=0 PID=1234", status[0].ID, status[0].PID)
	}
	if status[1].ID != 1 || status[1].PID != 1235 {
		t.Errorf("worker 1: got ID=%d PID=%d, want ID=1 PID=1235", status[1].ID, status[1].PID)
	}
}

func TestManager_Status_ReturnsSnapshot(t *testing.T) {
	mgr := &Manager{
		numWorkers: 1,
		workers:    make([]*WorkerProcess, 0, 1),
	}
	mgr.workers = append(mgr.workers, &WorkerProcess{
		ID:     0,
		PID:    9999,
		Status: StatusRunning,
	})

	snapshot := mgr.Status()

	// Mutating the snapshot should not affect the manager's internal state.
	snapshot[0].Status = StatusDead
	if mgr.workers[0].Status != StatusRunning {
		t.Error("mutating Status() snapshot should not affect internal workers")
	}
}

func TestLeaderLock_Contention_TwoLocks(t *testing.T) {
	// On non-Linux (Windows/macOS), both locks always succeed (stub).
	// On Linux, one should succeed and the other should fail.
	// This test validates the behavior on the current platform.
	dir := t.TempDir()
	path := dir + "/leader.lock"

	lock1 := NewLeaderLock(path)
	lock2 := NewLeaderLock(path)
	defer lock1.Release()
	defer lock2.Release()

	result1 := lock1.TryAcquire()
	result2 := lock2.TryAcquire()

	if runtime.GOOS == "linux" {
		// On Linux, only one should succeed.
		if result1 && result2 {
			t.Error("on Linux, both locks should not acquire simultaneously (flock exclusion)")
		}
		if !result1 && !result2 {
			t.Error("on Linux, at least one lock should acquire")
		}
	} else {
		// On non-Linux (stub), both succeed.
		if !result1 {
			t.Error("expected first lock to succeed")
		}
		if !result2 {
			t.Error("expected second lock to succeed (stub on non-Linux)")
		}
	}
}

func TestLeaderLock_Contention_AcquireAfterRelease(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/leader.lock"

	lock1 := NewLeaderLock(path)
	lock2 := NewLeaderLock(path)
	defer lock2.Release()

	if !lock1.TryAcquire() {
		t.Fatal("first lock should acquire")
	}

	// Release the first lock.
	lock1.Release()

	// Now the second lock should be able to acquire (on all platforms).
	if !lock2.TryAcquire() {
		t.Error("second lock should acquire after first is released")
	}
	if !lock2.IsLeader() {
		t.Error("lock2 should be leader after acquiring")
	}
}
