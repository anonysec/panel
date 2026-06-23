//go:build !lite

package support

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ──────────────────────────────────────────────────────────────────────────────
// Ticket Lifecycle Tests: Create → Reply → Status transitions → Resolve → Rate
// ──────────────────────────────────────────────────────────────────────────────

func TestTicketLifecycle_Create(t *testing.T) {
	tests := []struct {
		name       string
		customerID int64
		subject    string
		category   string
		priority   string
		body       string
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "successful create with all fields",
			customerID: 1,
			subject:    "Cannot connect to VPN",
			category:   "technical",
			priority:   "high",
			body:       "Getting timeout errors when connecting",
			wantErr:    false,
		},
		{
			name:       "defaults category and priority when empty",
			customerID: 2,
			subject:    "Billing question",
			category:   "",
			priority:   "",
			body:       "Need invoice for last month",
			wantErr:    false,
		},
		{
			name:       "error when subject is empty",
			customerID: 1,
			subject:    "",
			category:   "general",
			priority:   "low",
			body:       "Some body text",
			wantErr:    true,
			errMsg:     "subject is required",
		},
		{
			name:       "error when body is empty",
			customerID: 1,
			subject:    "Test subject",
			category:   "general",
			priority:   "low",
			body:       "",
			wantErr:    true,
			errMsg:     "message body is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()

			svc := New(db)
			var notifications []string
			svc.SetNotify(func(msg string) { notifications = append(notifications, msg) })

			ctx := context.Background()

			if !tt.wantErr {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO tickets").
					WithArgs(tt.customerID, tt.subject, expectCategory(tt.category), expectPriority(tt.priority)).
					WillReturnResult(sqlmock.NewResult(42, 1))
				mock.ExpectQuery("SELECT COALESCE\\(username, ''\\) FROM customers WHERE id = \\?").
					WithArgs(tt.customerID).
					WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("testuser"))
				mock.ExpectExec("INSERT INTO ticket_messages").
					WithArgs(int64(42), "testuser", tt.body).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectCommit()
			}

			ticket, err := svc.Create(ctx, tt.customerID, tt.subject, tt.category, tt.priority, tt.body)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("error = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ticket == nil {
				t.Fatal("expected ticket, got nil")
			}
			if ticket.ID != 42 {
				t.Errorf("ticket.ID = %d, want 42", ticket.ID)
			}
			if ticket.Status != "open" {
				t.Errorf("ticket.Status = %q, want %q", ticket.Status, "open")
			}
			if len(notifications) != 1 {
				t.Errorf("expected 1 notification, got %d", len(notifications))
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

func TestTicketLifecycle_Reply(t *testing.T) {
	tests := []struct {
		name       string
		ticketID   int64
		senderType string
		senderName string
		body       string
		isInternal bool
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "admin reply transitions ticket to waiting",
			ticketID:   1,
			senderType: "admin",
			senderName: "admin1",
			body:       "Looking into this now",
			isInternal: false,
			wantErr:    false,
		},
		{
			name:       "customer reply transitions ticket to open",
			ticketID:   1,
			senderType: "customer",
			senderName: "john",
			body:       "Thanks, still having issues",
			isInternal: false,
			wantErr:    false,
		},
		{
			name:       "internal note does not change status",
			ticketID:   1,
			senderType: "admin",
			senderName: "admin1",
			body:       "Checking server logs",
			isInternal: true,
			wantErr:    false,
		},
		{
			name:       "error when body is empty",
			ticketID:   1,
			senderType: "admin",
			senderName: "admin1",
			body:       "",
			isInternal: false,
			wantErr:    true,
			errMsg:     "message body is required",
		},
		{
			name:       "error when sender_type is invalid",
			ticketID:   1,
			senderType: "bot",
			senderName: "bot1",
			body:       "automated reply",
			isInternal: false,
			wantErr:    true,
			errMsg:     "invalid sender_type: bot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()

			svc := New(db)
			var notifications []string
			svc.SetNotify(func(msg string) { notifications = append(notifications, msg) })

			ctx := context.Background()

			if !tt.wantErr {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO ticket_messages").
					WithArgs(tt.ticketID, tt.senderType, tt.senderName, tt.body, tt.isInternal).
					WillReturnResult(sqlmock.NewResult(10, 1))

				if !tt.isInternal {
					expectedStatus := "waiting"
					if tt.senderType == "customer" {
						expectedStatus = "open"
					}
					mock.ExpectExec("UPDATE tickets SET status = \\?, updated_at = NOW\\(\\) WHERE id = \\? AND status != 'closed'").
						WithArgs(expectedStatus, tt.ticketID).
						WillReturnResult(sqlmock.NewResult(0, 1))
				} else {
					mock.ExpectExec("UPDATE tickets SET updated_at = NOW\\(\\) WHERE id = \\?").
						WithArgs(tt.ticketID).
						WillReturnResult(sqlmock.NewResult(0, 1))
				}

				mock.ExpectCommit()
			}

			msg, err := svc.Reply(ctx, tt.ticketID, tt.senderType, tt.senderName, tt.body, tt.isInternal)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("error = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if msg == nil {
				t.Fatal("expected message, got nil")
			}
			if msg.ID != 10 {
				t.Errorf("msg.ID = %d, want 10", msg.ID)
			}
			if msg.SenderType != tt.senderType {
				t.Errorf("msg.SenderType = %q, want %q", msg.SenderType, tt.senderType)
			}

			// External replies should fire a notification
			if !tt.isInternal && len(notifications) != 1 {
				t.Errorf("expected 1 notification for external reply, got %d", len(notifications))
			}
			// Internal notes should NOT fire a notification
			if tt.isInternal && len(notifications) != 0 {
				t.Errorf("expected 0 notifications for internal note, got %d", len(notifications))
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

func TestTicketLifecycle_UpdateStatus(t *testing.T) {
	tests := []struct {
		name      string
		ticketID  int64
		newStatus string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "transition to in_progress",
			ticketID:  1,
			newStatus: "in_progress",
			wantErr:   false,
		},
		{
			name:      "transition to resolved sets resolved_at",
			ticketID:  1,
			newStatus: "resolved",
			wantErr:   false,
		},
		{
			name:      "transition to closed sets closed_at",
			ticketID:  1,
			newStatus: "closed",
			wantErr:   false,
		},
		{
			name:      "transition to waiting",
			ticketID:  1,
			newStatus: "waiting",
			wantErr:   false,
		},
		{
			name:      "invalid status returns error",
			ticketID:  1,
			newStatus: "deleted",
			wantErr:   true,
			errMsg:    "invalid status: deleted",
		},
		{
			name:      "ticket not found returns error",
			ticketID:  999,
			newStatus: "open",
			wantErr:   true,
			errMsg:    "ticket 999 not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()

			svc := New(db)
			ctx := context.Background()

			if !tt.wantErr {
				mock.ExpectExec("UPDATE tickets SET status").
					WithArgs(tt.newStatus, tt.ticketID).
					WillReturnResult(sqlmock.NewResult(0, 1))
			} else if tt.errMsg == fmt.Sprintf("ticket %d not found", tt.ticketID) {
				// Valid status but ticket not found
				mock.ExpectExec("UPDATE tickets SET status").
					WithArgs(tt.newStatus, tt.ticketID).
					WillReturnResult(sqlmock.NewResult(0, 0))
			}

			err = svc.UpdateStatus(ctx, tt.ticketID, tt.newStatus)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error %q, got nil", tt.errMsg)
				} else if err.Error() != tt.errMsg {
					t.Errorf("error = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

func TestTicketLifecycle_Rate(t *testing.T) {
	tests := []struct {
		name     string
		ticketID int64
		rating   int
		status   string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "rate resolved ticket with 5 stars",
			ticketID: 1,
			rating:   5,
			status:   "resolved",
			wantErr:  false,
		},
		{
			name:     "rate closed ticket with 3 stars",
			ticketID: 2,
			rating:   3,
			status:   "closed",
			wantErr:  false,
		},
		{
			name:     "rating below 1 returns error",
			ticketID: 1,
			rating:   0,
			status:   "",
			wantErr:  true,
			errMsg:   "rating must be between 1 and 5, got 0",
		},
		{
			name:     "rating above 5 returns error",
			ticketID: 1,
			rating:   6,
			status:   "",
			wantErr:  true,
			errMsg:   "rating must be between 1 and 5, got 6",
		},
		{
			name:     "cannot rate open ticket",
			ticketID: 1,
			rating:   4,
			status:   "open",
			wantErr:  true,
			errMsg:   "can only rate resolved or closed tickets, current status: open",
		},
		{
			name:     "cannot rate in_progress ticket",
			ticketID: 1,
			rating:   4,
			status:   "in_progress",
			wantErr:  true,
			errMsg:   "can only rate resolved or closed tickets, current status: in_progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()

			svc := New(db)
			ctx := context.Background()

			// Only set up mock for valid rating range
			if tt.rating >= 1 && tt.rating <= 5 {
				mock.ExpectQuery("SELECT status FROM tickets WHERE id = \\?").
					WithArgs(tt.ticketID).
					WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(tt.status))

				if tt.status == "resolved" || tt.status == "closed" {
					mock.ExpectExec("UPDATE tickets SET satisfaction_rating").
						WithArgs(tt.rating, tt.ticketID).
						WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}

			err = svc.Rate(ctx, tt.ticketID, tt.rating)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error %q, got nil", tt.errMsg)
				} else if err.Error() != tt.errMsg {
					t.Errorf("error = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Auto-Assign Tests: previous handler, round-robin, no admins
// ──────────────────────────────────────────────────────────────────────────────

func TestAutoAssign(t *testing.T) {
	tests := []struct {
		name         string
		ticketID     int64
		category     string
		customerID   int64
		prevAdmin    string // if non-empty, simulate a previously assigned admin
		roundRobin   string // if non-empty, simulate round-robin result
		wantAssigned string
		wantErr      bool
	}{
		{
			name:         "assigns to previous handler when available",
			ticketID:     10,
			category:     "technical",
			customerID:   1,
			prevAdmin:    "admin_alice",
			roundRobin:   "",
			wantAssigned: "admin_alice",
			wantErr:      false,
		},
		{
			name:         "falls back to round-robin when no previous handler",
			ticketID:     11,
			category:     "billing",
			customerID:   2,
			prevAdmin:    "",
			roundRobin:   "admin_bob",
			wantAssigned: "admin_bob",
			wantErr:      false,
		},
		{
			name:         "returns empty when no admins available",
			ticketID:     12,
			category:     "general",
			customerID:   3,
			prevAdmin:    "",
			roundRobin:   "",
			wantAssigned: "",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()

			svc := New(db)
			ctx := context.Background()

			// Step 1: Get customer_id for the ticket
			mock.ExpectQuery("SELECT customer_id FROM tickets WHERE id = \\?").
				WithArgs(tt.ticketID).
				WillReturnRows(sqlmock.NewRows([]string{"customer_id"}).AddRow(tt.customerID))

			// Step 2: Check previous handler
			if tt.prevAdmin != "" {
				mock.ExpectQuery("SELECT assigned_to FROM tickets").
					WithArgs(tt.customerID, tt.ticketID).
					WillReturnRows(sqlmock.NewRows([]string{"assigned_to"}).AddRow(tt.prevAdmin))
				// Step 3: Assign to previous handler
				mock.ExpectExec("UPDATE tickets SET assigned_to = \\?, updated_at = NOW\\(\\) WHERE id = \\?").
					WithArgs(tt.prevAdmin, tt.ticketID).
					WillReturnResult(sqlmock.NewResult(0, 1))
			} else {
				// No previous handler found
				mock.ExpectQuery("SELECT assigned_to FROM tickets").
					WithArgs(tt.customerID, tt.ticketID).
					WillReturnError(sql.ErrNoRows)

				// Step 3: Round-robin
				if tt.roundRobin != "" {
					mock.ExpectQuery("SELECT a.username FROM admins").
						WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow(tt.roundRobin))
					mock.ExpectExec("UPDATE tickets SET assigned_to = \\?, updated_at = NOW\\(\\) WHERE id = \\?").
						WithArgs(tt.roundRobin, tt.ticketID).
						WillReturnResult(sqlmock.NewResult(0, 1))
				} else {
					// No admins available
					mock.ExpectQuery("SELECT a.username FROM admins").
						WillReturnError(sql.ErrNoRows)
				}
			}

			assigned, err := svc.AutoAssign(ctx, tt.ticketID, tt.category)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if assigned != tt.wantAssigned {
				t.Errorf("assigned = %q, want %q", assigned, tt.wantAssigned)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Notification Trigger Tests
// ──────────────────────────────────────────────────────────────────────────────

func TestNotificationTriggers(t *testing.T) {
	t.Run("Create fires notification", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to open sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)
		var notifications []string
		svc.SetNotify(func(msg string) { notifications = append(notifications, msg) })

		ctx := context.Background()

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO tickets").
			WithArgs(int64(1), "Test", "general", "medium").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectQuery("SELECT COALESCE\\(username, ''\\) FROM customers WHERE id = \\?").
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("customer1"))
		mock.ExpectExec("INSERT INTO ticket_messages").
			WithArgs(int64(1), "customer1", "Body text").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		_, err = svc.Create(ctx, 1, "Test", "general", "medium", "Body text")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(notifications) != 1 {
			t.Fatalf("expected 1 notification on create, got %d", len(notifications))
		}
		if notifications[0] == "" {
			t.Error("notification message should not be empty")
		}
	})

	t.Run("Reply fires notification for external messages", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to open sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)
		var notifications []string
		svc.SetNotify(func(msg string) { notifications = append(notifications, msg) })

		ctx := context.Background()

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO ticket_messages").
			WithArgs(int64(5), "customer", "john", "Help please", false).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("UPDATE tickets SET status").
			WithArgs("open", int64(5)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		_, err = svc.Reply(ctx, 5, "customer", "john", "Help please", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(notifications) != 1 {
			t.Fatalf("expected 1 notification on reply, got %d", len(notifications))
		}
	})

	t.Run("Internal note does NOT fire notification", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to open sqlmock: %v", err)
		}
		defer db.Close()

		svc := New(db)
		var notifications []string
		svc.SetNotify(func(msg string) { notifications = append(notifications, msg) })

		ctx := context.Background()

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO ticket_messages").
			WithArgs(int64(5), "admin", "admin1", "Internal note", true).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("UPDATE tickets SET updated_at").
			WithArgs(int64(5)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		_, err = svc.Reply(ctx, 5, "admin", "admin1", "Internal note", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(notifications) != 0 {
			t.Errorf("expected 0 notifications for internal note, got %d", len(notifications))
		}
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Get and List Tests
// ──────────────────────────────────────────────────────────────────────────────

func TestTicketGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	svc := New(db)
	ctx := context.Background()

	now := time.Now()
	resolvedAt := now.Add(-1 * time.Hour)

	mock.ExpectQuery("SELECT id, customer_id, subject, category, priority, status").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "customer_id", "subject", "category", "priority", "status",
			"assigned_to", "satisfaction_rating", "created_at", "updated_at", "resolved_at", "closed_at",
		}).AddRow(42, 1, "Test ticket", "technical", "high", "resolved",
			"admin1", 5, now, now, resolvedAt, nil))

	ticket, err := svc.Get(ctx, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ticket.ID != 42 {
		t.Errorf("ticket.ID = %d, want 42", ticket.ID)
	}
	if ticket.AssignedTo != "admin1" {
		t.Errorf("ticket.AssignedTo = %q, want %q", ticket.AssignedTo, "admin1")
	}
	if ticket.SatisfactionRating == nil || *ticket.SatisfactionRating != 5 {
		t.Errorf("ticket.SatisfactionRating = %v, want 5", ticket.SatisfactionRating)
	}
	if ticket.ResolvedAt == nil {
		t.Error("ticket.ResolvedAt should not be nil")
	}
}

func TestTicketList(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	svc := New(db)
	ctx := context.Background()

	now := time.Now()

	// Count query
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM tickets").
		WithArgs("open").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	// List query
	mock.ExpectQuery("SELECT id, customer_id, subject, category, priority, status").
		WithArgs("open", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "customer_id", "subject", "category", "priority", "status",
			"assigned_to", "satisfaction_rating", "created_at", "updated_at", "resolved_at", "closed_at",
		}).
			AddRow(1, 1, "Ticket A", "technical", "high", "open", nil, nil, now, now, nil, nil).
			AddRow(2, 2, "Ticket B", "billing", "low", "open", "admin1", nil, now, now, nil, nil))

	filter := ListFilter{
		Status: "open",
		Limit:  20,
		Offset: 0,
	}
	tickets, total, err := svc.List(ctx, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(tickets) != 2 {
		t.Errorf("len(tickets) = %d, want 2", len(tickets))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

func expectCategory(cat string) string {
	if cat == "" {
		return "general"
	}
	return cat
}

func expectPriority(pri string) string {
	if pri == "" {
		return "normal"
	}
	return pri
}
