package grpcclient

import (
	"context"
	"database/sql"
	"io"
	"log"
	"time"

	"KorisPanel/panel/internal/dbstore"
	"KorisPanel/panel/internal/knodepb"
)

// MetricsEvent represents a streaming metrics payload from a knode instance.
// This is a local type since the panel doesn't yet have generated protobuf code.
type MetricsEvent struct {
	CPUPercent     float64
	RAMPercent     float64
	DiskPercent    float64
	RxBPS          int64
	TxBPS          int64
	ActiveSessions int
	UptimeSeconds  int64
	Cores          []CoreStatus
}

// CoreStatus represents the state of a single VPN core on a knode.
type CoreStatus struct {
	Type           string // e.g., "openvpn", "wireguard", "l2tp", "ikev2", "ssh", "mtproto"
	State          string // "running", "stopped", "error", etc.
	ActiveSessions int
	PID            int
}

// MetricsConsumer processes incoming MetricsEvent data from knode streams.
// It writes time-series history via dbstore.InsertMetrics, updates the node_status
// table with live values, and updates per-core info in node_services.
type MetricsConsumer struct {
	store dbstore.Store
	pool  *connPool
}

// NewMetricsConsumer creates a MetricsConsumer with the given database store and connection pool.
func NewMetricsConsumer(store dbstore.Store, pool *connPool) *MetricsConsumer {
	return &MetricsConsumer{
		store: store,
		pool:  pool,
	}
}

// MinMetricsInterval is the minimum allowed StreamMetrics interval (5 seconds).
// Any configured interval below this floor is clamped to 5s.
const MinMetricsInterval = 5 * time.Second

// DefaultMetricsInterval is the default StreamMetrics reporting interval (10 seconds).
const DefaultMetricsInterval = 10 * time.Second

// StartStream opens a StreamMetrics subscription to the specified node via the
// generated knodepb proto client. It requests metrics at the configured interval
// (default 10s, minimum 5s), reads events from the server stream, and passes
// each to ProcessEvent.
// On stream error or context cancellation, it logs and triggers reconnection via the pool.
func (mc *MetricsConsumer) StartStream(ctx context.Context, nodeID int64) {
	mc.StartStreamWithInterval(ctx, nodeID, DefaultMetricsInterval)
}

// StartStreamWithInterval opens a StreamMetrics subscription with a custom interval.
// The interval is clamped to a minimum of 5 seconds per requirement 10.5.
func (mc *MetricsConsumer) StartStreamWithInterval(ctx context.Context, nodeID int64, interval time.Duration) {
	// Enforce minimum interval of 5 seconds
	if interval < MinMetricsInterval {
		interval = MinMetricsInterval
	}
	mc.pool.mu.RLock()
	entry, ok := mc.pool.connections[nodeID]
	mc.pool.mu.RUnlock()
	if !ok {
		log.Printf("[grpc-client] StartStream: node %d not found in pool", nodeID)
		return
	}

	conn := entry.conn
	if conn.Conn == nil {
		log.Printf("[grpc-client] StartStream: node %d has no active gRPC connection", nodeID)
		return
	}

	client := knodepb.NewKnodeServiceClient(conn.Conn)

	intervalSec := int32(interval.Seconds())
	stream, err := client.StreamMetrics(ctx, &knodepb.StreamMetricsRequest{
		IntervalSeconds: intervalSec,
	})
	if err != nil {
		log.Printf("[grpc-client] StartStream: failed to open StreamMetrics for node %d: %v", nodeID, err)
		return
	}

	log.Printf("[grpc-client] StreamMetrics opened for node %d (interval: %s)", nodeID, interval)

	go func() {
		for {
			pbEvent, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					log.Printf("[grpc-client] StreamMetrics EOF for node %d — stream ended by server", nodeID)
				} else if ctx.Err() != nil {
					log.Printf("[grpc-client] StreamMetrics cancelled for node %d", nodeID)
					return
				} else {
					log.Printf("[grpc-client] StreamMetrics error for node %d: %v", nodeID, err)
				}
				// Trigger reconnection via the pool's status transition
				mc.pool.SetStatus(nodeID, StatusOffline)
				return
			}

			// Convert proto MetricsEvent to local MetricsEvent
			event := metricsEventFromProto(pbEvent)
			if processErr := mc.ProcessEvent(nodeID, event); processErr != nil {
				log.Printf("[grpc-client] ProcessEvent error for node %d: %v", nodeID, processErr)
			}
		}
	}()
}

