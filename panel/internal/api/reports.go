package api

import (
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"time"
)

// ========== Revenue Reports ==========

func (s *Server) revenueReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	period := r.URL.Query().Get("period") // daily, weekly, monthly
	if period == "" {
		period = "daily"
	}

	var groupBy, dateFormat string
	switch period {
	case "weekly":
		groupBy = "TO_CHAR(created_at, 'IYYY-\"W\"IW')"
		dateFormat = "IYYY-\"W\"IW"
	case "monthly":
		groupBy = "TO_CHAR(created_at, 'YYYY-MM')"
		dateFormat = "YYYY-MM"
	default:
		groupBy = "DATE(created_at)"
		dateFormat = "YYYY-MM-DD"
	}

	// Revenue by period
	rows, err := s.DB.Query(`
		SELECT TO_CHAR(created_at, '` + dateFormat + `') as period,
		       COUNT(*) as count,
		       COALESCE(SUM(amount), 0) as total
		FROM payments
		WHERE status = 'approved'
		AND created_at >= NOW() - INTERVAL '90 days'
		GROUP BY ` + groupBy + `
		ORDER BY period DESC
		LIMIT 90`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type RevenuePoint struct {
		Period string  `json:"period"`
		Count  int     `json:"count"`
		Total  float64 `json:"total"`
	}
	points := []RevenuePoint{}
	for rows.Next() {
		var p RevenuePoint
		if rows.Scan(&p.Period, &p.Count, &p.Total) == nil {
			points = append(points, p)
		}
	}

	// Revenue by plan
	planRows, _ := s.DB.Query(`
		SELECT COALESCE(p.name, 'Unknown') as plan_name, COUNT(*) as count, COALESCE(SUM(pay.amount), 0) as total
		FROM payments pay
		LEFT JOIN subscriptions sub ON sub.id = pay.intent_id AND pay.intent_type = 'plan'
		LEFT JOIN plans p ON p.id = sub.plan_id
		WHERE pay.status = 'approved' AND pay.created_at >= NOW() - INTERVAL '30 days'
		GROUP BY p.name
		ORDER BY total DESC`)
	type PlanRevenue struct {
		Plan  string  `json:"plan"`
		Count int     `json:"count"`
		Total float64 `json:"total"`
	}
	byPlan := []PlanRevenue{}
	if planRows != nil {
		defer planRows.Close()
		for planRows.Next() {
			var p PlanRevenue
			if planRows.Scan(&p.Plan, &p.Count, &p.Total) == nil {
				byPlan = append(byPlan, p)
			}
		}
	}

	// Summary stats
	var totalRevenue, todayRevenue float64
	var totalPayments, pendingPayments int
	s.DB.QueryRow(`SELECT COALESCE(SUM(amount),0), COUNT(*) FROM payments WHERE status='approved'`).Scan(&totalRevenue, &totalPayments)
	s.DB.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM payments WHERE status='approved' AND DATE(created_at)=CURRENT_DATE`).Scan(&todayRevenue)
	s.DB.QueryRow(`SELECT COUNT(*) FROM payments WHERE status='pending'`).Scan(&pendingPayments)

	writeJSON(w, map[string]any{
		"ok":               true,
		"period":           period,
		"revenue":          points,
		"by_plan":          byPlan,
		"total_revenue":    totalRevenue,
		"today_revenue":    todayRevenue,
		"total_payments":   totalPayments,
		"pending_payments": pendingPayments,
	})
}

// ========== User Reports ==========

func (s *Server) userReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// New registrations per day (last 30 days)
	regRows, _ := s.DB.Query(`
		SELECT DATE(created_at) as day, COUNT(*) as count
		FROM customers
		WHERE deleted_at IS NULL AND created_at >= NOW() - INTERVAL '30 days'
		GROUP BY DATE(created_at)
		ORDER BY day DESC`)
	type DayCount struct {
		Day   string `json:"day"`
		Count int    `json:"count"`
	}
	registrations := []DayCount{}
	if regRows != nil {
		defer regRows.Close()
		for regRows.Next() {
			var d DayCount
			var t time.Time
			if regRows.Scan(&t, &d.Count) == nil {
				d.Day = t.Format("2006-01-02")
				registrations = append(registrations, d)
			}
		}
	}

	// Status breakdown
	var active, limited, disabled, expired, total int
	s.DB.QueryRow(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL`).Scan(&total)
	s.DB.QueryRow(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL AND status='active'`).Scan(&active)
	s.DB.QueryRow(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL AND status='limited'`).Scan(&limited)
	s.DB.QueryRow(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL AND status='disabled'`).Scan(&disabled)
	s.DB.QueryRow(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL AND status='expired'`).Scan(&expired)

	writeJSON(w, map[string]any{
		"ok":            true,
		"registrations": registrations,
		"total":         total,
		"active":        active,
		"limited":       limited,
		"disabled":      disabled,
		"expired":       expired,
	})
}

