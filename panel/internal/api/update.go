package api

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"

	"KorisPanel/panel/internal/updater"
)

// validTimeFormat matches HH:MM (00:00 to 23:59).
var validTimeFormat = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// handleUpdateSettings handles GET and POST for auto-update configuration.
// GET  /api/admin/settings → returns auto_update_enabled and auto_update_time
// POST /api/admin/settings → stores auto_update_enabled and auto_update_time
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		keys := []string{"auto_update_enabled", "auto_update_time"}
		settings := map[string]string{}
		for _, k := range keys {
			var v string
			if err := s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = $1`, k).Scan(&v); err == nil {
				settings[k] = v
			}
		}
		writeJSON(w, map[string]any{"ok": true, "settings": settings})

	case http.MethodPost:
		limitBody(w, r, maxJSONBody)
		var in struct {
			AutoUpdateEnabled *string `json:"auto_update_enabled"`
			AutoUpdateTime    *string `json:"auto_update_time"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}

		// Validate auto_update_enabled
		if in.AutoUpdateEnabled != nil {
			v := *in.AutoUpdateEnabled
			if v != "true" && v != "false" {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_auto_update_enabled"})
				return
			}
		}

		// Validate auto_update_time (HH:MM)
		if in.AutoUpdateTime != nil {
			if !validTimeFormat.MatchString(*in.AutoUpdateTime) {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_auto_update_time"})
				return
			}
		}

		// Upsert provided values
		if in.AutoUpdateEnabled != nil {
			_, err := s.DB.Exec(
				`INSERT INTO panel_settings(setting_key, setting_value) VALUES($1, $2) ON CONFLICT (setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value`,
				"auto_update_enabled", *in.AutoUpdateEnabled,
			)
			if err != nil {
				log.Printf("[update] failed to save auto_update_enabled: %v", err)
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
				return
			}
		}
		if in.AutoUpdateTime != nil {
			_, err := s.DB.Exec(
				`INSERT INTO panel_settings(setting_key, setting_value) VALUES($1, $2) ON CONFLICT (setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value`,
				"auto_update_time", *in.AutoUpdateTime,
			)
			if err != nil {
				log.Printf("[update] failed to save auto_update_time: %v", err)
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
				return
			}
		}

		actor, _, _ := s.currentAdmin(r)
		s.logAudit(actor, "settings.auto_update", "panel_settings", "", nil, map[string]any{
			"auto_update_enabled": in.AutoUpdateEnabled,
			"auto_update_time":    in.AutoUpdateTime,
		}, clientIP(r))

		writeJSON(w, map[string]any{"ok": true})

	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// handleUpdateCheck checks for available panel updates.
// GET /api/admin/update/check
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	if s.Config.ReleaseURL == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": "updates_disabled",
		})
		return
	}

	u := updater.New(s.Config.Version, s.Config.ReleaseURL, "")
	info, err := u.Check()
	if err != nil {
		log.Printf("[update] check failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": "check_failed",
		})
		return
	}

	writeJSON(w, map[string]any{
		"ok":     true,
		"update": info,
	})
}

// handleUpdateRollback rolls back to the previous panel binary version.
// POST /api/admin/update/rollback
func (s *Server) handleUpdateRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var in struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		// Allow empty body — reason is optional
		in.Reason = ""
	}

	if s.Config.ReleaseURL == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": "updates_disabled",
		})
		return
	}

	u := updater.New(s.Config.Version, s.Config.ReleaseURL, "")

	if err := u.Rollback(); err != nil {
		log.Printf("[update] rollback failed: %v (reason: %s)", err, in.Reason)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": "rollback_failed",
		})
		return
	}

	log.Printf("[update] rollback completed (reason: %s)", in.Reason)
	writeJSON(w, map[string]any{
		"ok":      true,
		"message": "rollback completed",
	})
}

// handleUpdateApply applies an available panel update with WebSocket progress broadcast.
// POST /api/admin/update/apply
func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	if s.Config.ReleaseURL == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": "updates_disabled",
		})
		return
	}

	u := updater.New(s.Config.Version, s.Config.ReleaseURL, "")

	info, err := u.Check()
	if err != nil {
		log.Printf("[update] check before apply failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": "check_failed",
		})
		return
	}

	if !info.Available {
		writeJSON(w, map[string]any{
			"ok":    false,
			"error": "no_update_available",
		})
		return
	}

	progressFn := func(stage string, pct float64) {
		s.broadcastWSCleanup(map[string]any{
			"type":     "update_progress",
			"stage":    stage,
			"progress": pct,
		})
	}

	if err := u.Apply(info, progressFn); err != nil {
		log.Printf("[update] apply failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": "apply_failed",
		})
		return
	}

	writeJSON(w, map[string]any{
		"ok":      true,
		"message": "update applied, restarting...",
	})
}
