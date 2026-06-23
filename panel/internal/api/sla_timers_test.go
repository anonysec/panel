//go:build !lite

package api

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ──────────────────────────────────────────────────────────────────────────────
// loadSLATargets Tests
// ──────────────────────────────────────────────────────────────────────────────

func TestLoadSLATargets(t *testing.T) {
	tests := []struct {
		name       string
		rows       [][]string // each row: [key, value]
		queryErr   bool
		wantLow    int
		wantMedium int
		wantHigh   int
	}{
		{
			name:       "returns defaults when no settings exist",
			rows:       nil,
			wantLow:    defaultSLALowMinutes,
			wantMedium: defaultSLAMediumMinutes,
			wantHigh:   defaultSLAHighMinutes,
		},
		{
			name: "overrides all three targets from DB",
			rows: [][]string{
				{"sla_response_minutes_low", "600"},
				{"sla_response_minutes_medium", "60"},
				{"sla_response_minutes_high", "15"},
			},
			wantLow:    600,
			wantMedium: 60,
			wantHigh:   15,
		},
		{
			name: "partial override — only high configured",
			rows: [][]string{
				{"sla_response_minutes_high", "10"},
			},
			wantLow:    defaultSLALowMinutes,
			wantMedium: defaultSLAMediumMinutes,
			wantHigh:   10,
		},
		{
			name: "ignores invalid (non-positive) values",
			rows: [][]string{
				{"sla_response_minutes_low", "-5"},
				{"sla_response_minutes_medium", "0"},
				{"sla_response_minutes_high", "abc"},
			},
			wantLow:    defaultSLALowMinutes,
			wantMedium: defaultSLAMediumMinutes,
			wantHigh:   defaultSLAHighMinutes,
		},
		{
			name: "trims whitespace from values",
			rows: [][]string{
				{"sla_response_minutes_low", " 300 "},
			},
			wantLow:    300,
			wantMedium: defaultSLAMediumMinutes,
			wantHigh:   defaultSLAHighMinutes,
		},
		{
			name:       "returns defaults on query error",
			rows:       nil,
			queryErr:   true,
			wantLow:    defaultSLALowMinutes,
			wantMedium: defaultSLAMediumMinutes,
			wantHigh:   defaultSLAHighMinutes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()

			if tt.queryErr {
				mock.ExpectQuery("SELECT setting_key, setting_value FROM panel_settings").
					WillReturnError(sqlmock.ErrCancelled)
			} else {
				rows := sqlmock.NewRows([]string{"setting_key", "setting_value"})
				for _, r := range tt.rows {
					rows.AddRow(r[0], r[1])
				}
				mock.ExpectQuery("SELECT setting_key, setting_value FROM panel_settings").
					WillReturnRows(rows)
			}

			targets := loadSLATargets(db)

			if targets.LowMinutes != tt.wantLow {
				t.Errorf("LowMinutes = %d, want %d", targets.LowMinutes, tt.wantLow)
			}
			if targets.MediumMinutes != tt.wantMedium {
				t.Errorf("MediumMinutes = %d, want %d", targets.MediumMinutes, tt.wantMedium)
			}
			if targets.HighMinutes != tt.wantHigh {
				t.Errorf("HighMinutes = %d, want %d", targets.HighMinutes, tt.wantHigh)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// slaTargetForPriority Tests
// ──────────────────────────────────────────────────────────────────────────────

func TestSLATargetForPriority(t *testing.T) {
	targets := SLATargets{
		LowMinutes:    480,
		MediumMinutes: 120,
		HighMinutes:   30,
	}

	tests := []struct {
		name     string
		priority string
		want     time.Duration
	}{
		{"high priority", "high", 30 * time.Minute},
		{"medium priority", "medium", 120 * time.Minute},
		{"low priority", "low", 480 * time.Minute},
		{"unknown priority defaults to low", "urgent", 480 * time.Minute},
		{"empty priority defaults to low", "", 480 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slaTargetForPriority(targets, tt.priority)
			if got != tt.want {
				t.Errorf("slaTargetForPriority(%q) = %v, want %v", tt.priority, got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// CheckOverdueTickets Tests
// ──────────────────────────────────────────────────────────────────────────────

func TestCheckOverdueTickets(t *testing.T) {
	tests := []struct {
		name            string
		tickets         []overdueTicketRow
		slaSettings     [][]string // [key, value] pairs
		wantNotifyCount int
		wantAlertedIDs  []int64
	}{
		{
			name: "notifies for overdue high-priority ticket",
			tickets: []overdueTicketRow{
				{
					id:             1,
					subject:        "Server down",
					priority:       "high",
					createdAt:      time.Now().Add(-2 * time.Hour),
					customerName:   "alice",
					lastCustomerAt: time.Now().Add(-2 * time.Hour), // 2h ago, target is 30m
				},
			},
			slaSettings:     nil, // use defaults
			wantNotifyCount: 1,
			wantAlertedIDs:  []int64{1},
		},
		{
			name: "does not notify for ticket within SLA",
			tickets: []overdueTicketRow{
				{
					id:             2,
					subject:        "Minor issue",
					priority:       "low",
					createdAt:      time.Now().Add(-1 * time.Hour),
					customerName:   "bob",
					lastCustomerAt: time.Now().Add(-1 * time.Hour), // 1h ago, target is 8h
				},
			},
			slaSettings:     nil,
			wantNotifyCount: 0,
			wantAlertedIDs:  nil,
		},
		{
			name: "multiple tickets — only overdue ones get notified",
			tickets: []overdueTicketRow{
				{
					id:             3,
					subject:        "Overdue medium",
					priority:       "medium",
					createdAt:      time.Now().Add(-3 * time.Hour),
					customerName:   "charlie",
					lastCustomerAt: time.Now().Add(-3 * time.Hour), // 3h ago, target is 2h
				},
				{
					id:             4,
					subject:        "Within SLA medium",
					priority:       "medium",
					createdAt:      time.Now().Add(-30 * time.Minute),
					customerName:   "dave",
					lastCustomerAt: time.Now().Add(-30 * time.Minute), // 30m ago, target is 2h
				},
			},
			slaSettings:     nil,
			wantNotifyCount: 1,
			wantAlertedIDs:  []int64{3},
		},
		{
			name: "custom SLA targets from settings",
			tickets: []overdueTicketRow{
				{
					id:             5,
					subject:        "Custom SLA",
					priority:       "low",
					createdAt:      time.Now().Add(-2 * time.Hour),
					customerName:   "eve",
					lastCustomerAt: time.Now().Add(-2 * time.Hour), // 2h ago
				},
			},
			slaSettings: [][]string{
				{"sla_response_minutes_low", "60"}, // custom: 1h target
			},
			wantNotifyCount: 1,
			wantAlertedIDs:  []int64{5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()

			// Mock loadSLATargets query
			settingsRows := sqlmock.NewRows([]string{"setting_key", "setting_value"})
			for _, s := range tt.slaSettings {
				settingsRows.AddRow(s[0], s[1])
			}
			mock.ExpectQuery("SELECT setting_key, setting_value FROM panel_settings").
				WillReturnRows(settingsRows)

			// Mock the overdue tickets query
			ticketRows := sqlmock.NewRows([]string{
				"id", "subject", "priority", "created_at", "customer_name", "last_customer_msg",
			})
			for _, tk := range tt.tickets {
				ticketRows.AddRow(tk.id, tk.subject, tk.priority, tk.createdAt, tk.customerName, tk.lastCustomerAt)
			}
			mock.ExpectQuery("SELECT t.id, t.subject, t.priority, t.created_at").
				WillReturnRows(ticketRows)

			// Mock the UPDATE for each ticket that should be marked as alerted
			for _, id := range tt.wantAlertedIDs {
				mock.ExpectExec("UPDATE tickets SET sla_alerted_at = NOW\\(\\) WHERE id = \\?").
					WithArgs(id).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}

			var notifications []string
			notify := func(msg string) {
				notifications = append(notifications, msg)
			}

			CheckOverdueTickets(db, notify)

			if len(notifications) != tt.wantNotifyCount {
				t.Errorf("notification count = %d, want %d", len(notifications), tt.wantNotifyCount)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

func TestCheckOverdueTickets_AlreadyAlertedSkipped(t *testing.T) {
	// This test verifies that tickets with sla_alerted_at already set
	// are excluded by the WHERE clause (sla_alerted_at IS NULL).
	// Since the query filters them out, we just verify no notifications fire
	// when the query returns no rows.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	// SLA targets query
	mock.ExpectQuery("SELECT setting_key, setting_value FROM panel_settings").
		WillReturnRows(sqlmock.NewRows([]string{"setting_key", "setting_value"}))

	// Empty result set — all overdue tickets were already alerted
	mock.ExpectQuery("SELECT t.id, t.subject, t.priority, t.created_at").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "subject", "priority", "created_at", "customer_name", "last_customer_msg",
		}))

	var notifications []string
	notify := func(msg string) {
		notifications = append(notifications, msg)
	}

	CheckOverdueTickets(db, notify)

	if len(notifications) != 0 {
		t.Errorf("expected 0 notifications for already-alerted tickets, got %d", len(notifications))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestCheckOverdueTickets_NotifyCalledWithSLABreachMessage(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	// SLA targets — defaults
	mock.ExpectQuery("SELECT setting_key, setting_value FROM panel_settings").
		WillReturnRows(sqlmock.NewRows([]string{"setting_key", "setting_value"}))

	// One overdue ticket (high priority, 1 hour past the 30m target)
	ticketRows := sqlmock.NewRows([]string{
		"id", "subject", "priority", "created_at", "customer_name", "last_customer_msg",
	}).AddRow(99, "Urgent bug", "high", time.Now().Add(-2*time.Hour), "customer_x", time.Now().Add(-1*time.Hour))

	mock.ExpectQuery("SELECT t.id, t.subject, t.priority, t.created_at").
		WillReturnRows(ticketRows)
	mock.ExpectExec("UPDATE tickets SET sla_alerted_at = NOW\\(\\) WHERE id = \\?").
		WithArgs(int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	var notifications []string
	notify := func(msg string) {
		notifications = append(notifications, msg)
	}

	CheckOverdueTickets(db, notify)

	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}

	// Verify the notification message contains expected elements
	msg := notifications[0]
	expectedContains := []string{"SLA Breach", "#99", "Urgent bug", "customer_x", "HIGH"}
	for _, expected := range expectedContains {
		if !slaContains(msg, expected) {
			t.Errorf("notification message missing %q, got: %s", expected, msg)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// formatDuration Tests
// ──────────────────────────────────────────────────────────────────────────────

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		dur  time.Duration
		want string
	}{
		{"less than a minute", 30 * time.Second, "< 1m"},
		{"exactly one minute", time.Minute, "1m"},
		{"45 minutes", 45 * time.Minute, "45m"},
		{"1 hour 15 minutes", 75 * time.Minute, "1h 15m"},
		{"2 hours 0 minutes", 2 * time.Hour, "2h 0m"},
		{"3 hours 30 minutes", 210 * time.Minute, "3h 30m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.dur)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.dur, got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

type overdueTicketRow struct {
	id             int64
	subject        string
	priority       string
	createdAt      time.Time
	customerName   string
	lastCustomerAt time.Time
}

func slaContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
