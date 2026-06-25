package grpcclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
)

// NodeStatus represents the connection state of a node.
type NodeStatus string

const (
	StatusOnline  NodeStatus = "online"
	StatusOffline NodeStatus = "offline"
	StatusStale   NodeStatus = "stale"
)

// NodeConnection represents a persistent connection to a single knode instance.
type NodeConnection struct {
	NodeID      int64
	NodeName    string
	Address     string
	Port        int
	Status      NodeStatus
	LastMetrics time.Time
	Conn        *grpc.ClientConn
}

// NodeConfig holds the credentials needed to connect to a knode.
type NodeConfig struct {
	NodeID     int64
	Name       string
	Address    string
	Port       int
	APIKey     string          // decrypted
	ClientCert tls.Certificate // parsed client TLS certificate + key
	CACert     []byte          // PEM-encoded CA certificate
}

// StatusChangeFunc is a callback invoked when a node's status changes.
type StatusChangeFunc func(nodeID int64, old, new NodeStatus)

// Pool manages all gRPC connections to knode instances.
type Pool interface {
	// Lifecycle
	Start(ctx context.Context) error
	Stop() error

	// Connection management
	Connect(ctx context.Context, node NodeConfig) error
	Disconnect(nodeID int64) error
	Reconnect(nodeID int64) error

	// Access
	Get(nodeID int64) (*NodeConnection, error)
	All() []*NodeConnection
	Status(nodeID int64) NodeStatus

	// Events
	OnStatusChange(fn StatusChangeFunc)
}

// PoolConfig holds configuration for the connection pool.
type PoolConfig struct {
	ConnectTimeout    time.Duration
	KeepaliveInterval time.Duration
	Backoff           BackoffPolicy
}

// DefaultPoolConfig returns pool configuration with sensible defaults.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		ConnectTimeout:    10 * time.Second,
		KeepaliveInterval: 30 * time.Second,
		Backoff:           DefaultBackoff(),
	}
}

// connPool is the concrete implementation of Pool.
type connPool struct {
	mu          sync.RWMutex
	connections map[int64]*nodeEntry
	callbacks   []StatusChangeFunc
	config      PoolConfig
	ctx         context.Context
	cancel      context.CancelFunc
	stopped     bool
}

// nodeEntry wraps a NodeConnection with reconnection state.
type nodeEntry struct {
	conn       *NodeConnection
	nodeConfig NodeConfig
	cancelReco context.CancelFunc // cancel the reconnection goroutine
}

// NewPool creates a new connection pool with the given configuration.
func NewPool(cfg PoolConfig) Pool {
	return &connPool{
		connections: make(map[int64]*nodeEntry),
		config:      cfg,
	}
}

// Start initializes the pool. The context is used as the pool's lifetime context.
func (p *connPool) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.stopped = false
	log.Printf("[grpc-client] Connection pool started")
	return nil
}

// Stop closes all connections and shuts down the pool.
func (p *connPool) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped {
		return nil
	}
	p.stopped = true

	// Cancel the pool context (stops all reconnection goroutines)
	if p.cancel != nil {
		p.cancel()
	}

	// Close all connections
	var firstErr error
	for id, entry := range p.connections {
		if entry.cancelReco != nil {
			entry.cancelReco()
		}
		if entry.conn.Conn != nil {
			if err := entry.conn.Conn.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		log.Printf("[grpc-client] Closed connection to node %q (id=%d)", entry.conn.NodeName, id)
	}

	p.connections = make(map[int64]*nodeEntry)
	log.Printf("[grpc-client] Connection pool stopped")
	return firstErr
}

// Connect establishes a gRPC connection to the specified node.
func (p *connPool) Connect(ctx context.Context, node NodeConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped {
		return errors.New("grpcclient: pool is stopped")
	}

	// If already connected, disconnect first
	if existing, ok := p.connections[node.NodeID]; ok {
		if existing.cancelReco != nil {
			existing.cancelReco()
		}
		if existing.conn.Conn != nil {
			existing.conn.Conn.Close()
		}
	}

	conn, err := p.dial(ctx, node)
	if err != nil {
		// Store as offline but save config for reconnection
		entry := &nodeEntry{
			conn: &NodeConnection{
				NodeID:   node.NodeID,
				NodeName: node.Name,
				Address:  node.Address,
				Port:     node.Port,
				Status:   StatusOffline,
			},
			nodeConfig: node,
		}
		p.connections[node.NodeID] = entry
		log.Printf("[grpc-client] Failed to connect to node %q at %s:%d: %v", node.Name, node.Address, node.Port, err)

		// Spawn reconnection goroutine
		p.startReconnect(entry)
		return fmt.Errorf("grpcclient: dial %s:%d: %w", node.Address, node.Port, err)
	}

	entry := &nodeEntry{
		conn: &NodeConnection{
			NodeID:      node.NodeID,
			NodeName:    node.Name,
			Address:     node.Address,
			Port:        node.Port,
			Status:      StatusOnline,
			LastMetrics: time.Now(),
			Conn:        conn,
		},
		nodeConfig: node,
	}
	p.connections[node.NodeID] = entry
	log.Printf("[grpc-client] Connected to node %q at %s:%d", node.Name, node.Address, node.Port)
	return nil
}

