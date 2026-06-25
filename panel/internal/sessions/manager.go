package sessions

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log"
	"net/http"
	"time"

	"KorisPanel/panel/internal/dbstore"
)

const (
	// AdminCookieName is the HTTP cookie name for admin sessions.
	AdminCookieName = "koris_admin_session"
	// CustomerCookieName is the HTTP cookie name for customer sessions.
	CustomerCookieName = "koris_customer_session"

	// DefaultSessionTTL is the default session lifetime.
	DefaultSessionTTL = 24 * time.Hour

	// DefaultGCInterval is how often expired sessions are cleaned up.
	DefaultGCInterval = 5 * time.Minute

	// tokenLength is the number of random bytes used for session tokens.
	tokenLength = 32
)

// Manager handles database-backed HTTP sessions.
// It delegates all persistence to a dbstore.Store implementation,
// making it compatible with multi-worker deployments.
type Manager struct {
	store        dbstore.Store
	secureCookie bool
	gcInterval   time.Duration
	done         chan struct{}
}

// NewManager creates a session manager backed by the given store.
// secureCookie controls whether the Secure flag is set on session cookies.
func NewManager(store dbstore.Store, secureCookie bool) *Manager {
	return &Manager{
		store:        store,
		secureCookie: secureCookie,
		gcInterval:   DefaultGCInterval,
		done:         make(chan struct{}),
	}
}

// SetGCInterval configures how often the GC ticker runs.
// Must be called before Start.
func (m *Manager) SetGCInterval(d time.Duration) {
	if d > 0 {
		m.gcInterval = d
	}
}

// Start begins the background session GC goroutine.
func (m *Manager) Start() {
	go func() {
		ticker := time.NewTicker(m.gcInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.cleanExpired()
			case <-m.done:
				return
			}
		}
	}()
}

// Stop halts the background GC goroutine.
func (m *Manager) Stop() {
	close(m.done)
}

// Create creates a new session and sets the session cookie on the response.
// It returns the session token.
func (m *Manager) Create(w http.ResponseWriter, r *http.Request, cookieName string, adminID, customerID int64, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}

	token := generateToken()
	now := time.Now().UTC()

	sess := &dbstore.Session{
		Token:      token,
		AdminID:    toNullInt64(adminID),
		CustomerID: toNullInt64(customerID),
		IPAddress:  clientIP(r),
		UserAgent:  r.UserAgent(),
		CreatedAt:  now,
		ExpiresAt:  now.Add(ttl),
		LastSeen:   now,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := m.store.SaveSession(ctx, sess); err != nil {
		return "", err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secureCookie,
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})

	return token, nil
}

// Get retrieves the session associated with the given cookie name from the request.
// Returns nil if no valid session exists (missing cookie, expired, or not in DB).
func (m *Manager) Get(r *http.Request, cookieName string) (*dbstore.Session, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sess, err := m.store.GetSession(ctx, cookie.Value)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, nil
	}

	// Check expiry
	if time.Now().UTC().After(sess.ExpiresAt) {
		// Expired — clean it up asynchronously
		go func() {
			bgCtx, bgCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer bgCancel()
			_ = m.store.DeleteSession(bgCtx, cookie.Value)
		}()
		return nil, nil
	}

	// Update last_seen (fire-and-forget to avoid slowing reads)
	go func() {
		updated := *sess
		updated.LastSeen = time.Now().UTC()
		bgCtx, bgCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer bgCancel()
		_ = m.store.SaveSession(bgCtx, &updated)
	}()

	return sess, nil
}

// Delete removes the session and clears the cookie.
func (m *Manager) Delete(w http.ResponseWriter, r *http.Request, cookieName string) error {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		// No cookie to clear, just set expired cookie
		m.clearCookie(w, cookieName)
		return nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := m.store.DeleteSession(ctx, cookie.Value); err != nil {
		// Log but still clear the cookie on the client side
		log.Printf("[sessions] delete session: %v", err)
	}

	m.clearCookie(w, cookieName)
	return nil
}

// DeleteByToken removes a session by its token (without needing an HTTP request).
func (m *Manager) DeleteByToken(ctx context.Context, token string) error {
	return m.store.DeleteSession(ctx, token)
}

// cleanExpired runs the store's CleanExpiredSessions method.
func (m *Manager) cleanExpired() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := m.store.CleanExpiredSessions(ctx); err != nil {
		log.Printf("[sessions] GC clean expired: %v", err)
	}
}

// clearCookie sets an expired cookie on the response to delete it from the browser.
func (m *Manager) clearCookie(w http.ResponseWriter, cookieName string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secureCookie,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

// generateToken produces a cryptographically random hex token.
func generateToken() string {
	b := make([]byte, tokenLength)
	if _, err := rand.Read(b); err != nil {
		panic("sessions: failed to generate random token: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// toNullInt64 converts an int64 to sql.NullInt64 (valid only if > 0).
func toNullInt64(v int64) sql.NullInt64 {
	if v > 0 {
		return sql.NullInt64{Int64: v, Valid: true}
	}
	return sql.NullInt64{}
}

// clientIP extracts the client IP from the request.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Strip port from RemoteAddr
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}
