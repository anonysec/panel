//go:build !lite

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"KorisPanel/panel/internal/support"
)

// ──────────────────────────────────────────────────────────────────────────────
// Canned Responses CRUD (pre-written reply templates for support tickets)
// Requirements: 15.1, 15.2, 15.3, 15.4, 15.5
// ──────────────────────────────────────────────────────────────────────────────

// CannedResponse represents a pre-written reply template.
type CannedResponse struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Category   string `json:"category"`
	UsageCount int    `json:"usage_count"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// adminCannedResponses dispatches GET/POST for /api/admin/canned-responses and /api/canned-responses.
func (s *Server) adminCannedResponses(w http.ResponseWriter, r *http.Request) {
	// Check if path has subpath (e.g., /api/canned-responses/preview or /api/canned-responses/123)
	for _, prefix := range []string{"/api/canned-responses/", "/api/admin/canned-responses/"} {
		if strings.HasPrefix(r.URL.Path, prefix) {
			rest := strings.TrimPrefix(r.URL.Path, prefix)
			rest = strings.Trim(rest, "/")
			if rest == "" {
				break
			}
			// Handle /api/canned-responses/preview
			if rest == "preview" {
				if r.Method != http.MethodPost {
					http.Error(w, "method", http.StatusMethodNotAllowed)
					return
				}
				s.cannedResponsePreview(w, r)
				return
			}
			// Handle /api/canned-responses/{id} and /api/canned-responses/{id}/use
			parts := strings.Split(rest, "/")
			id, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil || id <= 0 {
				writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
				return
			}
			if len(parts) > 1 && parts[1] == "use" {
				if r.Method != http.MethodPost {
					http.Error(w, "method", http.StatusMethodNotAllowed)
					return
				}
				s.useCannedResponse(w, r, id)
				return
			}
			// PATCH or DELETE by ID
			switch r.Method {
			case http.MethodPatch:
				s.updateCannedResponse(w, r, id)
			case http.MethodDelete:
				s.deleteCannedResponseByID(w, r, id)
			default:
				http.Error(w, "method", http.StatusMethodNotAllowed)
			}
			return
		}
	}

	// Root path: GET (list) or POST (create)
	switch r.Method {
	case http.MethodGet:
		s.listCannedResponses(w, r)
	case http.MethodPost:
		s.createCannedResponse(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// listCannedResponses returns all canned responses, grouped by category, sorted by usage_count DESC within each.
func (s *Server) listCannedResponses(w http.ResponseWriter, r *http.Request) {
	category := strings.TrimSpace(r.URL.Query().Get("category"))

	query := `SELECT id, title, body, COALESCE(category,'general'), usage_count, created_at, updated_at FROM canned_responses`
	args := []any{}

	if category != "" {
		query += ` WHERE category = $1`
		args = append(args, category)
	}
	query += ` ORDER BY category ASC, usage_count DESC, title ASC`

	rows, err := s.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "query_failed"})
		return
	}
	defer rows.Close()

	// Group responses by category
	grouped := make(map[string][]CannedResponse)
	allResponses := []CannedResponse{}
	for rows.Next() {
		var cr CannedResponse
		if err := rows.Scan(&cr.ID, &cr.Title, &cr.Body, &cr.Category, &cr.UsageCount, &cr.CreatedAt, &cr.UpdatedAt); err != nil {
			continue
		}
		allResponses = append(allResponses, cr)
		grouped[cr.Category] = append(grouped[cr.Category], cr)
	}

	writeJSON(w, map[string]any{
		"ok":        true,
		"responses": allResponses,
		"grouped":   grouped,
	})
}

// createCannedResponse creates a new canned response.
func (s *Server) createCannedResponse(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	in.Category = strings.TrimSpace(in.Category)

	if in.Title == "" || in.Body == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "title_and_body_required"})
		return
	}
	if in.Category == "" {
		in.Category = "general"
	}

	res, err := s.DB.ExecContext(r.Context(),
		`INSERT INTO canned_responses (title, body, category) VALUES ($1, $2, $3)`,
		in.Title, in.Body, in.Category)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "insert_failed"})
		return
	}

	id, _ := res.LastInsertId()

	admin, _, _ := s.currentAdmin(r)
	s.logAudit(admin, "canned_response.created", "canned_response", strconv.FormatInt(id, 10), nil,
		map[string]any{"title": in.Title, "category": in.Category}, clientIP(r))

	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// updateCannedResponse updates a canned response (title, body, category).
func (s *Server) updateCannedResponse(w http.ResponseWriter, r *http.Request, id int64) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		Title    *string `json:"title"`
		Body     *string `json:"body"`
		Category *string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Build dynamic UPDATE query
	sets := []string{}
	args := []any{}

	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "title_cannot_be_empty"})
			return
		}
		sets = append(sets, "title = $1")
		args = append(args, t)
	}
	if in.Body != nil {
		b := strings.TrimSpace(*in.Body)
		if b == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "body_cannot_be_empty"})
			return
		}
		sets = append(sets, "body = $1")
		args = append(args, b)
	}
	if in.Category != nil {
		c := strings.TrimSpace(*in.Category)
		if c == "" {
			c = "general"
		}
		sets = append(sets, "category = $1")
		args = append(args, c)
	}

	if len(sets) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "no_fields_to_update"})
		return
	}

	args = append(args, id)
	query := fmt.Sprintf(`UPDATE canned_responses SET %s WHERE id = $1`, strings.Join(sets, ", "))

	result, err := s.DB.ExecContext(r.Context(), query, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "update_failed"})
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	admin, _, _ := s.currentAdmin(r)
	s.logAudit(admin, "canned_response.updated", "canned_response", strconv.FormatInt(id, 10), nil, nil, clientIP(r))

	writeJSON(w, map[string]any{"ok": true})
}

// deleteCannedResponseByID deletes a canned response by ID from path.
func (s *Server) deleteCannedResponseByID(w http.ResponseWriter, r *http.Request, id int64) {
	result, err := s.DB.ExecContext(r.Context(), `DELETE FROM canned_responses WHERE id = $1`, id)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "delete_failed"})
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	admin, _, _ := s.currentAdmin(r)
	s.logAudit(admin, "canned_response.deleted", "canned_response", strconv.FormatInt(id, 10), nil, nil, clientIP(r))

	writeJSON(w, map[string]any{"ok": true})
}

// useCannedResponse increments usage_count for a canned response.
// POST /api/canned-responses/{id}/use
func (s *Server) useCannedResponse(w http.ResponseWriter, r *http.Request, id int64) {
	result, err := s.DB.ExecContext(r.Context(),
		`UPDATE canned_responses SET usage_count = usage_count + 1 WHERE id = $1`, id)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "update_failed"})
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// cannedResponsePreview substitutes placeholders and returns preview text.
// POST /api/canned-responses/preview
func (s *Server) cannedResponsePreview(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		Body string            `json:"body"`
		Vars map[string]string `json:"vars"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.Body == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "body_required"})
		return
	}

	result := support.SubstitutePlaceholders(in.Body, in.Vars)

	writeJSON(w, map[string]any{
		"ok":           true,
		"result":       result,
		"placeholders": support.DefaultPlaceholders(),
	})
}