// metricsEventFromProto converts a knodepb.MetricsEvent to the local MetricsEvent type.
func metricsEventFromProto(pb *knodepb.MetricsEvent) MetricsEvent {
	event := MetricsEvent{
		CPUPercent:     pb.GetCpuPercent(),
		RAMPercent:     pb.GetRamPercent(),
		DiskPercent:    pb.GetDiskPercent(),
		RxBPS:          pb.GetRxBps(),
		TxBPS:          pb.GetTxBps(),
		ActiveSessions: int(pb.GetActiveSessions()),
		UptimeSeconds:  pb.GetUptimeSeconds(),
	}

	for _, core := range pb.GetPerCore() {
		event.Cores = append(event.Cores, CoreStatus{
			Type:           core.GetType(),
			State:          coreStateToString(core.GetState()),
			ActiveSessions: int(core.GetActiveSessions()),
			PID:            int(core.GetPid()),
		})
	}

	return event
}

// coreStateToString maps the proto CoreState enum to a string status for the local CoreStatus.
func coreStateToString(state knodepb.CoreState) string {
	switch state {
	case knodepb.CoreState_CORE_STATE_RUNNING:
		return "running"
	case knodepb.CoreState_CORE_STATE_STOPPED:
		return "stopped"
	case knodepb.CoreState_CORE_STATE_CRASHED:
		return "crashed"
	default:
		return "unknown"
	}
}

// ProcessEvent handles a single MetricsEvent received from a knode stream.
// It performs three operations:
//  1. Writes the metrics to node_metrics_history via dbstore.InsertMetrics
//  2. Updates node_status table with the latest live values
//  3. Updates node_services table with per-core status
//
// It also updates the pool's LastMetrics timestamp to keep the status monitor accurate.
func (mc *MetricsConsumer) ProcessEvent(nodeID int64, event MetricsEvent) error {
	now := time.Now()

	// 1. Update pool's last metrics timestamp (keeps status monitor happy)
	mc.pool.UpdateLastMetrics(nodeID, now)

	// 2. Write to node_metrics_history via dbstore
	row := &dbstore.MetricsRow{
		Time:           now,
		CPUPercent:     event.CPUPercent,
		RAMPercent:     event.RAMPercent,
		DiskPercent:    event.DiskPercent,
		RxBPS:          event.RxBPS,
		TxBPS:          event.TxBPS,
		ActiveSessions: event.ActiveSessions,
		UptimeSeconds:  event.UptimeSeconds,
	}

	ctx := context.Background()
	if err := mc.store.InsertMetrics(ctx, nodeID, row); err != nil {
		log.Printf("[grpc-client] Failed to insert metrics history for node %d: %v", nodeID, err)
		// Don't return — still update node_status and services
	}

	// 3. Update node_status with live values
	if err := mc.updateNodeStatus(ctx, nodeID, event, now); err != nil {
		log.Printf("[grpc-client] Failed to update node_status for node %d: %v", nodeID, err)
	}

	// 4. Update node_services with per-core status
	if err := mc.updateNodeServices(ctx, nodeID, event.Cores); err != nil {
		log.Printf("[grpc-client] Failed to update node_services for node %d: %v", nodeID, err)
	}

	return nil
}

