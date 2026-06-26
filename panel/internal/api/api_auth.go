package api

import (
	"KorisPanel/panel/internal/auth"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	uptime := int64(time.Since(s.StartedAt).Seconds())
	writeJSON(w, map[string]any{
		"ok":             true,
		"service":        "panel",
		"version":        s.Config.Version,
		"worker_id":      s.Config.WorkerID,
		"uptime_seconds": uptime,
		"time":           time.Now().UTC(),
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	c, err := s.Auth.AdminCount()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{
		"ok":                 true,
		"needs_setup":        c == 0,
		"setup_key_required": s.Config.SetupKey != "",
	})
}

func (s *Server) setupOwner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	limitBody(w, r, maxJSONBody)
	var in struct {
		SetupKey string `json:"setup_key"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if s.Config.SetupKey != "" && in.SetupKey != s.Config.SetupKey {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_setup_key"})
		return
	}
	c, err := s.Auth.AdminCount()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if c > 0 {
		writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "already_setup"})
		return
	}
	if err := s.Auth.CreateOwner(in.Username, in.Password); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	auth.SetSession(w, auth.AdminCookieName, in.Username, s.Config.SessionSecret, s.Config.SecureCookies)
	writeJSON(w, map[string]any{"ok": true, "username": in.Username, "role": "owner"})
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	limitBody(w, r, maxJSONBody)
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	ok, err := s.Auth.LoginAdmin(in.Username, in.Password)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid"})
		return
	}
	auth.SetSession(w, auth.AdminCookieName, in.Username, s.Config.SessionSecret, s.Config.SecureCookies)
	role := "admin"
	_ = s.DB.QueryRow(`SELECT role FROM admins WHERE username=$1 LIMIT 1`, in.Username).Scan(&role)
	writeJSON(w, map[string]any{"ok": true, "username": in.Username, "role": role})
}

func (s *Server) adminMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, role, ok := s.currentAdmin(r)
	credit := 0.00
	if ok {
		_ = s.DB.QueryRow(`SELECT COALESCE(credit, 0) FROM admins WHERE username=$1`, username).Scan(&credit)
	}
	writeJSON(w, map[string]any{"ok": true, "authenticated": ok, "username": username, "role": role, "credit": credit})
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	auth.ClearSession(w, auth.AdminCookieName, s.Config.SecureCookies)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) customerLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	limitBody(w, r, maxJSONBody)
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	var pw string
	err := s.DB.QueryRow(`SELECT value FROM radcheck WHERE username=$1 AND attribute IN('Cleartext-Password','User-Password') ORDER BY id DESC LIMIT 1`, in.Username).Scan(&pw)
	if err != nil {
		// Perform dummy comparison to prevent timing-based user enumeration
		subtle.ConstantTimeCompare([]byte("dummy-value-padding"), []byte(in.Password))
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(pw), []byte(in.Password)) != 1 {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid"})
		return
	}
	_, _ = s.DB.Exec(`INSERT INTO customers(username,sub_token) VALUES($1,$2) ON CONFLICT (username) DO NOTHING`, in.Username, auth.RandomToken(24))
	_, _ = s.DB.Exec(`INSERT INTO wallets(username,credit) VALUES($1,0) ON CONFLICT (username) DO NOTHING`, in.Username)
	auth.SetSession(w, auth.CustomerCookieName, in.Username, s.Config.SessionSecret, s.Config.SecureCookies)
	writeJSON(w, map[string]any{"ok": true, "username": in.Username})
}

func (s *Server) customerLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	auth.ClearSession(w, auth.CustomerCookieName, s.Config.SecureCookies)
	writeJSON(w, map[string]any{"ok": true})
}
