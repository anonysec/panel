//go:build !lite

package api

import (
	"net/http"
	"strconv"
	"time"
)

// statisticsGet serves time-series statistics for the admin dashboard.
// GET /api/admin/statistics?metric=bandwidth&period=day&from=2024-01-01&to=2024-01-31&nodeId=0
func (s *Server) statisticsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	metric := q.Get("metric")
	period := q.Get("period")
	fromStr := q.Get("from")
	toStr := q.Get("to")
	nodeIDStr := q.Get("nodeId")

	if metric == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_metric"})
		return
	}

	// Parse date range (default: last 30 days)
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -30)
	to := now

	if fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			to = t.Add(24 * time.Hour) // Include the full end day
		}
	}

	nodeID, _ := strconv.ParseInt(nodeIDStr, 10, 64)
	_ = period // Used for grouping in future refinement

	switch metric {
	case "bandwidth":
		s.statisticsBandwidth(w, from, to, nodeID)
	case "user_growth":
		s.statisticsUserGrowth(w, from, to)
	case "revenue":
		s.statisticsRevenue(w, from, to)
	case "protocol_usage":
		s.statisticsProtocol(w, from, to, nodeID)
	case "node_performance":
		s.statisticsNodePerformance(w, from, to)
	default:
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_metric"})
	}
}

func (s *Server) statisticsBandwidth(w http.ResponseWriter, from, to time.Time, nodeID int64) {
	query := `SELECT hour_start, SUM(rx_bytes), SUM(tx_bytes), MAX(peak_rx_bps), MAX(peak_tx_bps)
		FROM bandwidth_hourly WHERE hour_start >= ? AND hour_start < ?`
	args := []any{from, to}
	if nodeID > 0 {
		query += ` AND node_id = ?`
		args = append(args, nodeID)
	}
	query += ` GROUP BY hour_start ORDER BY hour_start ASC`

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type point struct {
		Time   string `json:"time"`
		RxMB   int64  `json:"rx_mb"`
		TxMB   int64  `json:"tx_mb"`
		PeakRx int64  `json:"peak_rx_bps"`
		PeakTx int64  `json:"peak_tx_bps"`
	}

	var series []point
	var totalRx, totalTx, peakRx, peakTx int64
	for rows.Next() {
		var p point
		var rx, tx, prx, ptx int64
		var t time.Time
		if err := rows.Scan(&t, &rx, &tx, &prx, &ptx); err != nil {
			continue
		}
		p.Time = t.Format(time.RFC3339)
		p.RxMB = rx / (1024 * 1024)
		p.TxMB = tx / (1024 * 1024)
		p.PeakRx = prx
		p.PeakTx = ptx
		series = append(series, p)
		totalRx += rx
		totalTx += tx
		if prx > peakRx {
			peakRx = prx
		}
		if ptx > peakTx {
			peakTx = ptx
		}
	}

	writeJSON(w, map[string]any{
		"ok":     true,
		"metric": "bandwidth",
		"series": series,
		"summary": map[string]any{
			"total_rx_gb": totalRx / (1024 * 1024 * 1024),
			"total_tx_gb": totalTx / (1024 * 1024 * 1024),
			"peak_rx_bps": peakRx,
			"peak_tx_bps": peakTx,
		},
	})
}

func (s *Server) statisticsUserGrowth(w http.ResponseWriter, from, to time.Time) {
	query := `SELECT day_date, new_customers, churned_customers, active_customers
		FROM revenue_daily WHERE day_date >= ? AND day_date < ? ORDER BY day_date ASC`

	rows, err := s.DB.Query(query, from, to)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type point struct {
		Date      string `json:"date"`
		New       int    `json:"new"`
		Churned   int    `json:"churned"`
		Active    int    `json:"active"`
		NetGrowth int    `json:"net_growth"`
	}

	var series []point
	var totalNew, totalChurned int
	for rows.Next() {
		var p point
		var t time.Time
		if err := rows.Scan(&t, &p.New, &p.Churned, &p.Active); err != nil {
			continue
		}
		p.Date = t.Format("2006-01-02")
		p.NetGrowth = p.New - p.Churned
		series = append(series, p)
		totalNew += p.New
		totalChurned += p.Churned
	}

	writeJSON(w, map[string]any{
		"ok":     true,
		"metric": "user_growth",
		"series": series,
		"summary": map[string]any{
			"total_new":     totalNew,
			"total_churned": totalChurned,
			"net_growth":    totalNew - totalChurned,
		},
	})
}

