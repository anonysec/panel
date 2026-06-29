package api

import (
	"net/http"
	"time"
)

// handlePortalWalletTransactions handles GET /api/portal/wallet-transactions.
// Returns up to 100 wallet transactions for the authenticated customer, ordered by created_at DESC.
func (s *Server) handlePortalWalletTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	rows, err := s.DB.Query(`
		SELECT id, amount, type, COALESCE(description, ''), created_at
		FROM wallet_transactions
		WHERE username = $1
		ORDER BY created_at DESC
		LIMIT 100
	`, username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type transaction struct {
		ID          int64   `json:"id"`
		Amount      float64 `json:"amount"`
		Type        string  `json:"type"`
		Description string  `json:"description"`
		CreatedAt   string  `json:"created_at"`
	}

	transactions := []transaction{}
	for rows.Next() {
		var t transaction
		var createdAt time.Time
		if err := rows.Scan(&t.ID, &t.Amount, &t.Type, &t.Description, &createdAt); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		t.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		transactions = append(transactions, t)
	}

	if err := rows.Err(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "transactions": transactions})
}
