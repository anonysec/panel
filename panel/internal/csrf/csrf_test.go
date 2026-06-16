package csrf

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testSecret = "test-csrf-secret-at-least-32-chars-long"

// computeExpectedToken is a helper that computes the expected CSRF token
// for a given session value and secret.
func computeExpectedToken(sessionValue, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sessionValue))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// okHandler is a simple handler that returns 200 OK.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
})

func TestTokenGeneration_GET(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	sessionValue := "admin-session-value.123456.signature"
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: sessionValue})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	token := rr.Header().Get("X-CSRF-Token")
	if token == "" {
		t.Fatal("expected X-CSRF-Token response header to be set")
	}

	expected := computeExpectedToken(sessionValue, testSecret)
	if token != expected {
		t.Fatalf("token mismatch: got %q, want %q", token, expected)
	}
}

func TestTokenGeneration_CustomerCookie(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	req := httptest.NewRequest(http.MethodGet, "/portal", nil)
	sessionValue := "customer-session-value.789.sig"
	req.AddCookie(&http.Cookie{Name: customerCookieName, Value: sessionValue})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	token := rr.Header().Get("X-CSRF-Token")
	expected := computeExpectedToken(sessionValue, testSecret)
	if token != expected {
		t.Fatalf("token mismatch: got %q, want %q", token, expected)
	}
}

func TestTokenGeneration_NoSession(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	// No session cookie

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	token := rr.Header().Get("X-CSRF-Token")
	if token != "" {
		t.Fatalf("expected no X-CSRF-Token header when no session, got %q", token)
	}
}

func TestSafeMethods_PassThrough(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	methods := []string{http.MethodGet, http.MethodHead, http.MethodOptions}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/something", nil)
			req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "session-val"})

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200 for %s, got %d", method, rr.Code)
			}
		})
	}
}

func TestStateChangingMethods_ValidToken(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	sessionValue := "my-session.12345.sig"
	token := computeExpectedToken(sessionValue, testSecret)

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/customers", nil)
			req.AddCookie(&http.Cookie{Name: adminCookieName, Value: sessionValue})
			req.Header.Set("X-CSRF-Token", token)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200 for %s with valid token, got %d", method, rr.Code)
			}
		})
	}
}

func TestStateChangingMethods_MissingToken(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/customers", nil)
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "session-val"})
	// No X-CSRF-Token header

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["ok"] != false {
		t.Fatalf("expected ok=false, got %v", resp["ok"])
	}
	if resp["error"] != "csrf_invalid" {
		t.Fatalf("expected error=csrf_invalid, got %v", resp["error"])
	}
}

func TestStateChangingMethods_InvalidToken(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/customers", nil)
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "session-val"})
	req.Header.Set("X-CSRF-Token", "invalid-token-value")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "csrf_invalid" {
		t.Fatalf("expected error=csrf_invalid, got %v", resp["error"])
	}
}

func TestStateChangingMethods_NoSession(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/customers", nil)
	// No session cookie, no CSRF token

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when no session, got %d", rr.Code)
	}
}

func TestExemptPaths_NodeAPI(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	paths := []string{
		"/api/node/heartbeat",
		"/api/node/status",
		"/api/node/tasks",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			// No CSRF token, no session cookie

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200 for exempt path %s, got %d", path, rr.Code)
			}
		})
	}
}

func TestExemptPaths_BotWebhook(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/bot/webhook", nil)
	// No CSRF token, no session cookie

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for exempt /api/bot/webhook, got %d", rr.Code)
	}
}

func TestExemptPaths_AuthEndpoints(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	// All /api/auth/* and /api/setup/* paths are exempt
	paths := []string{
		"/api/auth/admin",
		"/api/auth/customer",
		"/api/auth/logout",
		"/api/auth/customer/logout",
		"/api/auth/me",
		"/api/setup/owner",
		"/api/setup/owner/something",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			// No CSRF token, no session cookie — auth endpoints are exempt

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200 for exempt auth path %s, got %d", path, rr.Code)
			}
		})
	}
}

func TestExemptPaths_NonAuthPathsNotExempt(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	// Paths outside /api/auth/ and /api/setup/ should NOT be exempt
	paths := []string{
		"/api/customers",
		"/api/servers",
		"/api/plans",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "session-val"})
			// No CSRF token

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Fatalf("expected 403 for non-exempt path %s, got %d", path, rr.Code)
			}
		})
	}
}

func TestExemptPaths_NonExempt(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	// /api/bot/webhook/extra should NOT be exempt (not exact match)
	req := httptest.NewRequest(http.MethodPost, "/api/bot/webhook/extra", nil)
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "session-val"})
	// No CSRF token

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-exempt /api/bot/webhook/extra, got %d", rr.Code)
	}
}

func TestAdminCookiePriority(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	adminSession := "admin-session-value"
	customerSession := "customer-session-value"

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: adminSession})
	req.AddCookie(&http.Cookie{Name: customerCookieName, Value: customerSession})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	token := rr.Header().Get("X-CSRF-Token")
	expectedAdmin := computeExpectedToken(adminSession, testSecret)
	if token != expectedAdmin {
		t.Fatalf("expected admin cookie to take priority, got token for different session")
	}
}

func TestResponseHeader_ContentType(t *testing.T) {
	handler := Middleware(testSecret, okHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/customers", nil)
	req.AddCookie(&http.Cookie{Name: adminCookieName, Value: "session-val"})
	// No CSRF token - should get 403

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json on rejection, got %q", ct)
	}
}
