//go:build !lite

// Package support provides the ticket-based support system for KorisPanel.
// It handles ticket CRUD, conversation threads, file attachments,
// auto-assignment, and customer satisfaction ratings.
package support

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Ticket status constants.
const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusWaiting    = "waiting"
	StatusResolved   = "resolved"
	StatusClosed     = "closed"
)

// Ticket category constants.
const (
	CategoryBilling   = "billing"
	CategoryTechnical = "technical"
	CategoryGeneral   = "general"
)

// Ticket priority constants.
const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

// Sender type constants.
const (
	SenderCustomer = "customer"
	SenderAdmin    = "admin"
)

// Ticket represents a customer support ticket.
type Ticket struct {
	ID                 int64      `json:"id"`
	CustomerID         int64      `json:"customer_id"`
	Subject            string     `json:"subject"`
	Category           string     `json:"category"`            // billing, technical, general
	Priority           string     `json:"priority"`            // low, medium, high
	Status             string     `json:"status"`              // open, in_progress, waiting, resolved, closed
	AssignedTo         string     `json:"assigned_to"`         // admin username or empty
	SatisfactionRating *int       `json:"satisfaction_rating"` // 1-5 after resolution
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`
	ClosedAt           *time.Time `json:"closed_at,omitempty"`
}

// Message represents a single message within a ticket conversation thread.
type Message struct {
	ID         int64     `json:"id"`
	TicketID   int64     `json:"ticket_id"`
	SenderType string    `json:"sender_type"` // customer, admin
	SenderName string    `json:"sender_name"`
	Body       string    `json:"body"`
	IsInternal bool      `json:"is_internal"` // internal notes not visible to customer
	CreatedAt  time.Time `json:"created_at"`
}

// Attachment represents a file attached to a message.
type Attachment struct {
	ID        int64     `json:"id"`
	MessageID int64     `json:"message_id"`
	Filename  string    `json:"filename"`
	FilePath  string    `json:"file_path"`
	FileSize  int       `json:"file_size"`
	MimeType  string    `json:"mime_type"`
	CreatedAt time.Time `json:"created_at"`
}

// ListFilter defines filters for listing tickets.
type ListFilter struct {
	Status     string // filter by status (empty = all)
	Category   string // filter by category (empty = all)
	Priority   string // filter by priority (empty = all)
	CustomerID int64  // filter by customer (0 = all)
	AssignedTo string // filter by assignee (empty = all)
	Limit      int
	Offset     int
}

// TicketService provides support ticket operations backed by MariaDB.
type TicketService struct {
	db     *sql.DB
	notify func(msg string)
}

// New creates a new TicketService with the given database connection.
func New(db *sql.DB) *TicketService {
	return &TicketService{
		db:     db,
		notify: func(msg string) { log.Printf("[support] %s", msg) },
	}
}

// SetNotify sets a custom notification function for support events.
func (s *TicketService) SetNotify(fn func(msg string)) {
	if fn != nil {
		s.notify = fn
	}
}

// Create opens a new ticket and inserts the initial message (ticket body).
// Returns the created ticket with populated ID and timestamps.
func (s *TicketService) Create(ctx context.Context, customerID int64, subject, category, priority, body string) (*Ticket, error) {
	if subject == "" {
		return nil, fmt.Errorf("subject is required")
	}
	if body == "" {
		return nil, fmt.Errorf("message body is required")
	}
	if category == "" {
		category = "general"
	}
	if priority == "" {
		priority = "normal"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Insert ticket
	result, err := tx.ExecContext(ctx, `
		INSERT INTO tickets (customer_id, subject, category, priority, status)
		VALUES (?, ?, ?, ?, 'open')`,
		customerID, subject, category, priority,
	)
	if err != nil {
		return nil, fmt.Errorf("insert ticket: %w", err)
	}

	ticketID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get ticket id: %w", err)
	}

	// Fetch customer name for sender_name
	var senderName string
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(username, '') FROM customers WHERE id = ?`, customerID).Scan(&senderName)
	if err != nil {
		senderName = fmt.Sprintf("customer_%d", customerID)
	}

	// Insert initial message
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ticket_messages (ticket_id, sender_type, sender_name, body, is_internal)
		VALUES (?, 'customer', ?, ?, FALSE)`,
		ticketID, senderName, body,
	)
	if err != nil {
		return nil, fmt.Errorf("insert initial message: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	ticket := &Ticket{
		ID:         ticketID,
		CustomerID: customerID,
		Subject:    subject,
		Category:   category,
		Priority:   priority,
		Status:     "open",
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	s.notify(fmt.Sprintf("new ticket #%d from customer %d: %s [%s/%s]",
		ticketID, customerID, subject, category, priority))
	log.Printf("[support] created ticket #%d for customer %d", ticketID, customerID)
	return ticket, nil
}

// Get retrieves a single ticket by ID.
func (s *TicketService) Get(ctx context.Context, ticketID int64) (*Ticket, error) {
	t := &Ticket{}
	var assignedTo sql.NullString
	var rating sql.NullInt64
	var resolvedAt, closedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, customer_id, subject, category, priority, status,
		       assigned_to, satisfaction_rating, created_at, updated_at, resolved_at, closed_at
		FROM tickets WHERE id = ?`, ticketID,
	).Scan(
		&t.ID, &t.CustomerID, &t.Subject, &t.Category, &t.Priority, &t.Status,
		&assignedTo, &rating, &t.CreatedAt, &t.UpdatedAt, &resolvedAt, &closedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get ticket %d: %w", ticketID, err)
	}

	if assignedTo.Valid {
		t.AssignedTo = assignedTo.String
	}
	if rating.Valid {
		r := int(rating.Int64)
		t.SatisfactionRating = &r
	}
	if resolvedAt.Valid {
		t.ResolvedAt = &resolvedAt.Time
	}
	if closedAt.Valid {
		t.ClosedAt = &closedAt.Time
	}

	return t, nil
}