// updateNodeStatus writes the latest metrics to the node_status table.
// Uses INSERT ... ON CONFLICT for PostgreSQL upsert behavior.
func (mc *MetricsConsumer) updateNodeStatus(ctx context.Context, nodeID int64, event MetricsEvent, now time.Time) error {
	db := mc.store.DB()

	_, err := db.ExecContext(ctx, `
		INSERT INTO node_status (node_id, cpu_percent, ram_percent, disk_percent, rx_bps, tx_bps, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (node_id) DO UPDATE SET
			cpu_percent = EXCLUDED.cpu_percent,
			ram_percent = EXCLUDED.ram_percent,
			disk_percent = EXCLUDED.disk_percent,
			rx_bps = EXCLUDED.rx_bps,
			tx_bps = EXCLUDED.tx_bps,
			updated_at = EXCLUDED.updated_at`,
		nodeID,
		event.CPUPercent,
		event.RAMPercent,
		event.DiskPercent,
		event.RxBPS,
		event.TxBPS,
		now,
	)
	if err != nil {
		return err
	}

	// Also update the gRPC-specific columns added by migration 070
	_, err = db.ExecContext(ctx, `
		UPDATE node_status
		SET last_metrics_at = $1, metrics_state = 'streaming', grpc_connected = TRUE
		WHERE node_id = $2`,
		now, nodeID,
	)
	return err
}

// updateNodeServices updates the node_services table with per-core status info.
// Each core reported in the MetricsEvent gets an upsert row.
func (mc *MetricsConsumer) updateNodeServices(ctx context.Context, nodeID int64, cores []CoreStatus) error {
	if len(cores) == 0 {
		return nil
	}

	db := mc.store.DB()

	for _, core := range cores {
		_, err := db.ExecContext(ctx, `
			INSERT INTO node_services (node_id, service, status, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (node_id, service) DO UPDATE SET status = EXCLUDED.status, updated_at = NOW()`,
			nodeID,
			core.Type,
			core.State,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// NodeStatusFromEvent extracts a summary suitable for logging from a MetricsEvent.
func NodeStatusFromEvent(event MetricsEvent) map[string]any {
	return map[string]any{
		"cpu":             event.CPUPercent,
		"ram":             event.RAMPercent,
		"disk":            event.DiskPercent,
		"rx_bps":          event.RxBPS,
		"tx_bps":          event.TxBPS,
		"active_sessions": event.ActiveSessions,
		"uptime_seconds":  event.UptimeSeconds,
		"cores":           len(event.Cores),
	}
}

// metricsRowFromEvent is a helper that converts a MetricsEvent into a dbstore.MetricsRow.
func metricsRowFromEvent(event MetricsEvent) *dbstore.MetricsRow {
	return &dbstore.MetricsRow{
		Time:           time.Now(),
		CPUPercent:     event.CPUPercent,
		RAMPercent:     event.RAMPercent,
		DiskPercent:    event.DiskPercent,
		RxBPS:          event.RxBPS,
		TxBPS:          event.TxBPS,
		ActiveSessions: event.ActiveSessions,
		UptimeSeconds:  event.UptimeSeconds,
	}
}

// NewMetricsConsumerFromPool creates a MetricsConsumer using the Pool interface.
// This is a convenience constructor for when you have the pool as an interface.
// It requires the underlying pool to be a *connPool (which it always is in practice).
func NewMetricsConsumerFromPool(store dbstore.Store, pool Pool) *MetricsConsumer {
	cp, ok := pool.(*connPool)
	if !ok {
		log.Printf("[grpc-client] WARNING: MetricsConsumer requires *connPool, got %T — UpdateLastMetrics will not work", pool)
		return &MetricsConsumer{store: store}
	}
	return &MetricsConsumer{
		store: store,
		pool:  cp,
	}
}

// markNodeMetricsState updates the metrics_state column in node_status.
// Called by the status monitor when a node transitions to stale or offline.
func markNodeMetricsState(db *sql.DB, nodeID int64, state string) {
	_, err := db.Exec(`UPDATE node_status SET metrics_state = $1 WHERE node_id = $2`, state, nodeID)
	if err != nil {
		log.Printf("[grpc-client] Failed to update metrics_state for node %d: %v", nodeID, err)
	}
}
