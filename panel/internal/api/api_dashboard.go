package api

import "net/http"

func (s *Server) dashboardStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	result, err := s.cachedQuery("stats:dashboard", func() (any, error) {
		return s.dashboardStatsPayload(), nil
	})
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, result)
}

func (s *Server) dashboardStatsPayload() map[string]any {
	var rx, tx float64
	_ = s.DB.QueryRow(`SELECT COALESCE(SUM(ns.rx_bps),0), COALESCE(SUM(ns.tx_bps),0) FROM node_status ns JOIN nodes n ON n.id=ns.node_id WHERE n.status <> 'disabled' AND ns.updated_at >= NOW() - INTERVAL '5 minutes'`).Scan(&rx, &tx)

	// Total data usage from radacct (all sessions, including closed ones)
	var totalInput, totalOutput int64
	_ = s.DB.QueryRow(`SELECT COALESCE(SUM(acctinputoctets),0), COALESCE(SUM(acctoutputoctets),0) FROM radacct`).Scan(&totalInput, &totalOutput)

	// Today's data usage
	var todayInput, todayOutput int64
	_ = s.DB.QueryRow(`SELECT COALESCE(SUM(acctinputoctets),0), COALESCE(SUM(acctoutputoctets),0) FROM radacct WHERE acctstarttime >= CURRENT_DATE`).Scan(&todayInput, &todayOutput)

	return map[string]any{
		"ok":                 true,
		"customers":          s.count(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL`),
		"active_customers":   s.count(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL AND status='active'`),
		"plans":              s.count(`SELECT COUNT(*) FROM plans WHERE is_active=TRUE`),
		"nodes":              s.count(`SELECT COUNT(*) FROM nodes WHERE status IN('online','stale')`),
		"online_users":       s.count(`SELECT COUNT(DISTINCT username) FROM radacct WHERE acctstoptime IS NULL`),
		"active_sessions":    s.count(`SELECT COUNT(*) FROM radacct WHERE acctstoptime IS NULL`),
		"open_tickets":       s.count(`SELECT COUNT(*) FROM tickets WHERE deleted_at IS NULL AND status='open'`),
		"pending_payments":   s.count(`SELECT COUNT(*) FROM payments WHERE status='pending'`),
		"approved_payments":  s.sum(`SELECT COALESCE(SUM(amount),0) FROM payments WHERE status='approved'`),
		"unseen_events":      s.count(`SELECT COUNT(*) FROM events WHERE seen=FALSE`),
		"total_rx_bps":       rx,
		"total_tx_bps":       tx,
		"total_input_bytes":  totalInput,
		"total_output_bytes": totalOutput,
		"today_input_bytes":  todayInput,
		"today_output_bytes": todayOutput,
	}
}
