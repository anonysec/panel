package sessions

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"KorisPanel/panel/internal/dbstore"
)

// mockStore is an in-memory mock implementation of dbstore.Store for testing.
// Only session methods are implemented; others panic.
type mockStore struct {
	mu       sync.Mutex
	sessions map[string]*dbstore.Session
	cleaned  int
}

func newMockStore() *mockStore {
	return &mockStore{sessions: make(map[string]*dbstore.Session)}
}

func (m *mockStore) DB() *sql.DB                                          { return nil }
func (m *mockStore) Close() error                                         { return nil }
func (m *mockStore) Ping(_ context.Context) error                         { return nil }
func (m *mockStore) Migrate(_ context.Context, _ string) error            { return nil }
func (m *mockStore) Begin(_ context.Context) (dbstore.Tx, error)          { return nil, nil }
func (m *mockStore) AcquireLock(_ context.Context, _ int64) (bool, error) { return true, nil }
func (m *mockStore) ReleaseLock(_ context.Context, _ int64) error         { return nil }
func (m *mockStore) InsertMetrics(_ context.Context, _ int64, _ *dbstore.MetricsRow) error {
	return nil
}
func (m *mockStore) InsertTrafficLog(_ context.Context, _ *dbstore.TrafficLogEntry) error {
	return nil
}
func (m *mockStore) QueryMetrics(_ context.Context, _ int64, _, _ time.Time) ([]dbstore.MetricsRow, error) {
	return nil, nil
}

func (m *mockStore) GetSession(_ context.Context, token string) (*dbstore.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok {
		return nil, nil
	}
	// Return a copy
	cp := *s
	return &cp, nil
}

func (m *mockStore) SaveSession(_ context.Context, s *dbstore.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *s
	m.sessions[s.Token] = &cp
	return nil
}

func (m *mockStore) DeleteSession(_ context.Context, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
	return nil
}

func (m *mockStore) CleanExpiredSessions(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for token, s := range m.sessions {
		if now.After(s.ExpiresAt) {
			delete(m.sessions, token)
		}
	}
	m.cleaned++
	return nil
}

func TestManager_CreateAndGet(t *testing.T) {
	store := newMockStore()
	mgr := NewManager(store, false)

	// Create a session
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/admin/login", nil)
	r.RemoteAddr = "192.168.1.1:12345"
	r.Header.Set("User-Agent", "test-agent")

	token, err := mgr.Create(w, r, AdminCookieName, 42, 0, DefaultSessionTTL)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Verify cookie was set
	resp := w.Result()
	cookies := resp.Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == AdminCookieName {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected session cookie to be set")
	}
	if found.Value != token {
		t.Fatalf("cookie value = %q, want %q", found.Value, token)
	}
	if !found.HttpOnly {
		t.Error("expected HttpOnly cookie")
	}

	// Read the session back
	r2 := httptest.NewRequest(http.MethodGet, "/api/admin/me", nil)
	r2.AddCookie(found)

	sess, err := mgr.Get(r2, AdminCookieName)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.Token != token {
		t.Errorf("session token = %q, want %q", sess.Token, token)
	}
	if !sess.AdminID.Valid || sess.AdminID.Int64 != 42 {
		t.Errorf("admin_id = %v, want 42", sess.AdminID)
	}
	if sess.IPAddress != "192.168.1.1" {
		t.Errorf("ip_address = %q, want 192.168.1.1", sess.IPAddress)
	}
	if sess.UserAgent != "test-agent" {
		t.Errorf("user_agent = %q, want test-agent", sess.UserAgent)
	}
}

func TestManager_GetExpiredSession(t *testing.T) {
	store := newMockStore()
	mgr := NewManager(store, false)

	// Insert an expired session directly
	token := "expired-token-abc123"
	store.sessions[token] = &dbstore.Session{
		Token:     token,
		AdminID:   sql.NullInt64{Int64: 1, Valid: true},
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour), // already expired
		LastSeen:  time.Now().Add(-2 * time.Hour),
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: AdminCookieName, Value: token})

	sess, err := mgr.Get(r, AdminCookieName)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess != nil {
		t.Error("expected nil for expired session")
	}

	// Give async delete a moment to fire
	time.Sleep(50 * time.Millisecond)

	store.mu.Lock()
	_, exists := store.sessions[token]
	store.mu.Unlock()
	if exists {
		t.Error("expired session should have been deleted asynchronously")
	}
}

