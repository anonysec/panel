package worker

import (
	"context"
	"sync"
)

// Manager orchestrates multiple worker processes sharing the same port
// via SO_REUSEPORT for kernel-level load balancing.
type Manager struct {
	numWorkers int
	addr       string
	workers    []*WorkerProcess
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	cfg        Config
}

// NewManager creates a Manager with the given configuration.
// It resolves the effective worker count from cfg.NumWorkers (auto if 0).
func NewManager(cfg Config) *Manager {
	return &Manager{
		numWorkers: cfg.ResolvedWorkers(),
		addr:       cfg.Addr,
		workers:    make([]*WorkerProcess, 0, cfg.ResolvedWorkers()),
		cfg:        cfg,
	}
}

// Start spawns all worker processes and begins monitoring them.
// The provided context controls the lifetime of the manager; cancelling it
// triggers a graceful shutdown of all workers.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Fork all worker processes.
	for i := 0; i < m.numWorkers; i++ {
		wp, err := m.forkWorker(i)
		if err != nil {
			return err
		}
		wp.Status = StatusRunning

		m.mu.Lock()
		m.workers = append(m.workers, wp)
		m.mu.Unlock()

		// Start a goroutine that waits for this worker to exit.
		go m.waitForExit(wp)
	}

	// Start the monitor loop in a background goroutine.
	go m.monitorLoop(m.ctx)

	// Block until the context is cancelled.
	<-m.ctx.Done()
	return nil
}

// Stop gracefully shuts down all worker processes.
// It cancels the manager context (stopping the monitor loop), sends an
// interrupt signal to each worker, and waits up to GracefulWait before
// forcefully killing remaining processes.
func (m *Manager) Stop() error {
	// Cancel context first to stop the monitor loop from restarting workers.
	if m.cancel != nil {
		m.cancel()
	}

	m.gracefulShutdown()
	return nil
}

// Reload performs a graceful restart of all workers. New workers are spawned
// and confirmed healthy before old workers are terminated, achieving
// zero-downtime restarts. Implementation is in reload.go.

// Status returns a snapshot of all worker processes and their current state.
func (m *Manager) Status() []WorkerProcess {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]WorkerProcess, len(m.workers))
	for i, w := range m.workers {
		if w != nil {
			result[i] = *w
		}
	}
	return result
}
