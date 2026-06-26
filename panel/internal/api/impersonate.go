package api

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"time"

	"KorisPanel/panel/internal/auth"
)

const impersonateSessionTTL = 30 * time.Minute

// adminImpersonateCustomer handles POST /api/admin/customers/:id/impersonate.
// It creates a temporary portal session token for the given customer, allowing
// the admin to view the portal as that customer. The session is short-lived (30 min)
// and marked as impersonated in the audit log.
func (s *Server) adminImpersonateCustomer(w http.ResponseWriter, r *http.Request, customerID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	actor, _, _ := s.currentAdmin(r)
	ip := clientIP(r)

	// Look up customer — must exist and not be deleted
	var username string
	var deletedAt sql.NullTime
	err := s.DB.QueryRow(
		`SELECT username, deleted_at FROM customers WHERE id=$1 LIMIT 1`,
		customerID,
	).Scan(&username, &deletedAt)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		log.Printf("[impersonate] db error looking up customer %d: %v", customerID, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "internal"})
		return
	}
	if deletedAt.Valid {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "customer_deleted"})
		return
	}

	// Create a short-lived portal session token for this customer
	token := auth.MakeSession(username, s.Config.SessionSecret, impersonateSessionTTL)

	portalURL := "/portal/?token=" + token
	expiresIn := int(impersonateSessionTTL.Seconds())

	// Audit log
	s.logAudit(actor, "customer.impersonated", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{
		"username":   username,
		"expires_in": expiresIn,
	}, ip)

	log.Printf("[impersonate] admin=%s impersonated customer=%s (id=%d) ip=%s", actor, username, customerID, ip)

	writeJSON(w, map[string]any{
		"ok":         true,
		"portal_url": portalURL,
		"expires_in": expiresIn,
	})
}

// portalImpersonateLogin handles GET /api/portal/impersonate-login?token=...
// It validates the signed session token and sets the customer session cookie.
// This allows the portal frontend to redirect here and get an authenticated session.
func (s *Server) portalImpersonateLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "token_required"})
		return
	}

	// Validate the token (same HMAC-signed format as session cookie)
	username, ok := auth.ValidateToken(token, s.Config.SessionSecret)
	if !ok || username == "" {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid_token"})
		return
	}

	// Verify the customer still exists and is not deleted/disabled
	var status string
	err := s.DB.QueryRow(`SELECT status FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, username).Scan(&status)
	if err != nil {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid_token"})
		return
	}

	// Set the customer session cookie with the impersonation TTL
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CustomerCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.Config.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(impersonateSessionTTL),
	})

	writeJSON(w, map[string]any{"ok": true, "username": username})
}
