package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	AdminCookieName    = "koris_admin_session"
	CustomerCookieName = "koris_customer_session"
)

type Service struct{ DB *sql.DB }

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func RandomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s Service) AdminCount() (int, error) {
	var c int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM admins`).Scan(&c)
	return c, err
}

func (s Service) CreateOwner(username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || len(password) < 6 {
		return errors.New("invalid owner")
	}
	h, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO admins(username,password_hash,role) VALUES(?,?, 'owner')`, username, h)
	return err
}

// RateLimitWindow is the time window for counting failed login attempts.
const RateLimitWindow = 15 * time.Minute

// MaxLoginAttempts is the maximum number of failed attempts per IP within the window.
const MaxLoginAttempts = 10

// IsRateLimited checks if the given IP has exceeded the login attempt limit.
func (s Service) IsRateLimited(ip string) bool {
	var count int
	err := s.DB.QueryRow(
		`SELECT COUNT(*) FROM admin_login_attempts WHERE ip=? AND success=0 AND created_at > ?`,
		ip, time.Now().Add(-RateLimitWindow),
	).Scan(&count)
	if err != nil {
		return false // fail open on DB error
	}
	return count >= MaxLoginAttempts
}

// RecordLoginAttempt records a login attempt for rate limiting purposes.
func (s Service) RecordLoginAttempt(ip, username string, success bool) {
	successInt := 0
	if success {
		successInt = 1
	}
	_, _ = s.DB.Exec(
		`INSERT INTO admin_login_attempts(ip, username, success) VALUES(?, ?, ?)`,
		ip, username, successInt,
	)
	// Prune old entries periodically (older than 24h)
	_, _ = s.DB.Exec(`DELETE FROM admin_login_attempts WHERE created_at < ?`, time.Now().Add(-24*time.Hour))
}

func (s Service) LoginAdmin(username, password string) (bool, error) {
	username = strings.TrimSpace(username)
	var hash string
	var active int
	err := s.DB.QueryRow(`SELECT password_hash,is_active FROM admins WHERE username=? LIMIT 1`, username).Scan(&hash, &active)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return active == 1 && CheckPassword(hash, password), nil
}

func MakeSession(username, secret string, ttl time.Duration) string {
	encodedUser := base64.RawURLEncoding.EncodeToString([]byte(username))
	expires := time.Now().Add(ttl).Unix()
	payload := fmt.Sprintf("%s.%d", encodedUser, expires)
	return payload + "." + sign(payload, secret)
}

func ReadSession(r *http.Request, cookieName, secret string) (string, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 {
		return "", false
	}
	payload := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(sign(payload, secret))) {
		return "", false
	}
	expires, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return "", false
	}
	userBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	username := strings.TrimSpace(string(userBytes))
	return username, username != ""
}

func SetSession(w http.ResponseWriter, cookieName, username, secret string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    MakeSession(username, secret, 24*time.Hour),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})
}

func ClearSession(w http.ResponseWriter, cookieName string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func sign(payload, secret string) string {
	if secret == "" {
		secret = "koris-next-dev-session-secret"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
