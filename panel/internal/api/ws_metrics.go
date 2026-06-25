package api

import (
	"log"
	"time"

	"KorisPanel/panel/internal/grpcclient"
)

// BroadcastNodeMetrics sends a node_metrics message to all connected WebSocket clients.
// Called by the MetricsConsumer (or a callback wired to it) when new metrics arrive from a knode.
func (s *Server) BroadcastNodeMetrics(nodeID int64, nodeName string, status string, event grpcclient.MetricsEvent) {
	msg := map[string]any{
		"type": "node_metrics",
		"data": map[string]any{
			"node_id":      nodeID,
			"name":         nodeName,
			"status":       status,
			"cpu_percent":  event.CPUPercent,
			"ram_percent":  event.RAMPercent,
			"disk_percent": event.DiskPercent,
			"rx_bps":       event.RxBPS,
			"tx_bps":       event.TxBPS,
			"sessions":     event.ActiveSessions,
			"uptime":       event.UptimeSeconds,
		},
	}

	s.wsNotifMu.RLock()
	defer s.wsNotifMu.RUnlock()

	for _, ch := range s.wsNotifChans {
		select {
		case ch <- msg:
		default:
			// Channel full, skip this subscriber
		}
	}
}

// BroadcastNodeStatusChange sends a node_status_change message to all connected WebSocket clients.
// Called when a node transitions between online/stale/offline states.
func (s *Server) BroadcastNodeStatusChange(nodeID int64, nodeName string, oldStatus, newStatus string) {
	msg := map[string]any{
		"type": "node_status_change",
		"data": map[string]any{
			"node_id":    nodeID,
			"name":       nodeName,
			"old_status": oldStatus,
			"new_status": newStatus,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		},
	}

	s.wsNotifMu.RLock()
	defer s.wsNotifMu.RUnlock()

	for _, ch := range s.wsNotifChans {
		select {
		case ch <- msg:
		default:
			// Channel full, skip this subscriber
		}
	}
}

// RegisterWSMetricsBroadcast wires the WebSocket metrics broadcasting into the gRPC pool's
// status change callback and provides a hook for the MetricsConsumer to forward metrics.
// Call this after the Server and GRPCPool are both initialized.
func (s *Server) RegisterWSMetricsBroadcast() {
	if s.GRPCPool == nil {
		return
	}

	// Register a status change callback that broadcasts to WebSocket clients
	s.GRPCPool.OnStatusChange(func(nodeID int64, old, new grpcclient.NodeStatus) {
		// Look up node name from the pool
		nodeName := s.nodeNameByID(nodeID)

		s.BroadcastNodeStatusChange(nodeID, nodeName, string(old), string(new))
		log.Printf("[ws] broadcasting node_status_change: node=%d %s→%s", nodeID, old, new)
	})
}

// nodeNameByID looks up a node's display name by its ID.
func (s *Server) nodeNameByID(nodeID int64) string {
	// Try to get from GRPCPool connections first
	if s.GRPCPool != nil {
		conn, err := s.GRPCPool.Get(nodeID)
		if err == nil && conn != nil {
			return conn.NodeName
		}
	}

	// Fallback: query database
	var name string
	_ = s.DB.QueryRow(`SELECT COALESCE(name, '') FROM nodes WHERE id = $1`, nodeID).Scan(&name)
	return name
}
