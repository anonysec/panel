//go:build !lite

package api

import (
	"net/http"
	"time"
)

// adminBillingRevenue returns revenue breakdown by period with MRR calculation.
// GET /api/admin/billing/revenue?period=daily|weekly|monthly&from=2024-01-01&to=2024-01-31
func (s *Server) adminBillingRevenue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse query params
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "daily"
	}
	if period != "daily" && period != "weekly" && period != "monthly" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_period"})
		return
	}

	// Default date range: last 30 days
	now := time.Now().UTC()
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	var fromDate, toDate time.Time
	var err error
	if fromStr != "" {
		fromDate, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_from_date"})
			return
		}
	} else {
		fromDate = now.AddDate(0, 0, -30)
	}
	if toStr != "" {
		toDate, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_to_date"})
			return
		}
		// Include the entire "to" day
		toDate = toDate.Add(24*time.Hour - time.Second)
	} else {
		toDate = now
	}

	// Build GROUP BY clause based on period
	var groupExpr, dateExpr string
	switch period {
	case "weekly":
		groupExpr = "YEARWEEK(created_at, 1)"
		dateExpr = "DATE_FORMAT(MIN(created_at), '%Y-%m-%d')"
	case "monthly":
		groupExpr = "DATE_FORMAT(created_at, '%Y-%m')"
		dateExpr = "DATE_FORMAT(created_at, '%Y-%m')"
	default: // daily
		groupExpr = "DATE(created_at)"
		dateExpr = "DATE(created_at)"
	}

	// Query revenue breakdown from wallet_transactions
	query := `
		SELECT ` + dateExpr + ` AS period_date,
		       COALESCE(SUM(ABS(amount)), 0) AS total_amount,
		       COUNT(*) AS tx_count
		FROM wallet_transactions
		WHERE created_at >= ? AND created_at <= ?
		  AND type IN ('purchase', 'topup', 'refund', 'debit')
		GROUP BY ` + groupExpr + `
		ORDER BY period_date ASC`

	rows, err := s.DB.Query(query, fromDate.Format("2006-01-02 15:04:05"), toDate.Format("2006-01-02 15:04:05"))
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type BreakdownEntry struct {
		Date         string  `json:"date"`
		Amount       float64 `json:"amount"`
		Transactions int     `json:"transactions"`
	}
	breakdown := []BreakdownEntry{}
	var totalRevenue float64

	for rows.Next() {
		var entry BreakdownEntry
		if err := rows.Scan(&entry.Date, &entry.Amount, &entry.Transactions); err != nil {
			continue
		}
		totalRevenue += entry.Amount
		breakdown = append(breakdown, entry)
	}

	// Revenue by type within the date range
	typeRows, err := s.DB.Query(`
		SELECT type, COALESCE(SUM(amount), 0) AS total
		FROM wallet_transactions
		WHERE created_at >= ? AND created_at <= ?
		  AND type IN ('purchase', 'topup', 'refund', 'debit')
		GROUP BY type`, fromDate.Format("2006-01-02 15:04:05"), toDate.Format("2006-01-02 15:04:05"))

	byType := map[string]float64{}
	if err == nil {
		defer typeRows.Close()
		for typeRows.Next() {
			var txType string
			var amount float64
			if typeRows.Scan(&txType, &amount) == nil {
				byType[txType] = amount
			}
		}
	}

	// Calculate MRR: sum of plan prices for all active subscriptions, normalized to monthly
	var mrr float64
	err = s.DB.QueryRow(`
		SELECT COALESCE(SUM(p.price * (30.0 / GREATEST(p.duration_days, 1))), 0)
		FROM subscriptions sub
		JOIN plans p ON p.id = sub.plan_id
		WHERE sub.status = 'active'`).Scan(&mrr)
	if err != nil {
		mrr = 0
	}

	writeJSON(w, map[string]any{
		"ok": true,
		"revenue": map[string]any{
			"total":     totalRevenue,
			"mrr":       mrr,
			"period":    period,
			"breakdown": breakdown,
			"by_type":   byType,
		},
	})
}
