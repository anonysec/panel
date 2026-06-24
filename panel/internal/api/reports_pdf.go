//go:build !lite

package api

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"KorisPanel/panel/internal/reports"
)

// handleReportPDF generates a PDF report for admin download.
// GET /api/admin/reports/pdf?type=revenue&period=monthly
func (s *Server) handleReportPDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	reportType := r.URL.Query().Get("type")
	period := r.URL.Query().Get("period")

	// Validate report type
	switch reportType {
	case "revenue", "users", "bandwidth":
	default:
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_report_type"})
		return
	}

	// Validate period
	var reportPeriod reports.ReportPeriod
	switch period {
	case "daily":
		reportPeriod = reports.PeriodDaily
	case "weekly":
		reportPeriod = reports.PeriodWeekly
	case "monthly":
		reportPeriod = reports.PeriodMonthly
	default:
		reportPeriod = reports.PeriodMonthly
		period = "monthly"
	}

	// Build report data based on type
	var data reports.ReportData
	var err error

	switch reportType {
	case "revenue":
		data, err = s.buildRevenueReportData(reportPeriod)
	case "users":
		data, err = s.buildUsersReportData(reportPeriod)
	case "bandwidth":
		data, err = s.buildBandwidthReportData(reportPeriod)
	}

	if err != nil {
		log.Printf("[reports] failed to build %s report data: %v", reportType, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "report_data_failed"})
		return
	}

	// Generate PDF
	pdfBytes, err := reports.GeneratePDF(data)
	if err != nil {
		log.Printf("[reports] PDF generation failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "pdf_generation_failed"})
		return
	}

	// Set response headers
	now := time.Now().UTC()
	filename := fmt.Sprintf("report-%s-%s-%s.pdf", reportType, period, now.Format("2006-01"))

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))
	w.Write(pdfBytes)
}