// ========== Bandwidth Stats (Dashboard) ==========

func (s *Server) bandwidthStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "1d"
	}

	// Validate period
	var whereClause, groupBy, labelFormat string
	switch period {
	case "1d":
		whereClause = "WHERE acctstarttime >= NOW() - INTERVAL '1 day'"
		groupBy = "EXTRACT(HOUR FROM acctstarttime)"
		labelFormat = "hour"
	case "7d":
		whereClause = "WHERE acctstarttime >= NOW() - INTERVAL '7 days'"
		groupBy = "DATE(acctstarttime)"
		labelFormat = "date"
	case "30d":
		whereClause = "WHERE acctstarttime >= NOW() - INTERVAL '30 days'"
		groupBy = "DATE(acctstarttime)"
		labelFormat = "date"
	case "all":
		whereClause = ""
		groupBy = "TO_CHAR(acctstarttime, 'YYYY-MM')"
		labelFormat = "month"
	default:
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_period"})
		return
	}

	// Get total download/upload for the period
	totalQuery := fmt.Sprintf(`SELECT COALESCE(SUM(acctinputoctets),0), COALESCE(SUM(acctoutputoctets),0) FROM radacct %s`, whereClause)
	var totalDownload, totalUpload int64
	_ = s.DB.QueryRow(totalQuery).Scan(&totalDownload, &totalUpload)

	// Get data points grouped by interval
	var pointsQuery string
	switch labelFormat {
	case "hour":
		pointsQuery = fmt.Sprintf(`SELECT EXTRACT(HOUR FROM acctstarttime)::INT as lbl, COALESCE(SUM(acctinputoctets),0), COALESCE(SUM(acctoutputoctets),0)
			FROM radacct %s GROUP BY %s ORDER BY lbl ASC`, whereClause, groupBy)
	case "date":
		pointsQuery = fmt.Sprintf(`SELECT TO_CHAR(DATE(acctstarttime), 'YYYY-MM-DD') as lbl, COALESCE(SUM(acctinputoctets),0), COALESCE(SUM(acctoutputoctets),0)
			FROM radacct %s GROUP BY %s ORDER BY lbl ASC`, whereClause, groupBy)
	case "month":
		pointsQuery = fmt.Sprintf(`SELECT TO_CHAR(acctstarttime, 'YYYY-MM') as lbl, COALESCE(SUM(acctinputoctets),0), COALESCE(SUM(acctoutputoctets),0)
			FROM radacct GROUP BY %s ORDER BY lbl ASC LIMIT 12`, groupBy)
	}

	rows, err := s.DB.Query(pointsQuery)
	if err != nil {
		log.Printf("[bandwidth-stats] query error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "query_failed"})
		return
	}
	defer rows.Close()

	type DataPoint struct {
		Label    string `json:"label"`
		Download int64  `json:"download"`
		Upload   int64  `json:"upload"`
	}
	points := []DataPoint{}

	for rows.Next() {
		var dp DataPoint
		if labelFormat == "hour" {
			var hour int
			if rows.Scan(&hour, &dp.Download, &dp.Upload) == nil {
				dp.Label = fmt.Sprintf("%02d:00", hour)
				points = append(points, dp)
			}
		} else {
			if rows.Scan(&dp.Label, &dp.Download, &dp.Upload) == nil {
				points = append(points, dp)
			}
		}
	}

	// For 1d period, fill in missing hours
	if labelFormat == "hour" {
		filled := make([]DataPoint, 24)
		hourMap := map[string]DataPoint{}
		for _, p := range points {
			hourMap[p.Label] = p
		}
		for h := 0; h < 24; h++ {
			label := fmt.Sprintf("%02d:00", h)
			if dp, ok := hourMap[label]; ok {
				filled[h] = dp
			} else {
				filled[h] = DataPoint{Label: label, Download: 0, Upload: 0}
			}
		}
		points = filled
	}

	// For 7d and 30d periods, fill in missing dates
	if labelFormat == "date" {
		days := 7
		if period == "30d" {
			days = 30
		}
		dateMap := map[string]DataPoint{}
		for _, p := range points {
			dateMap[p.Label] = p
		}
		filled := make([]DataPoint, 0, days)
		now := time.Now().UTC()
		for i := days - 1; i >= 0; i-- {
			d := now.AddDate(0, 0, -i)
			label := d.Format("2006-01-02")
			if dp, ok := dateMap[label]; ok {
				filled = append(filled, dp)
			} else {
				filled = append(filled, DataPoint{Label: label, Download: 0, Upload: 0})
			}
		}
		points = filled
	}

	// For "all" period, fill in missing months (last 12 months)
	if labelFormat == "month" {
		monthMap := map[string]DataPoint{}
		for _, p := range points {
			monthMap[p.Label] = p
		}
		filled := make([]DataPoint, 0, 12)
		now := time.Now().UTC()
		for i := 11; i >= 0; i-- {
			d := now.AddDate(0, -i, 0)
			label := d.Format("2006-01")
			if dp, ok := monthMap[label]; ok {
				filled = append(filled, dp)
			} else {
				filled = append(filled, DataPoint{Label: label, Download: 0, Upload: 0})
			}
		}
		points = filled
	}

	writeJSON(w, map[string]any{
		"ok":             true,
		"total_download": totalDownload,
		"total_upload":   totalUpload,
		"points":         points,
	})
}

