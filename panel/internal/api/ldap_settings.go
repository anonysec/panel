//go:build !lite

package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"KorisPanel/panel/internal/ldap"
)

// adminLDAPSettings handles GET and POST for LDAP configuration.
func (s *Server) adminLDAPSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getLDAPSettings(w, r)
	case http.MethodPost:
		s.setLDAPSettings(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// adminLDAPTest handles POST to test LDAP connection with provided config.
func (s *Server) adminLDAPTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var cfg ldap.LDAPConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if err := cfg.Validate(); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// If password is masked, load the existing password from DB
	if cfg.BindPassword == "********" {
		existing := ldap.LoadConfigFromDB(s.DB)
		cfg.BindPassword = existing.BindPassword
	}

	svc := ldap.New(cfg, s.DB)
	if err := svc.TestConnection(context.Background()); err != nil {
		log.Printf("[ldap] test connection failed: %v", err)
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "message": "connection_successful"})
}

// getLDAPSettings returns the current LDAP config with the password masked.
func (s *Server) getLDAPSettings(w http.ResponseWriter, r *http.Request) {
	cfg := ldap.LoadConfigFromDB(s.DB)
	masked := cfg.MaskedConfig()
	writeJSON(w, map[string]any{"ok": true, "config": masked})
}

// setLDAPSettings saves LDAP configuration to the panel_settings table.
func (s *Server) setLDAPSettings(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var cfg ldap.LDAPConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if err := cfg.Validate(); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// If password is masked, preserve the existing password
	if cfg.BindPassword == "********" {
		existing := ldap.LoadConfigFromDB(s.DB)
		cfg.BindPassword = existing.BindPassword
	}

	if err := ldap.SaveConfigToDB(s.DB, cfg); err != nil {
		log.Printf("[ldap] failed to save config: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "save_failed"})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "settings.ldap_update", "panel_settings", "ldap_config", nil, map[string]any{
		"enabled":    cfg.Enabled,
		"server_url": cfg.ServerURL,
	}, r.RemoteAddr)

	log.Printf("[ldap] config updated by %s (enabled=%v)", actor, cfg.Enabled)
	writeJSON(w, map[string]any{"ok": true})
}