// buildRevenueReportData fetches revenue data from the database.
func (s *Server) buildRevenueReportData(period reports.ReportPeriod) (reports.ReportData, error) {
	data := reports.ReportData{
		Title:       "Revenue Report",
		Period:      period,
		GeneratedAt: time.Now().UTC(),
		Columns:     []string{"Date", "Revenue", "Transactions", "New Subscriptions"},
	}

	var groupBy, dateFormat, interval string
	switch period {
	case reports.PeriodWeekly:
		groupBy = "YEARWEEK(wt.created_at)"
		dateFormat = "%Y-W%u"
		interval = "6 MONTH"
	case reports.PeriodMonthly:
		groupBy = "DATE_FORMAT(wt.created_at, '%Y-%m')"
		dateFormat = "%Y-%m"
		interval = "12 MONTH"
	default:
		groupBy = "DATE(wt.created_at)"
		dateFormat = "%Y-%m-%d"
		interval = "90 DAY"
	}

	// Revenue grouped by period
	query := fmt.Sprintf(`
		SELECT DATE_FORMAT(wt.created_at, '%s') as period_label,
		       COALESCE(SUM(CASE WHEN wt.type = 'credit' THEN wt.amount ELSE 0 END), 0) as revenue,
		       COUNT(*) as tx_count
		FROM wallet_transactions wt
		WHERE wt.created_at >= NOW() - INTERVAL %s
		GROUP BY %s
		ORDER BY period_label DESC
		LIMIT 50`, dateFormat, interval, groupBy)

	rows, err := s.DB.Query(query)
	if err != nil {
		return data, fmt.Errorf("revenue query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var label string
		var revenue float64
		var txCount int
		if err := rows.Scan(&label, &revenue, &txCount); err != nil {
			continue
		}
		data.Rows = append(data.Rows, reports.ReportRow{
			Label: label,
			Values: map[string]any{
				"Revenue":           fmt.Sprintf("%.2f", revenue),
				"Transactions":      txCount,
				"New Subscriptions": "-",
			},
		})
	}

	// Add new subscriptions count per period
	subQuery := fmt.Sprintf(`
		SELECT DATE_FORMAT(created_at, '%s') as period_label, COUNT(*) as sub_count
		FROM subscriptions
		WHERE created_at >= NOW() - INTERVAL %s
		GROUP BY %s`, dateFormat, interval, fmt.Sprintf("DATE_FORMAT(created_at, '%s')", dateFormat))

	subRows, err := s.DB.Query(subQuery)
	if err == nil {
		defer subRows.Close()
		subMap := make(map[string]int)
		for subRows.Next() {
			var label string
			var count int
			if subRows.Scan(&label, &count) == nil {
				subMap[label] = count
			}
		}
		// Merge into existing rows
		for i := range data.Rows {
			if count, ok := subMap[data.Rows[i].Label]; ok {
				data.Rows[i].Values["New Subscriptions"] = count
			}
		}
	}

	// Summary
	var totalRevenue float64
	var totalTx int
	s.DB.QueryRow(`SELECT COALESCE(SUM(amount), 0), COUNT(*) FROM wallet_transactions WHERE type = 'credit'`).Scan(&totalRevenue, &totalTx)

	var mrr float64
	s.DB.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM wallet_transactions WHERE type = 'credit' AND created_at >= NOW() - INTERVAL 30 DAY`).Scan(&mrr)

	avgDaily := float64(0)
	if totalTx > 0 {
		s.DB.QueryRow(`SELECT COALESCE(SUM(amount), 0) / GREATEST(DATEDIFF(NOW(), MIN(created_at)), 1) FROM wallet_transactions WHERE type = 'credit'`).Scan(&avgDaily)
	}

	data.Summary = map[string]string{
		"Total Revenue": fmt.Sprintf("%.2f", totalRevenue),
		"Average Daily": fmt.Sprintf("%.2f", avgDaily),
		"MRR":           fmt.Sprintf("%.2f", mrr),
	}

	return data, nil
}

// buildUsersReportData fetches user statistics from the database.
func (s *Server) buildUsersReportData(period reports.ReportPeriod) (reports.ReportData, error) {
	data := reports.ReportData{
		Title:       "Users Report",
		Period:      period,
		GeneratedAt: time.Now().UTC(),
		Columns:     []string{"Date", "New Users", "Active Users", "Churned"},
	}

	var groupBy, dateFormat, interval string
	switch period {
	case reports.PeriodWeekly:
		groupBy = "YEARWEEK(c.created_at)"
		dateFormat = "%Y-W%u"
		interval = "6 MONTH"
	case reports.PeriodMonthly:
		groupBy = "DATE_FORMAT(c.created_at, '%Y-%m')"
		dateFormat = "%Y-%m"
		interval = "12 MONTH"
	default:
		groupBy = "DATE(c.created_at)"
		dateFormat = "%Y-%m-%d"
		interval = "90 DAY"
	}

	// New users per period
	query := fmt.Sprintf(`
		SELECT DATE_FORMAT(c.created_at, '%s') as period_label, COUNT(*) as new_users
		FROM customers c
		WHERE c.deleted_at IS NULL AND c.created_at >= NOW() - INTERVAL %s
		GROUP BY %s
		ORDER BY period_label DESC
		LIMIT 50`, dateFormat, interval, groupBy)

	rows, err := s.DB.Query(query)
	if err != nil {
		return data, fmt.Errorf("users query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var label string
		var newUsers int
		if err := rows.Scan(&label, &newUsers); err != nil {
			continue
		}
		data.Rows = append(data.Rows, reports.ReportRow{
			Label: label,
			Values: map[string]any{
				"New Users":    newUsers,
				"Active Users": "-",
				"Churned":      "-",
			},
		})
	}

	// Total and active counts for summary
	var totalUsers, activeUsers, expiredUsers int
	s.DB.QueryRow(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL`).Scan(&totalUsers)
	s.DB.QueryRow(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL AND status = 'active'`).Scan(&activeUsers)
	s.DB.QueryRow(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL AND status = 'expired'`).Scan(&expiredUsers)

	growthRate := float64(0)
	if totalUsers > 0 {
		// Growth rate: new users in last 30 days / total users
		var newLast30 int
		s.DB.QueryRow(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL AND created_at >= NOW() - INTERVAL 30 DAY`).Scan(&newLast30)
		growthRate = float64(newLast30) / float64(totalUsers) * 100
	}

	data.Summary = map[string]string{
		"Total Users":  fmt.Sprintf("%d", totalUsers),
		"Active Users": fmt.Sprintf("%d", activeUsers),
		"Growth Rate":  fmt.Sprintf("%.1f%%", growthRate),
	}

	return data, nil
}

// buildBandwidthReportData fetches bandwidth statistics from the database.
func (s *Server) buildBandwidthReportData(period reports.ReportPeriod) (reports.ReportData, error) {
	data := reports.ReportData{
		Title:       "Bandwidth Report",
		Period:      period,
		GeneratedAt: time.Now().UTC(),
		Columns:     []string{"Date", "Upload (GB)", "Download (GB)", "Total (GB)"},
	}

	var groupBy, dateFormat, interval string
	switch period {
	case reports.PeriodWeekly:
		groupBy = "YEARWEEK(s.created_at)"
		dateFormat = "%Y-W%u"
		interval = "6 MONTH"
	case reports.PeriodMonthly:
		groupBy = "DATE_FORMAT(s.created_at, '%Y-%m')"
		dateFormat = "%Y-%m"
		interval = "12 MONTH"
	default:
		groupBy = "DATE(s.created_at)"
		dateFormat = "%Y-%m-%d"
		interval = "90 DAY"
	}

	query := fmt.Sprintf(`
		SELECT DATE_FORMAT(s.created_at, '%s') as period_label,
		       COALESCE(SUM(s.tx_bytes), 0) as upload,
		       COALESCE(SUM(s.rx_bytes), 0) as download
		FROM node_usage_snapshots s
		WHERE s.created_at >= NOW() - INTERVAL %s
		GROUP BY %s
		ORDER BY period_label DESC
		LIMIT 50`, dateFormat, interval, groupBy)

	rows, err := s.DB.Query(query)
	if err != nil {
		return data, fmt.Errorf("bandwidth query: %w", err)
	}
	defer rows.Close()

	var totalUpload, totalDownload int64
	var peakTotal int64
	var peakDay string

	for rows.Next() {
		var label string
		var upload, download int64
		if err := rows.Scan(&label, &upload, &download); err != nil {
			continue
		}
		total := upload + download
		totalUpload += upload
		totalDownload += download

		if total > peakTotal {
			peakTotal = total
			peakDay = label
		}

		data.Rows = append(data.Rows, reports.ReportRow{
			Label: label,
			Values: map[string]any{
				"Upload (GB)":   fmt.Sprintf("%.2f", float64(upload)/1073741824),
				"Download (GB)": fmt.Sprintf("%.2f", float64(download)/1073741824),
				"Total (GB)":    fmt.Sprintf("%.2f", float64(total)/1073741824),
			},
		})
	}

	totalBandwidth := totalUpload + totalDownload
	data.Summary = map[string]string{
		"Total Bandwidth": fmt.Sprintf("%.2f GB", float64(totalBandwidth)/1073741824),
		"Total Upload":    fmt.Sprintf("%.2f GB", float64(totalUpload)/1073741824),
		"Total Download":  fmt.Sprintf("%.2f GB", float64(totalDownload)/1073741824),
		"Peak Day":        peakDay,
	}

	return data, nil
}