// List retrieves tickets matching the given filters, returning the list and total count.
func (s *TicketService) List(ctx context.Context, f ListFilter) ([]Ticket, int, error) {
	if f.Limit <= 0 {
		f.Limit = 20
	}

	// Build WHERE clause
	where := "WHERE 1=1"
	args := []any{}

	if f.Status != "" {
		where += " AND status = ?"
		args = append(args, f.Status)
	}
	if f.Category != "" {
		where += " AND category = ?"
		args = append(args, f.Category)
	}
	if f.Priority != "" {
		where += " AND priority = ?"
		args = append(args, f.Priority)
	}
	if f.CustomerID > 0 {
		where += " AND customer_id = ?"
		args = append(args, f.CustomerID)
	}
	if f.AssignedTo != "" {
		where += " AND assigned_to = ?"
		args = append(args, f.AssignedTo)
	}

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) FROM tickets " + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tickets: %w", err)
	}

	// Fetch page
	query := fmt.Sprintf(`
		SELECT id, customer_id, subject, category, priority, status,
		       assigned_to, satisfaction_rating, created_at, updated_at, resolved_at, closed_at
		FROM tickets %s
		ORDER BY updated_at DESC LIMIT ? OFFSET ?`, where)
	args = append(args, f.Limit, f.Offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list tickets: %w", err)
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		var assignedTo sql.NullString
		var rating sql.NullInt64
		var resolvedAt, closedAt sql.NullTime

		if err := rows.Scan(
			&t.ID, &t.CustomerID, &t.Subject, &t.Category, &t.Priority, &t.Status,
			&assignedTo, &rating, &t.CreatedAt, &t.UpdatedAt, &resolvedAt, &closedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan ticket row: %w", err)
		}

		if assignedTo.Valid {
			t.AssignedTo = assignedTo.String
		}
		if rating.Valid {
			r := int(rating.Int64)
			t.SatisfactionRating = &r
		}
		if resolvedAt.Valid {
			t.ResolvedAt = &resolvedAt.Time
		}
		if closedAt.Valid {
			t.ClosedAt = &closedAt.Time
		}

		tickets = append(tickets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate tickets: %w", err)
	}

	return tickets, total, nil
}

