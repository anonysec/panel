//go:build !lite

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNodeSLA_MethodNotAllowed(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/1/sla", nil)
	rec := httptest.NewRecorder()

	s.nodeSLA(rec, req, 1)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestNodeSLA_InvalidMonthFormat(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/nodes/1/sla?month=invalid", nil)
	rec := httptest.NewRecorder()

	s.nodeSLA(rec, req, 1)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "invalid_month_format" {
		t.Errorf("error = %v, want invalid_month_format", resp["error"])
	}
}

func TestNodeSLA_NoDowntimes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	// Expect query for current month (the main SLA call)
	rows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"})
	mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(rows)

	// Expect queries for 6 months of history (default)
	for i := 0; i < 6; i++ {
		histRows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"})
		mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(histRows)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/nodes/5/sla?month=2024-06", nil)
	rec := httptest.NewRecorder()

	s.nodeSLA(rec, req, 5)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
	if resp["node_id"].(float64) != 5 {
		t.Errorf("node_id = %v, want 5", resp["node_id"])
	}
	if resp["month"] != "2024-06" {
		t.Errorf("month = %v, want 2024-06", resp["month"])
	}
	// June has 30 days = 720 hours
	if resp["total_hours"].(float64) != 720 {
		t.Errorf("total_hours = %v, want 720", resp["total_hours"])
	}
	if resp["downtime_hours"].(float64) != 0 {
		t.Errorf("downtime_hours = %v, want 0", resp["downtime_hours"])
	}
	if resp["downtime_minutes"].(float64) != 0 {
		t.Errorf("downtime_minutes = %v, want 0", resp["downtime_minutes"])
	}
	if resp["availability_percent"].(float64) != 100 {
		t.Errorf("availability_percent = %v, want 100", resp["availability_percent"])
	}
	downtimes := resp["downtimes"].([]any)
	if len(downtimes) != 0 {
		t.Errorf("downtimes length = %d, want 0", len(downtimes))
	}
	// Check history is returned
	history := resp["history"].([]any)
	if len(history) != 6 {
		t.Errorf("history length = %d, want 6", len(history))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestNodeSLA_WithDowntimes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	// Simulate 2 downtime entries in June 2024
	start1 := time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC)
	end1 := time.Date(2024, 6, 10, 14, 0, 0, 0, time.UTC) // 2 hours = 7200 seconds

	start2 := time.Date(2024, 6, 20, 8, 0, 0, 0, time.UTC)
	end2 := time.Date(2024, 6, 20, 8, 30, 0, 0, time.UTC) // 30 minutes = 1800 seconds

	// Main query for current month SLA
	rows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"}).
		AddRow(1, start1, end1, 7200, "Hardware upgrade").
		AddRow(2, start2, end2, 1800, "Network issue")
	mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(rows)

	// History queries (6 months)
	for i := 0; i < 6; i++ {
		histRows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"})
		if i == 0 {
			// First history entry is same as current month
			histRows.AddRow(1, start1, end1, 7200, "Hardware upgrade").
				AddRow(2, start2, end2, 1800, "Network issue")
		}
		mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(histRows)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/nodes/5/sla?month=2024-06", nil)
	rec := httptest.NewRecorder()

	s.nodeSLA(rec, req, 5)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}

	// Total downtime: 7200 + 1800 = 9000 seconds = 2.5 hours = 150 minutes
	if resp["downtime_hours"].(float64) != 2.5 {
		t.Errorf("downtime_hours = %v, want 2.5", resp["downtime_hours"])
	}
	if resp["downtime_minutes"].(float64) != 150 {
		t.Errorf("downtime_minutes = %v, want 150", resp["downtime_minutes"])
	}

	// June = 30 days = 2592000 seconds
	// Availability: (2592000 - 9000) / 2592000 * 100 = 99.65...%
	avail := resp["availability_percent"].(float64)
	if avail < 99.65 || avail > 99.66 {
		t.Errorf("availability_percent = %v, want ~99.65", avail)
	}

	downtimes := resp["downtimes"].([]any)
	if len(downtimes) != 2 {
		t.Errorf("downtimes length = %d, want 2", len(downtimes))
	}

	// Check history includes incident count
	history := resp["history"].([]any)
	if len(history) != 6 {
		t.Errorf("history length = %d, want 6", len(history))
	}
	firstMonth := history[0].(map[string]any)
	if firstMonth["incident_count"].(float64) != 2 {
		t.Errorf("history[0].incident_count = %v, want 2", firstMonth["incident_count"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestNodeSLA_DefaultsToCurrentMonth(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	// Main query
	rows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"})
	mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(rows)

	// History queries (6 months default)
	for i := 0; i < 6; i++ {
		histRows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"})
		mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(histRows)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/nodes/3/sla", nil)
	rec := httptest.NewRecorder()

	s.nodeSLA(rec, req, 3)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	// Should default to current month
	now := time.Now().UTC()
	expectedMonth := now.Format("2006-01")
	if resp["month"] != expectedMonth {
		t.Errorf("month = %v, want %v", resp["month"], expectedMonth)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestNodeSLA_CustomMonthsParam(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	// Main query
	rows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"})
	mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(rows)

	// History queries (3 months as specified)
	for i := 0; i < 3; i++ {
		histRows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"})
		mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(histRows)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/nodes/2/sla?month=2024-06&months=3", nil)
	rec := httptest.NewRecorder()

	s.nodeSLA(rec, req, 2)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	history := resp["history"].([]any)
	if len(history) != 3 {
		t.Errorf("history length = %d, want 3", len(history))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestNodesSLASummary_MethodNotAllowed(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/nodes/sla-summary", nil)
	rec := httptest.NewRecorder()

	s.nodesSLASummary(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestNodesSLASummary_NoNodes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	nodeRows := sqlmock.NewRows([]string{"id", "name", "status"})
	mock.ExpectQuery("SELECT id, name, status FROM nodes").WillReturnRows(nodeRows)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/nodes/sla-summary?month=2024-06", nil)
	rec := httptest.NewRecorder()

	s.nodesSLASummary(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
	if resp["total_nodes"].(float64) != 0 {
		t.Errorf("total_nodes = %v, want 0", resp["total_nodes"])
	}
	if resp["fleet_availability"].(float64) != 0 {
		t.Errorf("fleet_availability = %v, want 0", resp["fleet_availability"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestNodesSLASummary_WithNodes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	// Two nodes
	nodeRows := sqlmock.NewRows([]string{"id", "name", "status"}).
		AddRow(1, "US-East-1", "online").
		AddRow(2, "EU-West-1", "online")
	mock.ExpectQuery("SELECT id, name, status FROM nodes").WillReturnRows(nodeRows)

	// Node 1: no downtimes
	node1Rows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"})
	mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(node1Rows)

	// Node 2: 1 hour downtime
	start := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 15, 11, 0, 0, 0, time.UTC)
	node2Rows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"}).
		AddRow(1, start, end, 3600, "Network outage")
	mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(node2Rows)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/nodes/sla-summary?month=2024-06", nil)
	rec := httptest.NewRecorder()

	s.nodesSLASummary(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
	if resp["total_nodes"].(float64) != 2 {
		t.Errorf("total_nodes = %v, want 2", resp["total_nodes"])
	}

	nodes := resp["nodes"].([]any)
	if len(nodes) != 2 {
		t.Fatalf("nodes length = %d, want 2", len(nodes))
	}

	// Node 1 should have 100% availability
	n1 := nodes[0].(map[string]any)
	if n1["availability_percent"].(float64) != 100 {
		t.Errorf("node1 availability = %v, want 100", n1["availability_percent"])
	}

	// Node 2: 1 hour downtime in June (2592000 sec total)
	// Availability: (2592000 - 3600) / 2592000 * 100 = 99.86%
	n2 := nodes[1].(map[string]any)
	if n2["availability_percent"].(float64) < 99.86 || n2["availability_percent"].(float64) > 99.87 {
		t.Errorf("node2 availability = %v, want ~99.86", n2["availability_percent"])
	}
	if n2["downtime_minutes"].(float64) != 60 {
		t.Errorf("node2 downtime_minutes = %v, want 60", n2["downtime_minutes"])
	}
	if n2["incident_count"].(float64) != 1 {
		t.Errorf("node2 incident_count = %v, want 1", n2["incident_count"])
	}

	// Fleet average should be ~99.93%
	fleetAvail := resp["fleet_availability"].(float64)
	if fleetAvail < 99.93 || fleetAvail > 99.94 {
		t.Errorf("fleet_availability = %v, want ~99.93", fleetAvail)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestRecordNodeDowntime_SkipsIfOpenExists(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	// Return count = 1 (open downtime already exists)
	mock.ExpectQuery("SELECT COUNT").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	RecordNodeDowntime(db, 5, "test reason")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestRecordNodeDowntime_CreatesEntry(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	// Return count = 0 (no open downtime)
	mock.ExpectQuery("SELECT COUNT").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	mock.ExpectExec("INSERT INTO node_downtimes").
		WithArgs(int64(5), "Node went offline").
		WillReturnResult(sqlmock.NewResult(1, 1))

	RecordNodeDowntime(db, 5, "Node went offline")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCloseNodeDowntime(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectExec("UPDATE node_downtimes SET ended_at").
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	CloseNodeDowntime(db, 7)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCalculateMonthSLA_OngoingDowntime(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	// An ongoing downtime (ended_at IS NULL) started 2 hours ago
	start := time.Now().UTC().Add(-2 * time.Hour)
	rows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"}).
		AddRow(1, start, nil, 0, "Network failure")
	mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(rows)

	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)

	result := s.calculateMonthSLA(1, monthStart, monthEnd)

	// Should have 1 incident
	if result.incidentCount != 1 {
		t.Errorf("incidentCount = %d, want 1", result.incidentCount)
	}
	// Downtime should be approximately 2 hours (7200 seconds), allow some tolerance
	if result.downtimeHours < 1.9 || result.downtimeHours > 2.1 {
		t.Errorf("downtimeHours = %v, want ~2.0", result.downtimeHours)
	}
	// Availability should be less than 100%
	if result.availabilityPercent >= 100 {
		t.Error("availability should be < 100% with ongoing downtime")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCalculateMonthSLA_DowntimeSpansMonthBoundary(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	// Downtime started May 30 and ended June 2 — only June portion counts
	monthStart := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	monthEnd := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	start := time.Date(2024, 5, 30, 12, 0, 0, 0, time.UTC) // before month
	end := time.Date(2024, 6, 2, 12, 0, 0, 0, time.UTC)    // 1.5 days into June

	rows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"}).
		AddRow(1, start, end, 259200, "Major outage")
	mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(rows)

	result := s.calculateMonthSLA(1, monthStart, monthEnd)

	// Effective downtime: June 1 00:00 to June 2 12:00 = 36 hours
	expectedHours := 36.0
	if result.downtimeHours < expectedHours-0.1 || result.downtimeHours > expectedHours+0.1 {
		t.Errorf("downtimeHours = %v, want ~%v", result.downtimeHours, expectedHours)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCalculateMonthSLA_DowntimeExceedsMonthEnd(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	// Downtime ends after month end — should be clamped
	monthStart := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	monthEnd := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	start := time.Date(2024, 6, 28, 0, 0, 0, 0, time.UTC) // 3 days before end
	end := time.Date(2024, 7, 5, 0, 0, 0, 0, time.UTC)    // 5 days after month start

	rows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"}).
		AddRow(1, start, end, 604800, "Prolonged outage")
	mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(rows)

	result := s.calculateMonthSLA(1, monthStart, monthEnd)

	// Effective downtime: June 28 to July 1 = 3 days = 72 hours
	expectedHours := 72.0
	if result.downtimeHours < expectedHours-0.1 || result.downtimeHours > expectedHours+0.1 {
		t.Errorf("downtimeHours = %v, want ~%v", result.downtimeHours, expectedHours)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCalculateMonthSLA_100PercentDowntime(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	// Downtime covers the entire month
	monthStart := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	monthEnd := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	start := time.Date(2024, 5, 25, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 7, 10, 0, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"}).
		AddRow(1, start, end, 3888000, "Complete outage")
	mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(rows)

	result := s.calculateMonthSLA(1, monthStart, monthEnd)

	// Should be 0% availability
	if result.availabilityPercent != 0 {
		t.Errorf("availability = %v, want 0", result.availabilityPercent)
	}
	// 720 hours of downtime in June
	if result.downtimeHours != 720 {
		t.Errorf("downtimeHours = %v, want 720", result.downtimeHours)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestCalculateMonthSLA_MultipleOverlappingDowntimes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	s := &Server{DB: db}

	monthStart := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	monthEnd := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	// Two entries that overlap (shouldn't happen but we test robustness)
	// Entry 1: June 10 12:00 - June 10 14:00 (2 hours)
	// Entry 2: June 10 13:00 - June 10 15:00 (2 hours)
	// Total counted: 4 hours (no dedup — records are independent)
	start1 := time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC)
	end1 := time.Date(2024, 6, 10, 14, 0, 0, 0, time.UTC)
	start2 := time.Date(2024, 6, 10, 13, 0, 0, 0, time.UTC)
	end2 := time.Date(2024, 6, 10, 15, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{"id", "started_at", "ended_at", "duration_seconds", "reason"}).
		AddRow(1, start1, end1, 7200, "Issue A").
		AddRow(2, start2, end2, 7200, "Issue B")
	mock.ExpectQuery("SELECT id, started_at, ended_at, duration_seconds, COALESCE").WillReturnRows(rows)

	result := s.calculateMonthSLA(1, monthStart, monthEnd)

	// Both entries counted independently → 4 hours total
	if result.incidentCount != 2 {
		t.Errorf("incidentCount = %d, want 2", result.incidentCount)
	}
	if result.downtimeHours != 4 {
		t.Errorf("downtimeHours = %v, want 4", result.downtimeHours)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestRecordNodeDowntime_MultipleCallsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	// First call: no open downtime → creates one
	mock.ExpectQuery("SELECT COUNT").
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO node_downtimes").
		WithArgs(int64(3), "Offline detected").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Second call: open downtime exists → skips
	mock.ExpectQuery("SELECT COUNT").
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	RecordNodeDowntime(db, 3, "Offline detected")
	RecordNodeDowntime(db, 3, "Offline detected")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
