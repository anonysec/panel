//go:build !lite

package reports

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/go-pdf/fpdf"
)

// ReportType defines the type of report to generate.
type ReportType string

const (
	ReportRevenue   ReportType = "revenue"
	ReportUsers     ReportType = "users"
	ReportBandwidth ReportType = "bandwidth"
)

// ReportPeriod defines the time granularity for report data.
type ReportPeriod string

const (
	PeriodDaily   ReportPeriod = "daily"
	PeriodWeekly  ReportPeriod = "weekly"
	PeriodMonthly ReportPeriod = "monthly"
)

// ReportData holds all data needed to generate a PDF report.
type ReportData struct {
	Title       string
	Period      ReportPeriod
	GeneratedAt time.Time
	Columns     []string
	Rows        []ReportRow
	Summary     map[string]string
}

// ReportRow represents a single row in the report table.
type ReportRow struct {
	Label  string
	Values map[string]any
}

// GeneratePDF generates a styled PDF document from report data.
func GeneratePDF(data ReportData) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 20)

	// Footer with page numbers
	pdf.SetFooterFunc(func() {
		pdf.SetY(-15)
		pdf.SetFont("Arial", "I", 8)
		pdf.SetTextColor(128, 128, 128)
		pdf.CellFormat(0, 10,
			fmt.Sprintf("Page %d/{nb}", pdf.PageNo()),
			"", 0, "C", false, 0, "")
	})
	pdf.AliasNbPages("")

	pdf.AddPage()

	// Header section
	pdf.SetFont("Arial", "B", 20)
	pdf.SetTextColor(33, 37, 41)
	pdf.CellFormat(0, 12, data.Title, "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(108, 117, 125)
	pdf.CellFormat(0, 6,
		fmt.Sprintf("Period: %s | Generated: %s",
			string(data.Period),
			data.GeneratedAt.Format("2006-01-02 15:04 UTC")),
		"", 1, "L", false, 0, "")

	pdf.Ln(4)

	// Separator line
	pdf.SetDrawColor(222, 226, 230)
	pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	pdf.Ln(6)

	// Table
	if len(data.Columns) > 0 && len(data.Rows) > 0 {
		drawTable(pdf, data.Columns, data.Rows)
	}

	pdf.Ln(8)

	// Summary section
	if len(data.Summary) > 0 {
		drawSummary(pdf, data.Summary)
	}

	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, fmt.Errorf("pdf generation failed: %w", err)
	}
	return buf.Bytes(), nil
}

func drawTable(pdf *fpdf.Fpdf, columns []string, rows []ReportRow) {
	colCount := len(columns)
	if colCount == 0 {
		return
	}

	// Calculate column widths (distribute across page width)
	pageWidth := 190.0 // A4 with 10mm margins
	colWidth := pageWidth / float64(colCount)

	// Table header
	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(248, 249, 250)
	pdf.SetTextColor(33, 37, 41)
	pdf.SetDrawColor(222, 226, 230)

	for _, col := range columns {
		pdf.CellFormat(colWidth, 8, col, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	// Table rows
	pdf.SetFont("Arial", "", 9)
	pdf.SetFillColor(255, 255, 255)

	for i, row := range rows {
		// Alternate row background
		if i%2 == 1 {
			pdf.SetFillColor(248, 249, 250)
		} else {
			pdf.SetFillColor(255, 255, 255)
		}

		// First column is the label
		pdf.CellFormat(colWidth, 7, row.Label, "1", 0, "L", true, 0, "")

		// Remaining columns from Values map
		for _, col := range columns[1:] {
			val := ""
			if v, ok := row.Values[col]; ok {
				val = fmt.Sprintf("%v", v)
			}
			pdf.CellFormat(colWidth, 7, val, "1", 0, "R", true, 0, "")
		}
		pdf.Ln(-1)
	}
}

func drawSummary(pdf *fpdf.Fpdf, summary map[string]string) {
	pdf.SetFont("Arial", "B", 12)
	pdf.SetTextColor(33, 37, 41)
	pdf.CellFormat(0, 8, "Summary", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	// Sort keys for consistent ordering
	keys := make([]string, 0, len(summary))
	for k := range summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pdf.SetFont("Arial", "", 10)
	for _, key := range keys {
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(60, 7, key+":", "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(0, 7, summary[key], "", 1, "L", false, 0, "")
	}
}