// UpdateStatus transitions a ticket to a new status with proper timestamp updates.
// Valid transitions: open→in_progress, *→waiting, *→resolved, *→closed.
func (s *TicketService) UpdateStatus(ctx context.Context, ticketID int64, newStatus string) error {
	// Validate status value
	switch newStatus {
	case "open", "in_progress", "waiting", "resolved", "closed":
		// valid
	default:
		return fmt.Errorf("invalid status: %s", newStatus)
	}

	var query string
	switch newStatus {
	case "resolved":
		query = `UPDATE tickets SET status = ?, resolved_at = NOW(), updated_at = NOW() WHERE id = ?`
	case "closed":
		query = `UPDATE tickets SET status = ?, closed_at = NOW(), updated_at = NOW() WHERE id = ?`
	default:
		query = `UPDATE tickets SET status = ?, updated_at = NOW() WHERE id = ?`
	}

	result, err := s.db.ExecContext(ctx, query, newStatus, ticketID)
	if err != nil {
		return fmt.Errorf("update ticket status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("ticket %d not found", ticketID)
	}

	log.Printf("[support] ticket #%d status changed to %s", ticketID, newStatus)
	return nil
}

// Reply adds a message to the ticket conversation thread.
// Updates the ticket's updated_at timestamp and sets status to "waiting" if admin replies,
// or "open" if customer replies to a waiting ticket.
func (s *TicketService) Reply(ctx context.Context, ticketID int64, senderType, senderName, body string, isInternal bool) (*Message, error) {
	if body == "" {
		return nil, fmt.Errorf("message body is required")
	}
	if senderType != "customer" && senderType != "admin" {
		return nil, fmt.Errorf("invalid sender_type: %s", senderType)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Insert message
	result, err := tx.ExecContext(ctx, `
		INSERT INTO ticket_messages (ticket_id, sender_type, sender_name, body, is_internal)
		VALUES (?, ?, ?, ?, ?)`,
		ticketID, senderType, senderName, body, isInternal,
	)
	if err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}

	msgID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get message id: %w", err)
	}

	// Update ticket status based on who replied (skip for internal notes)
	if !isInternal {
		var statusUpdate string
		if senderType == "admin" {
			statusUpdate = "waiting"
		} else {
			// Customer replied — reopen if it was waiting
			statusUpdate = "open"
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE tickets SET status = ?, updated_at = NOW() WHERE id = ? AND status != 'closed'`,
			statusUpdate, ticketID,
		)
		if err != nil {
			return nil, fmt.Errorf("update ticket status on reply: %w", err)
		}
	} else {
		// Just update timestamp for internal notes
		_, err = tx.ExecContext(ctx, `UPDATE tickets SET updated_at = NOW() WHERE id = ?`, ticketID)
		if err != nil {
			return nil, fmt.Errorf("update ticket timestamp: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	msg := &Message{
		ID:         msgID,
		TicketID:   ticketID,
		SenderType: senderType,
		SenderName: senderName,
		Body:       body,
		IsInternal: isInternal,
		CreatedAt:  time.Now().UTC(),
	}

	if !isInternal {
		s.notify(fmt.Sprintf("ticket #%d reply from %s (%s)", ticketID, senderName, senderType))
	}
	log.Printf("[support] message added to ticket #%d by %s (%s, internal=%v)",
		ticketID, senderName, senderType, isInternal)
	return msg, nil
}

// GetMessages retrieves all messages for a ticket, ordered by creation time ascending.
func (s *TicketService) GetMessages(ctx context.Context, ticketID int64) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, ticket_id, sender_type, sender_name, body, is_internal, created_at
		FROM ticket_messages
		WHERE ticket_id = ?
		ORDER BY created_at ASC`, ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("get messages for ticket %d: %w", ticketID, err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.TicketID, &m.SenderType, &m.SenderName, &m.Body, &m.IsInternal, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message row: %w", err)
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	return messages, nil
}

// Rate allows a customer to rate their support experience (1-5 stars).
// Only applicable to tickets with status "resolved" or "closed".
func (s *TicketService) Rate(ctx context.Context, ticketID int64, rating int) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be between 1 and 5, got %d", rating)
	}

	// Verify ticket is resolved or closed
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM tickets WHERE id = ?`, ticketID).Scan(&status)
	if err != nil {
		return fmt.Errorf("get ticket status: %w", err)
	}
	if status != "resolved" && status != "closed" {
		return fmt.Errorf("can only rate resolved or closed tickets, current status: %s", status)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE tickets SET satisfaction_rating = ? WHERE id = ?`, rating, ticketID,
	)
	if err != nil {
		return fmt.Errorf("update rating: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("ticket %d not found", ticketID)
	}

	log.Printf("[support] ticket #%d rated %d/5", ticketID, rating)
	return nil
}

// AutoAssign assigns a ticket to an admin based on assignment rules:
// 1. If the customer had a previous ticket, assign to the same admin.
// 2. Otherwise, round-robin among admins.
// Returns the assigned admin username.
func (s *TicketService) AutoAssign(ctx context.Context, ticketID int64, category string) (string, error) {
	// Get the customer for this ticket
	var customerID int64
	err := s.db.QueryRowContext(ctx, `SELECT customer_id FROM tickets WHERE id = ?`, ticketID).Scan(&customerID)
	if err != nil {
		return "", fmt.Errorf("get ticket customer: %w", err)
	}

	// Strategy 1: Check if this customer had a previous ticket with an assigned admin
	var prevAdmin sql.NullString
	err = s.db.QueryRowContext(ctx, `
		SELECT assigned_to FROM tickets
		WHERE customer_id = ? AND id != ? AND assigned_to IS NOT NULL AND assigned_to != ''
		ORDER BY updated_at DESC LIMIT 1`,
		customerID, ticketID,
	).Scan(&prevAdmin)
	if err == nil && prevAdmin.Valid && prevAdmin.String != "" {
		// Assign to previous handler
		_, err = s.db.ExecContext(ctx, `
			UPDATE tickets SET assigned_to = ?, updated_at = NOW() WHERE id = ?`,
			prevAdmin.String, ticketID,
		)
		if err != nil {
			return "", fmt.Errorf("assign to previous admin: %w", err)
		}
		log.Printf("[support] ticket #%d auto-assigned to %s (previous handler)", ticketID, prevAdmin.String)
		return prevAdmin.String, nil
	}

	// Strategy 2: Round-robin — find admin with fewest open tickets
	var admin sql.NullString
	err = s.db.QueryRowContext(ctx, `
		SELECT a.username FROM admins a
		LEFT JOIN tickets t ON t.assigned_to = a.username AND t.status IN ('open','in_progress','waiting')
		GROUP BY a.username
		ORDER BY COUNT(t.id) ASC
		LIMIT 1`,
	).Scan(&admin)
	if err != nil || !admin.Valid || admin.String == "" {
		// No admins available — leave unassigned
		log.Printf("[support] ticket #%d: no admin available for auto-assign", ticketID)
		return "", nil
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE tickets SET assigned_to = ?, updated_at = NOW() WHERE id = ?`,
		admin.String, ticketID,
	)
	if err != nil {
		return "", fmt.Errorf("assign via round-robin: %w", err)
	}

	log.Printf("[support] ticket #%d auto-assigned to %s (round-robin)", ticketID, admin.String)
	return admin.String, nil
}