// Disconnect closes the connection to the specified node and removes it from the pool.
func (p *connPool) Disconnect(nodeID int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, ok := p.connections[nodeID]
	if !ok {
		return fmt.Errorf("grpcclient: node %d not found", nodeID)
	}

	if entry.cancelReco != nil {
		entry.cancelReco()
	}
	if entry.conn.Conn != nil {
		entry.conn.Conn.Close()
	}

	delete(p.connections, nodeID)
	log.Printf("[grpc-client] Disconnected node %q (id=%d)", entry.conn.NodeName, nodeID)
	return nil
}

// Reconnect closes the existing connection and re-dials a node.
func (p *connPool) Reconnect(nodeID int64) error {
	p.mu.Lock()
	entry, ok := p.connections[nodeID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("grpcclient: node %d not found", nodeID)
	}

	// Cancel any existing reconnection goroutine
	if entry.cancelReco != nil {
		entry.cancelReco()
	}

	// Close existing connection
	if entry.conn.Conn != nil {
		entry.conn.Conn.Close()
		entry.conn.Conn = nil
	}

	oldStatus := entry.conn.Status
	entry.conn.Status = StatusOffline
	cfg := entry.nodeConfig
	p.mu.Unlock()

	// Emit status change if needed
	if oldStatus != StatusOffline {
		p.emitStatusChange(nodeID, oldStatus, StatusOffline)
	}

	// Attempt reconnection
	ctx := p.poolCtx()
	if ctx == nil {
		return errors.New("grpcclient: pool is stopped")
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, p.config.ConnectTimeout)
	defer dialCancel()

	conn, err := p.dial(dialCtx, cfg)
	if err != nil {
		log.Printf("[grpc-client] Reconnect failed for node %q (id=%d): %v", cfg.Name, nodeID, err)

		// Spawn background reconnection
		p.mu.Lock()
		if e, ok := p.connections[nodeID]; ok {
			p.startReconnect(e)
		}
		p.mu.Unlock()
		return fmt.Errorf("grpcclient: reconnect %s:%d: %w", cfg.Address, cfg.Port, err)
	}

	p.mu.Lock()
	if e, ok := p.connections[nodeID]; ok {
		e.conn.Conn = conn
		e.conn.Status = StatusOnline
		e.conn.LastMetrics = time.Now()
	}
	p.mu.Unlock()

	p.emitStatusChange(nodeID, StatusOffline, StatusOnline)
	log.Printf("[grpc-client] Reconnected to node %q (id=%d)", cfg.Name, nodeID)
	return nil
}

// Get returns the NodeConnection for the given node ID.
func (p *connPool) Get(nodeID int64) (*NodeConnection, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	entry, ok := p.connections[nodeID]
	if !ok {
		return nil, fmt.Errorf("grpcclient: node %d not found", nodeID)
	}
	return entry.conn, nil
}

// All returns all node connections in the pool.
func (p *connPool) All() []*NodeConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*NodeConnection, 0, len(p.connections))
	for _, entry := range p.connections {
		result = append(result, entry.conn)
	}
	return result
}

// Status returns the current status of a node. Returns StatusOffline if the node is not found.
func (p *connPool) Status(nodeID int64) NodeStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	entry, ok := p.connections[nodeID]
	if !ok {
		return StatusOffline
	}
	return entry.conn.Status
}

// OnStatusChange registers a callback invoked when any node's status changes.
func (p *connPool) OnStatusChange(fn StatusChangeFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.callbacks = append(p.callbacks, fn)
}

// dial creates a gRPC connection using mTLS and API key metadata.
func (p *connPool) dial(ctx context.Context, cfg NodeConfig) (*grpc.ClientConn, error) {
	// Build TLS config with client certificate and CA
	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build TLS config: %w", err)
	}

	target := fmt.Sprintf("%s:%d", cfg.Address, cfg.Port)

	// Create per-RPC credentials that inject the API key as metadata
	creds := &apiKeyCredentials{apiKey: cfg.APIKey}

	dialCtx, cancel := context.WithTimeout(ctx, p.config.ConnectTimeout)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx,
		target,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
		grpc.WithPerRPCCredentials(creds),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// buildTLSConfig creates a *tls.Config for mTLS to a knode instance.