// ========== Bandwidth Reports ==========

func (s *Server) bandwidthReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Per-node bandwidth (last 24h)
	nodeRows, _ := s.DB.Query(`
		SELECT n.name, COALESCE(SUM(s.rx_bytes),0) as rx, COALESCE(SUM(s.tx_bytes),0) as tx
		FROM node_usage_snapshots s
		JOIN nodes n ON n.id = s.node_id
		WHERE s.created_at >= NOW() - INTERVAL '24 hours'
		GROUP BY n.id, n.name
		ORDER BY rx + tx DESC`)
	type NodeBandwidth struct {
		Node    string `json:"node"`
		RxBytes int64  `json:"rx_bytes"`
		TxBytes int64  `json:"tx_bytes"`
	}
	byNode := []NodeBandwidth{}
	if nodeRows != nil {
		defer nodeRows.Close()
		for nodeRows.Next() {
			var nb NodeBandwidth
			if nodeRows.Scan(&nb.Node, &nb.RxBytes, &nb.TxBytes) == nil {
				byNode = append(byNode, nb)
			}
		}
	}

	// Top users by bandwidth (last 24h)
	userRows, _ := s.DB.Query(`
		SELECT username, COALESCE(SUM(acctinputoctets),0) as rx, COALESCE(SUM(acctoutputoctets),0) as tx
		FROM radacct
		WHERE acctstarttime >= NOW() - INTERVAL '24 hours'
		GROUP BY username
		ORDER BY rx + tx DESC
		LIMIT 20`)
	type UserBandwidth struct {
		Username string `json:"username"`
		RxBytes  int64  `json:"rx_bytes"`
		TxBytes  int64  `json:"tx_bytes"`
	}
	topUsers := []UserBandwidth{}
	if userRows != nil {
		defer userRows.Close()
		for userRows.Next() {
			var ub UserBandwidth
			if userRows.Scan(&ub.Username, &ub.RxBytes, &ub.TxBytes) == nil {
				topUsers = append(topUsers, ub)
			}
		}
	}

	// Total bandwidth today
	var todayRx, todayTx int64
	s.DB.QueryRow(`SELECT COALESCE(SUM(acctinputoctets),0), COALESCE(SUM(acctoutputoctets),0) FROM radacct WHERE acctstarttime >= CURRENT_DATE`).Scan(&todayRx, &todayTx)

	writeJSON(w, map[string]any{
		"ok":        true,
		"by_node":   byNode,
		"top_users": topUsers,
		"today_rx":  todayRx,
		"today_tx":  todayTx,
	})
}