func (s *Server) statisticsRevenue(w http.ResponseWriter, from, to time.Time) {
	query := `SELECT day_date, total_revenue, subscription_revenue, topup_revenue, refund_amount
		FROM revenue_daily WHERE day_date >= ? AND day_date < ? ORDER BY day_date ASC`

	rows, err := s.DB.Query(query, from, to)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type point struct {
		Date         string  `json:"date"`
		Total        float64 `json:"total"`
		Subscription float64 `json:"subscription"`
		Topup        float64 `json:"topup"`
		Refund       float64 `json:"refund"`
	}

	var series []point
	var totalRevenue float64
	for rows.Next() {
		var p point
		var t time.Time
		if err := rows.Scan(&t, &p.Total, &p.Subscription, &p.Topup, &p.Refund); err != nil {
			continue
		}
		p.Date = t.Format("2006-01-02")
		series = append(series, p)
		totalRevenue += p.Total
	}

	writeJSON(w, map[string]any{
		"ok":     true,
		"metric": "revenue",
		"series": series,
		"summary": map[string]any{
			"total_revenue": totalRevenue,
			"avg_daily":     totalRevenue / float64(max(1, len(series))),
		},
	})
}

func (s *Server) statisticsProtocol(w http.ResponseWriter, from, to time.Time, nodeID int64) {
	query := `SELECT protocol, SUM(session_count), SUM(total_bytes), SUM(unique_users)
		FROM protocol_usage_daily WHERE day_date >= ? AND day_date < ?`
	args := []any{from, to}
	if nodeID > 0 {
		query += ` AND node_id = ?`
		args = append(args, nodeID)
	}
	query += ` GROUP BY protocol ORDER BY SUM(session_count) DESC`

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type protoEntry struct {
		Protocol     string `json:"protocol"`
		Sessions     int64  `json:"sessions"`
		TotalBytesGB int64  `json:"total_bytes_gb"`
		UniqueUsers  int64  `json:"unique_users"`
	}

	var entries []protoEntry
	for rows.Next() {
		var e protoEntry
		var totalBytes int64
		if err := rows.Scan(&e.Protocol, &e.Sessions, &totalBytes, &e.UniqueUsers); err != nil {
			continue
		}
		e.TotalBytesGB = totalBytes / (1024 * 1024 * 1024)
		entries = append(entries, e)
	}

	writeJSON(w, map[string]any{
		"ok":        true,
		"metric":    "protocol_usage",
		"protocols": entries,
	})
}

func (s *Server) statisticsNodePerformance(w http.ResponseWriter, from, to time.Time) {
	query := `SELECT bh.node_id, n.name, SUM(bh.rx_bytes + bh.tx_bytes) as total_bytes, AVG(bh.online_users_avg) as avg_users
		FROM bandwidth_hourly bh
		LEFT JOIN nodes n ON n.id = bh.node_id
		WHERE bh.hour_start >= ? AND bh.hour_start < ?
		GROUP BY bh.node_id, n.name
		ORDER BY total_bytes DESC`

	rows, err := s.DB.Query(query, from, to)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type nodePerf struct {
		NodeID       int64   `json:"node_id"`
		NodeName     string  `json:"node_name"`
		TotalBytesGB int64   `json:"total_bytes_gb"`
		AvgUsers     float64 `json:"avg_users"`
	}

	var nodes []nodePerf
	for rows.Next() {
		var np nodePerf
		var totalBytes int64
		if err := rows.Scan(&np.NodeID, &np.NodeName, &totalBytes, &np.AvgUsers); err != nil {
			continue
		}
		np.TotalBytesGB = totalBytes / (1024 * 1024 * 1024)
		nodes = append(nodes, np)
	}

	writeJSON(w, map[string]any{
		"ok":     true,
		"metric": "node_performance",
		"nodes":  nodes,
	})
}