func buildTLSConfig(cfg NodeConfig) (*tls.Config, error) {
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(cfg.CACert) {
		return nil, errors.New("failed to parse CA certificate PEM")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cfg.ClientCert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// apiKeyCredentials implements grpc.PerRPCCredentials to inject the API key as metadata.
type apiKeyCredentials struct {
	apiKey string
}

func (c *apiKeyCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"x-api-key": c.apiKey,
	}, nil
}

func (c *apiKeyCredentials) RequireTransportSecurity() bool {
	return true // always require TLS
}

// startReconnect spawns a background goroutine that attempts reconnection with exponential backoff.
// Must be called with p.mu held.
func (p *connPool) startReconnect(entry *nodeEntry) {
	if p.ctx == nil {
		return
	}

	recoCtx, recoCancel := context.WithCancel(p.ctx)
	entry.cancelReco = recoCancel

	cfg := entry.nodeConfig
	nodeID := cfg.NodeID

	go func() {
		attempt := 0
		for {
			delay := p.config.Backoff.Delay(attempt)
			log.Printf("[grpc-client] Reconnecting to node %q (id=%d) in %s (attempt %d)", cfg.Name, nodeID, delay, attempt+1)

			select {
			case <-recoCtx.Done():
				return
			case <-time.After(delay):
			}

			dialCtx, dialCancel := context.WithTimeout(recoCtx, p.config.ConnectTimeout)
			conn, err := p.dial(dialCtx, cfg)
			dialCancel()

			if err != nil {
				attempt++
				log.Printf("[grpc-client] Reconnect attempt %d failed for node %q (id=%d): %v", attempt, cfg.Name, nodeID, err)
				continue
			}

			// Success — update the entry
			p.mu.Lock()
			e, ok := p.connections[nodeID]
			if ok {
				oldStatus := e.conn.Status
				e.conn.Conn = conn
				e.conn.Status = StatusOnline
				e.conn.LastMetrics = time.Now()
				e.cancelReco = nil
				p.mu.Unlock()

				if oldStatus != StatusOnline {
					p.emitStatusChange(nodeID, oldStatus, StatusOnline)
				}
				log.Printf("[grpc-client] Reconnected to node %q (id=%d) after %d attempts", cfg.Name, nodeID, attempt+1)
			} else {
				// Node was removed while reconnecting
				conn.Close()
				p.mu.Unlock()
			}
			return
		}
	}()
}

// emitStatusChange invokes all registered status change callbacks.
func (p *connPool) emitStatusChange(nodeID int64, old, new NodeStatus) {
	p.mu.RLock()
	cbs := make([]StatusChangeFunc, len(p.callbacks))
	copy(cbs, p.callbacks)
	p.mu.RUnlock()

	for _, fn := range cbs {
		fn(nodeID, old, new)
	}
}

// poolCtx returns the pool's context, or nil if the pool is stopped.
func (p *connPool) poolCtx() context.Context {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ctx
}

// SetStatus updates the status of a node in the pool. This is used by the
// status state machine to transition nodes between online/stale/offline.
func (p *connPool) SetStatus(nodeID int64, status NodeStatus) {
	p.mu.Lock()
	entry, ok := p.connections[nodeID]
	if !ok {
		p.mu.Unlock()
		return
	}
	old := entry.conn.Status
	if old == status {
		p.mu.Unlock()
		return
	}
	entry.conn.Status = status
	p.mu.Unlock()

	p.emitStatusChange(nodeID, old, status)

	// If transitioning to offline, start reconnection
	if status == StatusOffline && old != StatusOffline {
		p.mu.Lock()
		if e, ok := p.connections[nodeID]; ok && e.cancelReco == nil {
			// Close the dead connection
			if e.conn.Conn != nil {
				state := e.conn.Conn.GetState()
				if state == connectivity.Shutdown || state == connectivity.TransientFailure {
					e.conn.Conn.Close()
					e.conn.Conn = nil
				}
			}
			p.startReconnect(e)
		}
		p.mu.Unlock()
	}
}

// UpdateLastMetrics records the time of the last received metrics event for a node.
func (p *connPool) UpdateLastMetrics(nodeID int64, t time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, ok := p.connections[nodeID]; ok {
		entry.conn.LastMetrics = t
	}
}
