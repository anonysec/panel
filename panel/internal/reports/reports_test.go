//go:build !lite

package reports

import (
	"testing"
	"time"
)

func TestGeneratePDF_ValidOutput(t *testing.T) {
	data := ReportData{
		Title:       "Revenue Report",
		Period:      PeriodMonthly,
		GeneratedAt: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		Columns:     []string{"Date", "Revenue", "Transactions", "New Subscriptions"},
		Rows: []ReportRow{
			{Label: "2025-01", Values: map[string]any{"Revenue": "1250.00", "Transactions": 42, "New Subscriptions": 15}},
			{Label: "2024-12", Values: map[string]any{"Revenue": "980.50", "Transactions": 35, "New Subscriptions": 12}},
			{Label: "2024-11", Values: map[string]any{"Revenue": "1100.00", "Transactions": 38, "New Subscriptions": 14}},
		},
		Summary: map[string]string{
			"Total Revenue": "$3,330.50",
			"Average Daily": "$111.02",
			"MRR":           "$1,250.00",
		},
	}

	pdfBytes, err := GeneratePDF(data)
	if err != nil {
		t.Fatalf("GeneratePDF failed: %v", err)
	}

	if len(pdfBytes) == 0 {
		t.Fatal("GeneratePDF returned empty bytes")
	}

	// Valid PDF files start with %PDF-
	if len(pdfBytes) < 5 || string(pdfBytes[:5]) != "%PDF-" {
		t.Fatalf("output does not start with %%PDF- header, got: %q", string(pdfBytes[:min(20, len(pdfBytes))]))
	}
}

func TestGeneratePDF_EmptyRows(t *testing.T) {
	data := ReportData{
		Title:       "Empty Report",
		Period:      PeriodDaily,
		GeneratedAt: time.Now().UTC(),
		Columns:     []string{},
		Rows:        []ReportRow{},
		Summary:     map[string]string{"Status": "No data available"},
	}

	pdfBytes, err := GeneratePDF(data)
	if err != nil {
		t.Fatalf("GeneratePDF failed on empty data: %v", err)
	}

	if len(pdfBytes) == 0 {
		t.Fatal("GeneratePDF returned empty bytes for empty data")
	}

	if string(pdfBytes[:5]) != "%PDF-" {
		t.Fatalf("output does not start with %%PDF- header")
	}
}

func TestGeneratePDF_UserReport(t *testing.T) {
	data := ReportData{
		Title:       "Users Report",
		Period:      PeriodWeekly,
		GeneratedAt: time.Now().UTC(),
		Columns:     []string{"Date", "New Users", "Active Users", "Churned"},
		Rows: []ReportRow{
			{Label: "2025-W03", Values: map[string]any{"New Users": 25, "Active Users": 150, "Churned": 3}},
			{Label: "2025-W02", Values: map[string]any{"New Users": 18, "Active Users": 145, "Churned": 5}},
		},
		Summary: map[string]string{
			"Total Users": "320",
			"Growth Rate": "8.5%",
		},
	}

	pdfBytes, err := GeneratePDF(data)
	if err != nil {
		t.Fatalf("GeneratePDF failed: %v", err)
	}

	if string(pdfBytes[:5]) != "%PDF-" {
		t.Fatalf("output does not start with %%PDF- header")
	}
}

func TestReportTypes(t *testing.T) {
	if ReportRevenue != "revenue" {
		t.Errorf("ReportRevenue = %q, want %q", ReportRevenue, "revenue")
	}
	if ReportUsers != "users" {
		t.Errorf("ReportUsers = %q, want %q", ReportUsers, "users")
	}
	if ReportBandwidth != "bandwidth" {
		t.Errorf("ReportBandwidth = %q, want %q", ReportBandwidth, "bandwidth")
	}
}

func TestReportPeriods(t *testing.T) {
	if PeriodDaily != "daily" {
		t.Errorf("PeriodDaily = %q, want %q", PeriodDaily, "daily")
	}
	if PeriodWeekly != "weekly" {
		t.Errorf("PeriodWeekly = %q, want %q", PeriodWeekly, "weekly")
	}
	if PeriodMonthly != "monthly" {
		t.Errorf("PeriodMonthly = %q, want %q", PeriodMonthly, "monthly")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
