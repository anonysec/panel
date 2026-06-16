package csrf

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// Session cookie names matching the auth package constants.
const (
	adminCookieName    = "koris_admin_session"
	customerCookieName = "koris_customer_session"
)

// Middleware returns an http.Handler that validates CSRF tokens on
// state-changing requests (POST, PUT, PATCH, DELETE).
// Safe methods (GET, HEAD, OPTIONS) pass through and receive a fresh token.
// Token is delivered via X-CSRF-Token response header and validated
// from X-CSRF-Token request header.
// Exempt paths (/api/node/* and /api/bot/webhook) bypass CSRF validation entirely.
func Middleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt paths bypass CSRF entirely
		if isExempt(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Generate token from session cookie
		token := generateToken(r, secret)

		// Always set the response header when we have a token
		if token != "" {
			w.Header().Set("X-CSRF-Token", token)
		}

		// Safe methods pass through
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// State-changing methods require valid token
		provided := r.Header.Get("X-CSRF-Token")
		if token == "" || !validateToken(provided, token) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "csrf_invalid"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isExempt returns true for paths that bypass CSRF validation.
// Exempt: all paths starting with /api/node/, exact path /api/bot/webhook,
// all /api/auth/* paths (protected by credentials, not CSRF), and
// all /api/setup/* paths (protected by setup key).
func isExempt(path string) bool {
	// Node API and bot webhook bypass CSRF entirely
	if strings.HasPrefix(path, "/api/node/") || path == "/api/bot/webhook" {
		return true
	}
	// ALL auth endpoints are exempt — they're protected by credentials, not CSRF
	if strings.HasPrefix(path, "/api/auth/") || strings.HasPrefix(path, "/api/setup/") {
		return true
	}
	return false
}

// isSafeMethod returns true for HTTP methods that do not require CSRF validation.
func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

// generateToken computes an HMAC-SHA256 of the session cookie value with the
// secret, returned as a base64url-encoded string (no padding).
func generateToken(r *http.Request, secret string) string {
	sessionValue := extractSessionCookie(r)
	if sessionValue == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sessionValue))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// extractSessionCookie tries to read the admin session cookie first,
// then falls back to the customer session cookie.
func extractSessionCookie(r *http.Request) string {
	if c, err := r.Cookie(adminCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	if c, err := r.Cookie(customerCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

// validateToken performs a constant-time comparison of the provided token
// against the expected token.
func validateToken(provided, expected string) bool {
	if provided == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}
