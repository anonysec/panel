package api

import (
	"encoding/json"
	"net/http"
	"time"
)

// handleTheme dispatches GET/POST for /api/admin/theme.
func (s *Server) handleTheme(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.themeGet(w, r)
	case http.MethodPost:
		s.themePost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// themeGet returns the active theme configuration and list of available presets.
// GET /api/admin/theme
func (s *Server) themeGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get active theme ID from settings
	var activeID string
	_ = s.DB.QueryRow(`SELECT value FROM panel_settings WHERE key_name='theme_active_id'`).Scan(&activeID)
	if activeID == "" {
		activeID = "default-light"
	}

	// Get all theme presets
	rows, err := s.DB.Query(`SELECT id, name, mode, config_json, is_default, COALESCE(created_by,''), created_at FROM theme_presets ORDER BY is_default DESC, name ASC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type themeEntry struct {
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Mode      string          `json:"mode"`
		Config    json.RawMessage `json:"config"`
		IsDefault bool            `json:"is_default"`
		CreatedBy string          `json:"created_by,omitempty"`
		CreatedAt time.Time       `json:"created_at"`
	}

	themes := []themeEntry{}
	for rows.Next() {
		var t themeEntry
		var configStr string
		if err := rows.Scan(&t.ID, &t.Name, &t.Mode, &configStr, &t.IsDefault, &t.CreatedBy, &t.CreatedAt); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "scan_error"})
			return
		}
		t.Config = json.RawMessage(configStr)
		themes = append(themes, t)
	}

	writeJSON(w, map[string]any{
		"ok":        true,
		"active_id": activeID,
		"themes":    themes,
	})
}

// themePost creates or updates a theme preset and/or sets the active theme.
// POST /api/admin/theme
func (s *Server) themePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)

	var in struct {
		Action   string          `json:"action"` // "set_active", "create", "update", "delete"
		ID       string          `json:"id"`
		Name     string          `json:"name"`
		Mode     string          `json:"mode"`
		Config   json.RawMessage `json:"config"`
		ActiveID string          `json:"active_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	switch in.Action {
	case "set_active":
		if in.ActiveID == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_active_id"})
			return
		}
		_, err := s.DB.Exec(`INSERT INTO panel_settings (key_name, value) VALUES ('theme_active_id', $1) ON CONFLICT (key_name) DO UPDATE SET value = EXCLUDED.value`, in.ActiveID, in.ActiveID)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "active_id": in.ActiveID})

	case "create":
		if in.ID == "" || in.Name == "" || in.Mode == "" || len(in.Config) == 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_fields"})
			return
		}
		_, err := s.DB.Exec(`INSERT INTO theme_presets (id, name, mode, config_json, is_default) VALUES ($1, $2, $3, $4, FALSE)`,
			in.ID, in.Name, in.Mode, string(in.Config))
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "id": in.ID})

	case "update":
		if in.ID == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_id"})
			return
		}
		_, err := s.DB.Exec(`UPDATE theme_presets SET name=$1, mode=$2, config_json=$3 WHERE id=$4`,
			in.Name, in.Mode, string(in.Config), in.ID)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "id": in.ID})

	case "delete":
		if in.ID == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_id"})
			return
		}
		_, err := s.DB.Exec(`DELETE FROM theme_presets WHERE id=$1 AND is_default=FALSE`, in.ID)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		writeJSON(w, map[string]any{"ok": true})

	default:
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_action"})
	}
}

// brandingPost saves custom branding settings (logo, app name, primary color).
// POST /api/admin/branding
func (s *Server) brandingPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)

	var in struct {
		LogoURL      string `json:"logo_url"`
		AppName      string `json:"app_name"`
		PrimaryColor string `json:"primary_color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	settings := map[string]string{
		"branding_logo_url":      in.LogoURL,
		"branding_app_name":      in.AppName,
		"branding_primary_color": in.PrimaryColor,
	}

	for key, val := range settings {
		if val != "" {
			_, err := s.DB.Exec(`INSERT INTO panel_settings (setting_key, setting_value) VALUES ($1, $2) ON CONFLICT (setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value`, key, val, val)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
				return
			}
		}
	}

	writeJSON(w, map[string]any{"ok": true})
}
