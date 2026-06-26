//go:build !lite

package api

import (
	"KorisPanel/panel/internal/landing"
	"encoding/json"
	"log"
	"net/http"
)

// landingConfigKeys are the supported landing page configuration fields.
// Each is stored in panel_settings with a "landing_" prefix.
var landingConfigKeys = []string{
	"hero_headline",
	"hero_subheadline",
	"hero_cta_text",
	"hero_cta_url",
	"features_title",
	"features_subtitle",
	"show_pricing",
	"show_faq",
}

// adminLandingPage handles GET/POST /api/admin/landing-page.
// GET  — returns the current landing page configuration.
// POST — partial update of landing page config fields.
func (s *Server) adminLandingPage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getLandingConfig(w, r)
	case http.MethodPost:
		s.setLandingConfig(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getLandingConfig returns the current landing page configuration from panel_settings.
func (s *Server) getLandingConfig(w http.ResponseWriter, r *http.Request) {
	config := map[string]any{}

	for _, key := range landingConfigKeys {
		dbKey := "landing_" + key
		var val string
		err := s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = $1`, dbKey).Scan(&val)
		if err != nil {
			// Key not set yet — use defaults
			switch key {
			case "show_pricing", "show_faq":
				config[key] = true
			default:
				config[key] = ""
			}
			continue
		}
		// Parse booleans for show_* fields
		switch key {
		case "show_pricing", "show_faq":
			config[key] = val == "true"
		default:
			config[key] = val
		}
	}

	writeJSON(w, map[string]any{"ok": true, "config": config})
}

// setLandingConfig performs a partial update of landing page configuration.
func (s *Server) setLandingConfig(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if len(raw) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "empty_body"})
		return
	}

	// Build allowed keys set for validation
	allowed := map[string]bool{}
	for _, k := range landingConfigKeys {
		allowed[k] = true
	}

	for key, rawVal := range raw {
		if !allowed[key] {
			continue // skip unknown fields silently
		}

		// Convert value to string for storage
		var strVal string
		switch key {
		case "show_pricing", "show_faq":
			var boolVal bool
			if err := json.Unmarshal(rawVal, &boolVal); err != nil {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_value_" + key})
				return
			}
			if boolVal {
				strVal = "true"
			} else {
				strVal = "false"
			}
		default:
			var s string
			if err := json.Unmarshal(rawVal, &s); err != nil {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_value_" + key})
				return
			}
			strVal = s
		}

		dbKey := "landing_" + key
		_, err := s.DB.Exec(
			`INSERT INTO panel_settings (setting_key, setting_value) VALUES ($1, $2) ON CONFLICT (setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value`,
			dbKey, strVal,
		)
		if err != nil {
			log.Printf("[landing] failed to save %s: %v", dbKey, err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
	}

	writeJSON(w, map[string]any{"ok": true})

	// Invalidate the landing page meta tag cache so next request picks up new values
	s.InvalidateLandingMetaCache()
}

// adminLandingBlocklistCheck handles POST /api/admin/landing-page/check-blocklist.
// Accepts a JSON body with content fields and returns any blocklist matches found.
func (s *Server) adminLandingBlocklistCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)

	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	matches := landing.ValidateFields(req)
	writeJSON(w, map[string]any{
		"ok":      true,
		"matches": matches,
		"clean":   len(matches) == 0,
	})
}
