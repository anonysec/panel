//go:build !lite

package billing

import (
	"context"
	"fmt"
	"log"
	"time"
)

// RunAutoRenewalCheck finds customers whose subscriptions expire within 24 hours
// and have auto-renewal enabled, then processes renewal for each by deducting
// from their wallet. Errors are logged but do not halt processing of other customers.
func (b *BillingEngine) RunAutoRenewalCheck(ctx context.Context) error {
	rows, err := b.db.QueryContext(ctx, `
		SELECT DISTINCT c.id
		FROM customers c
		JOIN subscriptions s ON s.customer_id = c.id AND s.status = 'active'
		WHERE c.status = 'active'
		  AND c.auto_renew = 1
		  AND c.deleted_at IS NULL
		  AND s.expires_at IS NOT NULL
		  AND s.expires_at <= NOW() + INTERVAL 24 HOUR
		  AND s.expires_at > NOW()
	`)
	if err != nil {
		return fmt.Errorf("query auto-renewal candidates: %w", err)
	}
	defer rows.Close()

	var customerIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			log.Printf("[billing] scan auto-renewal candidate: %v", err)
			continue
		}
		customerIDs = append(customerIDs, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate auto-renewal candidates: %w", err)
	}

	for _, customerID := range customerIDs {
		if err := b.ProcessAutoRenewal(ctx, customerID); err != nil {
			log.Printf("[billing] auto-renewal failed for customer %d: %v", customerID, err)
			continue
		}
	}

	if len(customerIDs) > 0 {
		log.Printf("[billing] auto-renewal check complete: %d candidates processed", len(customerIDs))
	}
	return nil
}

// SendExpiryWarnings sends Telegram notifications to customers whose subscriptions
// are expiring within 48h or 24h but have not yet been renewed. It avoids duplicate
// notifications by checking the events table for recent warnings.
func (b *BillingEngine) SendExpiryWarnings(ctx context.Context) error {
	// Send 48h warnings
	if err := b.sendWarningsForWindow(ctx, 48, "48h"); err != nil {
		log.Printf("[billing] 48h expiry warning error: %v", err)
	}

	// Send 24h warnings
	if err := b.sendWarningsForWindow(ctx, 24, "24h"); err != nil {
		log.Printf("[billing] 24h expiry warning error: %v", err)
	}

	return nil
}

// sendWarningsForWindow queries customers expiring within the given hours window
// and sends a notification if one hasn't been sent already today.
func (b *BillingEngine) sendWarningsForWindow(ctx context.Context, hours int, label string) error {
	rows, err := b.db.QueryContext(ctx, `
		SELECT DISTINCT c.id, c.username, s.expires_at
		FROM customers c
		JOIN subscriptions s ON s.customer_id = c.id AND s.status = 'active'
		WHERE c.status = 'active'
		  AND c.deleted_at IS NULL
		  AND s.expires_at IS NOT NULL
		  AND s.expires_at <= NOW() + INTERVAL ? HOUR
		  AND s.expires_at > NOW()
	`, hours)
	if err != nil {
		return fmt.Errorf("query %s expiry candidates: %w", label, err)
	}
	defer rows.Close()

	type candidate struct {
		ID        int64
		Username  string
		ExpiresAt time.Time
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.ID, &c.Username, &c.ExpiresAt); err != nil {
			log.Printf("[billing] scan %s expiry candidate: %v", label, err)
			continue
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate %s expiry candidates: %w", label, err)
	}

	for _, c := range candidates {
		// Check if we already sent this warning today
		var alreadySent int
		err := b.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM events
			WHERE related = ? AND type = 'expiry_warning'
			  AND title LIKE ?
			  AND created_at > NOW() - INTERVAL 1 DAY
		`, c.Username, fmt.Sprintf("%%%s%%", label)).Scan(&alreadySent)
		if err != nil {
			log.Printf("[billing] check existing %s warning for %s: %v", label, c.Username, err)
			continue
		}
		if alreadySent > 0 {
			continue
		}

		// Send notification
		remaining := time.Until(c.ExpiresAt).Round(time.Hour)
		msg := fmt.Sprintf("⏰ Subscription expiring in ~%s: %s (expires %s)",
			label, c.Username, c.ExpiresAt.Format("2006-01-02 15:04"))
		b.notify(msg)

		// Record the event to prevent duplicate warnings
		_, _ = b.db.ExecContext(ctx, `
			INSERT INTO events (type, severity, title, message, actor, related)
			VALUES ('expiry_warning', 'warning', ?, ?, 'system', ?)
		`, fmt.Sprintf("%s expiry warning: %s", label, c.Username),
			fmt.Sprintf("Subscription expires in %v", remaining),
			c.Username)
	}

	return nil
}
