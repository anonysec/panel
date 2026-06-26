package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *Server) certificates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listCertificates(w)
	case http.MethodPost:
		s.uploadCertificate(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listCertificates(w http.ResponseWriter) {
	rows, err := s.DB.Query(`SELECT id, name, type, node_id, SUBSTRING(content, 1, 80), is_default, created_at FROM vpn_certificates ORDER BY is_default DESC, id DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	certs := []VPNCertificate{}
	for rows.Next() {
		var c VPNCertificate
		var nodeID sql.NullInt64
		var isDefault int
		var created sql.NullTime
		var preview string
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &nodeID, &preview, &isDefault, &created); err == nil {
			if nodeID.Valid {
				c.NodeID = &nodeID.Int64
			}
			c.Content = preview + "..."
			c.IsDefault = isDefault == 1
			if created.Valid {
				c.CreatedAt = created.Time.Format(time.RFC3339)
			}
			certs = append(certs, c)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "certificates": certs})
}

func (s *Server) uploadCertificate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		NodeID    *int64 `json:"node_id"`
		Content   string `json:"content"`
		IsDefault bool   `json:"is_default"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.ToLower(strings.TrimSpace(in.Type))
	in.Content = strings.TrimSpace(in.Content)

	if in.Name == "" || in.Content == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_content_required"})
		return
	}
	if in.Type != "ca" && in.Type != "tls_crypt" && in.Type != "client_cert" && in.Type != "client_key" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_type"})
		return
	}

	defaultInt := 0
	if in.IsDefault {
		defaultInt = 1
		// Unset other defaults of same type
		_, _ = s.DB.Exec(`UPDATE vpn_certificates SET is_default=0 WHERE type=$1`, in.Type)
	}

	res, err := s.DB.Exec(`INSERT INTO vpn_certificates(name, type, node_id, content, is_default) VALUES($1,$2,$3,$4,$5)`,
		in.Name, in.Type, in.NodeID, in.Content, defaultInt)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "certificate.uploaded", "certificate", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name, "type": in.Type}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) certificateByID(w http.ResponseWriter, r *http.Request) {
	id, _, ok := pathID(r.URL.Path, "/api/certificates/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		var c VPNCertificate
		var nodeID sql.NullInt64
		var isDefault int
		var created sql.NullTime
		err := s.DB.QueryRow(`SELECT id, name, type, node_id, content, is_default, created_at FROM vpn_certificates WHERE id=$1`, id).Scan(&c.ID, &c.Name, &c.Type, &nodeID, &c.Content, &isDefault, &created)
		if err == sql.ErrNoRows {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if nodeID.Valid {
			c.NodeID = &nodeID.Int64
		}
		c.IsDefault = isDefault == 1
		if created.Valid {
			c.CreatedAt = created.Time.Format(time.RFC3339)
		}
		writeJSON(w, map[string]any{"ok": true, "certificate": c})

	case http.MethodDelete:
		if _, err := s.DB.Exec(`DELETE FROM vpn_certificates WHERE id=$1`, id); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		actor, _, _ := s.currentAdmin(r)
		s.logAudit(actor, "certificate.deleted", "certificate", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
		writeJSON(w, map[string]any{"ok": true})

	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// ========== Panel Settings ==========

func (s *Server) panelSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := s.DB.Query(`SELECT setting_key, setting_value FROM panel_settings ORDER BY setting_key`)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer rows.Close()
		settings := map[string]string{}
		for rows.Next() {
			var k, v string
			if rows.Scan(&k, &v) == nil {
				settings[k] = v
			}
		}
		writeJSON(w, map[string]any{"ok": true, "settings": settings})

	case http.MethodPatch:
		// Only owner/admin can change settings
		_, role, _ := s.currentAdmin(r)
		if role == "reseller" {
			writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
			return
		}
		var in map[string]string
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}
		for k, v := range in {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			_, _ = s.DB.Exec(`INSERT INTO panel_settings(setting_key, setting_value) VALUES($1,$2) ON CONFLICT (setting_key) DO UPDATE SET setting_value=EXCLUDED.setting_value`, k, v)
		}
		actor, _, _ := s.currentAdmin(r)
		s.logAudit(actor, "settings.updated", "panel_settings", "", nil, map[string]any{"keys": len(in)}, clientIP(r))
		writeJSON(w, map[string]any{"ok": true})

	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// publicSettings returns non-sensitive panel settings (theme, mode, panel name, language)
// without requiring authentication. This allows the portal to fetch admin-chosen theme settings.
func (s *Server) publicSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	allowedKeys := map[string]bool{
		"ui_theme":   true,
		"ui_mode":    true,
		"panel_name": true,
		"language":   true,
	}
	brandingKeys := map[string]string{
		"branding_logo_url":      "logo_url",
		"branding_app_name":      "app_name",
		"branding_primary_color": "primary_color",
	}
	rows, err := s.DB.Query(`SELECT setting_key, setting_value FROM panel_settings ORDER BY setting_key`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	settings := map[string]any{}
	branding := map[string]string{
		"logo_url":      "",
		"app_name":      "",
		"primary_color": "",
	}
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			if allowedKeys[k] {
				settings[k] = v
			} else if field, ok := brandingKeys[k]; ok {
				branding[field] = v
			}
		}
	}
	settings["branding"] = branding
	writeJSON(w, map[string]any{"ok": true, "settings": settings})
}

// checkWSOrigin validates the WebSocket Origin header against allowed origins.
// Empty Origin is allowed (same-origin requests from some browsers).
// The configured PublicBase and AllowedOrigins are checked.
func (s *Server) checkWSOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Same-origin requests may not send Origin
	}

	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	originHost := originURL.Host

	// Check against PublicBase
	if s.Config.PublicBase != "" {
		if pubURL, err := url.Parse(s.Config.PublicBase); err == nil {
			if pubURL.Host != "" && strings.EqualFold(pubURL.Host, originHost) {
				return true
			}
		}
	}

	// Check against AllowedOrigins list
	for _, allowed := range s.Config.AllowedOrigins {
		if allowedURL, err := url.Parse(allowed); err == nil {
			if strings.EqualFold(allowedURL.Host, originHost) {
				return true
			}
		}
		// Also allow direct host comparison
		if strings.EqualFold(allowed, originHost) || strings.EqualFold(allowed, origin) {
			return true
		}
	}

	// Check if origin matches the request's Host header (same-origin)
	if strings.EqualFold(originHost, r.Host) {
		return true
	}

	return false
}
