//go:build !lite

package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"KorisPanel/panel/internal/xray"
)

// handleXrayTemplates handles GET (list) and POST (create) on /api/admin/xray/templates.
func (s *Server) handleXrayTemplates(w http.ResponseWriter, r *http.Request) {
	xraySvc := xray.New(s.DB)

	switch r.Method {
	case http.MethodGet:
		templates, err := xraySvc.ListTemplates(r.Context())
		if err != nil {
			log.Printf("[xray] list templates error: %v", err)
			writeJSON(w, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "templates": templates})

	case http.MethodPost:
		limitBody(w, r, maxJSONBody)
		var in struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			ConfigJSON  json.RawMessage `json:"config_json"`
			Category    string          `json:"category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}

		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_required"})
			return
		}
		if len(in.ConfigJSON) == 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "config_required"})
			return
		}

		tmpl := &xray.XrayTemplate{
			Name:        in.Name,
			Description: in.Description,
			ConfigJSON:  in.ConfigJSON,
			Category:    in.Category,
		}

		id, err := xraySvc.CreateTemplate(r.Context(), tmpl)
		if err != nil {
			log.Printf("[xray] create template error: %v", err)
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "create_failed"})
			return
		}

		writeJSON(w, map[string]any{"ok": true, "id": id})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleXrayTemplateByID handles GET, PUT, DELETE on /api/admin/xray/templates/{id}.
func (s *Server) handleXrayTemplateByID(w http.ResponseWriter, r *http.Request) {
	xraySvc := xray.New(s.DB)

	// Extract ID from path: /api/admin/xray/templates/{id} or /api/admin/xray/templates/{id}/apply
	pathAfter := strings.TrimPrefix(r.URL.Path, "/api/admin/xray/templates/")
	parts := strings.SplitN(pathAfter, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_id"})
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_id"})
		return
	}

	// Check for /apply sub-path.
	if len(parts) == 2 && parts[1] == "apply" {
		s.handleXrayTemplateApply(w, r, xraySvc, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		tmpl, err := xraySvc.GetTemplate(r.Context(), id)
		if err != nil {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "template": tmpl})

	case http.MethodPut:
		limitBody(w, r, maxJSONBody)
		var in struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			ConfigJSON  json.RawMessage `json:"config_json"`
			Category    string          `json:"category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}

		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_required"})
			return
		}
		if len(in.ConfigJSON) == 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "config_required"})
			return
		}

		tmpl := &xray.XrayTemplate{
			Name:        in.Name,
			Description: in.Description,
			ConfigJSON:  in.ConfigJSON,
			Category:    in.Category,
		}

		if err := xraySvc.UpdateTemplate(r.Context(), id, tmpl); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			} else {
				log.Printf("[xray] update template error: %v", err)
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "update_failed"})
			}
			return
		}

		writeJSON(w, map[string]any{"ok": true})

	case http.MethodDelete:
		if err := xraySvc.DeleteTemplate(r.Context(), id); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			} else {
				log.Printf("[xray] delete template error: %v", err)
				writeJSON(w, map[string]any{"ok": false, "error": "db_error"})
			}
			return
		}
		writeJSON(w, map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleXrayTemplateApply handles POST /api/admin/xray/templates/{id}/apply.
func (s *Server) handleXrayTemplateApply(w http.ResponseWriter, r *http.Request, xraySvc *xray.XrayService, templateID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitBody(w, r, maxJSONBody)
	var in struct {
		NodeID int64 `json:"node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.NodeID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_id_required"})
		return
	}

	if err := xraySvc.ApplyTemplate(r.Context(), in.NodeID, templateID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		} else {
			log.Printf("[xray] apply template error: %v", err)
			writeJSON(w, map[string]any{"ok": false, "error": "apply_failed"})
		}
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}
