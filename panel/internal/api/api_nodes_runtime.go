package api

import (
	"KorisPanel/panel/internal/grpcclient"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

func (s *Server) scanNode(row nodeScanner) (Node, error) {
	var n Node
	var lastSeen, created sql.NullTime
	var proxyConfig []byte
	if err := row.Scan(&n.ID, &n.Name, &n.PublicIP, &n.Domain, &n.Status, &lastSeen, &created, &proxyConfig); err != nil {
		return n, err
	}
	if lastSeen.Valid {
		n.LastSeenAt = lastSeen.Time.Format(time.RFC3339)
	}
	if created.Valid {
		n.CreatedAt = created.Time.Format(time.RFC3339)
	}
	if len(proxyConfig) > 0 {
		n.ProxyConfig = json.RawMessage(proxyConfig)
	}
	return n, nil
}

func (s *Server) fillNodeRuntime(n *Node) error {
	// Populate bandwidth quota fields from nodes table
	var quotaGB sql.NullInt64
	var usedBytes int64
	var resetAt sql.NullTime
	if err := s.DB.QueryRow(`SELECT bandwidth_quota_gb, bandwidth_used_bytes, bandwidth_reset_at FROM nodes WHERE id=$1`, n.ID).Scan(&quotaGB, &usedBytes, &resetAt); err == nil {
		if quotaGB.Valid {
			n.BandwidthQuotaGB = &quotaGB.Int64
		}
		n.BandwidthUsedBytes = usedBytes
		if resetAt.Valid {
			n.BandwidthResetAt = resetAt.Time.UTC().Format(time.RFC3339)
		}
	}

	var updated sql.NullTime
	_ = s.DB.QueryRow(`SELECT cpu_percent,ram_percent,disk_percent,rx_bps,tx_bps,openvpn_status,l2tp_status,ikev2_status,updated_at FROM node_status WHERE node_id=$1`, n.ID).Scan(&n.StatusMetrics.CPUPercent, &n.StatusMetrics.RAMPercent, &n.StatusMetrics.DiskPercent, &n.StatusMetrics.RxBps, &n.StatusMetrics.TxBps, &n.StatusMetrics.OpenVPN, &n.StatusMetrics.L2TP, &n.StatusMetrics.IKEv2, &updated)
	if updated.Valid {
		n.StatusMetrics.UpdatedAt = updated.Time.Format(time.RFC3339)
	}
	rows, err := s.DB.Query(`SELECT service,status,updated_at FROM node_services WHERE node_id=$1 ORDER BY service`, n.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var svc Service
			var t sql.NullTime
			if err := rows.Scan(&svc.Service, &svc.Status, &t); err == nil {
				if t.Valid {
					svc.UpdatedAt = t.Time.Format(time.RFC3339)
				}
				n.Services = append(n.Services, svc)
				// Populate SSH status from services
				if svc.Service == "ssh" && svc.Status != "" {
					n.StatusMetrics.SSH = svc.Status
				}
			}
		}
	}

	hRows, err := s.DB.Query(`SELECT id, node_id, rx_bytes, tx_bytes, online_users, created_at FROM node_usage_snapshots WHERE node_id=$1 ORDER BY id DESC LIMIT 15`, n.ID)
	if err == nil {
		defer hRows.Close()
		for hRows.Next() {
			var snap NodeUsageSnapshot
			var t time.Time
			if err := hRows.Scan(&snap.ID, &snap.NodeID, &snap.RxBytes, &snap.TxBytes, &snap.OnlineUsers, &t); err == nil {
				snap.CreatedAt = t.Format(time.RFC3339)
				n.History = append(n.History, snap)
			}
		}
	}

	var diag DiagnosticsReport
	err = s.DB.QueryRow(`SELECT agent_version, uptime_seconds, go_version, goroutines, mem_alloc_bytes FROM node_diagnostics WHERE node_id=$1`, n.ID).Scan(
		&diag.AgentVersion, &diag.UptimeSeconds, &diag.GoVersion, &diag.Goroutines, &diag.MemAllocBytes)
	if err == nil {
		n.Diagnostics = &diag
	}

	// Populate health score from gRPC pool status
	if s.GRPCPool != nil {
		switch s.GRPCPool.Status(n.ID) {
		case grpcclient.StatusOnline:
			n.HealthScore = 1.0
		case grpcclient.StatusStale:
			n.HealthScore = 0.5
		default:
			n.HealthScore = 0.0
		}
	}

	return nil
}

func (s *Server) markStaleNodes() {
	_, _ = s.DB.Exec(`UPDATE nodes SET status='stale' WHERE status='online' AND last_seen_at < (NOW() - INTERVAL '2 minutes')`)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func nullString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.TrimSpace(v)
}