// ========== CSV Export ==========

func (s *Server) exportRevenueCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`
		SELECT DATE(created_at), username, amount, method, status
		FROM payments
		WHERE created_at >= NOW() - INTERVAL '90 days'
		ORDER BY created_at DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="revenue-report.csv"`)
	cw := csv.NewWriter(w)
	cw.Write([]string{"Date", "Username", "Amount", "Method", "Status"})
	for rows.Next() {
		var date, username, method, status string
		var amount float64
		if rows.Scan(&date, &username, &amount, &method, &status) == nil {
			cw.Write([]string{date, username, fmt.Sprintf("%.2f", amount), method, status})
		}
	}
	cw.Flush()
}

// ========== Uptime Monitoring ==========

func (s *Server) uptimeReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Node uptime over last 24h (based on status changes)
	rows, err := s.DB.Query(`
		SELECT n.id, n.name, n.status, n.last_seen_at,
		       EXTRACT(EPOCH FROM (NOW() - COALESCE(n.last_seen_at, n.created_at)))::INT / 60 as minutes_since_seen
		FROM nodes n
		WHERE n.status <> 'disabled'
		ORDER BY n.id`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type NodeUptime struct {
		ID            int64   `json:"id"`
		Name          string  `json:"name"`
		Status        string  `json:"status"`
		LastSeen      string  `json:"last_seen_at"`
		MinutesSince  int     `json:"minutes_since_seen"`
		UptimePercent float64 `json:"uptime_percent"`
	}
	nodes := []NodeUptime{}
	for rows.Next() {
		var n NodeUptime
		var lastSeen time.Time
		if rows.Scan(&n.ID, &n.Name, &n.Status, &lastSeen, &n.MinutesSince) == nil {
			n.LastSeen = lastSeen.Format(time.RFC3339)
			// Simple uptime: if online and seen within 5 min = 100%, else degrade
			if n.Status == "online" && n.MinutesSince <= 5 {
				n.UptimePercent = 100.0
			} else if n.Status == "stale" || n.MinutesSince <= 15 {
				n.UptimePercent = 95.0
			} else if n.Status == "offline" {
				n.UptimePercent = 0.0
			} else {
				n.UptimePercent = 50.0
			}
			nodes = append(nodes, n)
		}
	}

	writeJSON(w, map[string]any{"ok": true, "nodes": nodes})
}

// ========== Balance / Wallet Summary ==========

func (s *Server) walletSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	var totalCredit, avgCredit float64
	var activeWallets, zeroWallets int
	s.DB.QueryRow(`SELECT COALESCE(SUM(credit),0), COALESCE(AVG(credit),0), COUNT(*) FROM wallets WHERE credit > 0`).Scan(&totalCredit, &avgCredit, &activeWallets)
	s.DB.QueryRow(`SELECT COUNT(*) FROM wallets WHERE credit <= 0`).Scan(&zeroWallets)

	// Recent transactions
	type RecentTx struct {
		Username string  `json:"username"`
		Amount   float64 `json:"amount"`
		Type     string  `json:"type"`
		Date     string  `json:"date"`
	}
	txRows, _ := s.DB.Query(`SELECT username, amount, type, created_at FROM wallet_transactions ORDER BY id DESC LIMIT 20`)
	recent := []RecentTx{}
	if txRows != nil {
		defer txRows.Close()
		for txRows.Next() {
			var tx RecentTx
			var created time.Time
			if txRows.Scan(&tx.Username, &tx.Amount, &tx.Type, &created) == nil {
				tx.Date = created.Format(time.RFC3339)
				recent = append(recent, tx)
			}
		}
	}

	writeJSON(w, map[string]any{
		"ok":             true,
		"total_credit":   totalCredit,
		"avg_credit":     avgCredit,
		"active_wallets": activeWallets,
		"zero_wallets":   zeroWallets,
		"recent":         recent,
	})
}