func TestManager_Delete(t *testing.T) {
	store := newMockStore()
	mgr := NewManager(store, true)

	// Create session
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/login", nil)
	r.RemoteAddr = "10.0.0.1:9999"

	token, _ := mgr.Create(w, r, CustomerCookieName, 0, 7, DefaultSessionTTL)

	// Delete session
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodPost, "/logout", nil)
	r2.AddCookie(&http.Cookie{Name: CustomerCookieName, Value: token})

	err := mgr.Delete(w2, r2, CustomerCookieName)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify cookie was cleared
	resp := w2.Result()
	for _, c := range resp.Cookies() {
		if c.Name == CustomerCookieName {
			if c.MaxAge != -1 {
				t.Errorf("expected MaxAge=-1 for cleared cookie, got %d", c.MaxAge)
			}
			break
		}
	}

	// Verify session is gone from store
	store.mu.Lock()
	_, exists := store.sessions[token]
	store.mu.Unlock()
	if exists {
		t.Error("session should have been deleted from store")
	}
}

func TestManager_GetNoCookie(t *testing.T) {
	store := newMockStore()
	mgr := NewManager(store, false)

	r := httptest.NewRequest(http.MethodGet, "/", nil)

	sess, err := mgr.Get(r, AdminCookieName)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess != nil {
		t.Error("expected nil when no cookie present")
	}
}

func TestManager_GetMissingSession(t *testing.T) {
	store := newMockStore()
	mgr := NewManager(store, false)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: AdminCookieName, Value: "nonexistent-token"})

	sess, err := mgr.Get(r, AdminCookieName)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess != nil {
		t.Error("expected nil for token not in store")
	}
}

func TestManager_GC(t *testing.T) {
	store := newMockStore()
	mgr := NewManager(store, false)
	mgr.SetGCInterval(50 * time.Millisecond)

	// Insert an expired and a valid session
	store.sessions["expired"] = &dbstore.Session{
		Token:     "expired",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	store.sessions["valid"] = &dbstore.Session{
		Token:     "valid",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	mgr.Start()
	time.Sleep(120 * time.Millisecond)
	mgr.Stop()

	store.mu.Lock()
	defer store.mu.Unlock()

	if _, ok := store.sessions["expired"]; ok {
		t.Error("GC should have removed expired session")
	}
	if _, ok := store.sessions["valid"]; !ok {
		t.Error("GC should NOT have removed valid session")
	}
	if store.cleaned == 0 {
		t.Error("expected GC to have run at least once")
	}
}

func TestManager_CustomerSession(t *testing.T) {
	store := newMockStore()
	mgr := NewManager(store, false)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/portal/login", nil)
	r.RemoteAddr = "172.16.0.5:8080"

	token, err := mgr.Create(w, r, CustomerCookieName, 0, 99, 12*time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify cookie name
	resp := w.Result()
	var found *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == CustomerCookieName {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected customer session cookie")
	}

	// Read back
	r2 := httptest.NewRequest(http.MethodGet, "/portal/me", nil)
	r2.AddCookie(found)

	sess, err := mgr.Get(r2, CustomerCookieName)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session")
	}
	if !sess.CustomerID.Valid || sess.CustomerID.Int64 != 99 {
		t.Errorf("customer_id = %v, want 99", sess.CustomerID)
	}
	if sess.AdminID.Valid {
		t.Error("admin_id should not be set for customer session")
	}
	_ = token
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name     string
		xff      string
		xri      string
		remote   string
		expected string
	}{
		{"xff single", "1.2.3.4", "", "5.6.7.8:1234", "1.2.3.4"},
		{"xff chain", "1.2.3.4, 5.6.7.8", "", "9.0.0.1:1234", "1.2.3.4"},
		{"x-real-ip", "", "10.0.0.1", "5.6.7.8:1234", "10.0.0.1"},
		{"remote addr", "", "", "192.168.1.1:4321", "192.168.1.1"},
		{"no port", "", "", "192.168.1.1", "192.168.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remote
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				r.Header.Set("X-Real-IP", tt.xri)
			}
			got := clientIP(r)
			if got != tt.expected {
				t.Errorf("clientIP() = %q, want %q", got, tt.expected)
			}
		})
	}
}
