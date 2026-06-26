package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// UserTemplate represents a user configuration template for quick customer provisioning.
type UserTemplate struct {
	ID              int64           `json:"id"`
	Name            string          `json:"name"`
	PlanID          *int64          `json:"plan_id"`
	Status          string          `json:"status"`
	ConnectionLimit int             `json:"connection_limit"`
	RadiusChecks    json.RawMessage `json:"radius_checks"`
	RadiusReplies   json.RawMessage `json:"radius_replies"`
	CreatedBy       string          `json:"created_by"`
	DeletedAt       *string         `json:"deleted_at"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

// templates handles GET /api/templates (list) and POST /api/templates (create).
func (s *Server) templates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listTemplates(w, r)
	case http.MethodPost:
		s.createTemplate(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// templateByID handles PATCH and DELETE for /api/templates/{id}.
func (s *Server) templateByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/templates/")
	if !ok || action != "" {
		writeError(w, http.StatusNotFound, "not_found", "template not found")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		s.updateTemplate(w, r, id)
	case http.MethodDelete:
		s.deleteTemplate(w, r, id)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// listTemplates returns all non-deleted templates.
func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(`SELECT id, name, plan_id, status, connection_limit, radius_checks, radius_replies, created_by, deleted_at, created_at, updated_at FROM user_templates WHERE deleted_at IS NULL ORDER BY id DESC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	defer rows.Close()

	templates := []UserTemplate{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		templates = append(templates, t)
	}
	writeJSON(w, map[string]any{"ok": true, "templates": templates})
}

// createTemplate creates a new user template.
func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name            string          `json:"name"`
		PlanID          *int64          `json:"plan_id"`
		Status          string          `json:"status"`
		ConnectionLimit int             `json:"connection_limit"`
		RadiusChecks    json.RawMessage `json:"radius_checks"`
		RadiusReplies   json.RawMessage `json:"radius_replies"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "name is required")
		return
	}

	// Default status to "active" if not provided
	if in.Status == "" {
		in.Status = "active"
	}
	if in.Status != "active" && in.Status != "disabled" {
		writeError(w, http.StatusBadRequest, "bad_request", "status must be 'active' or 'disabled'")
		return
	}

	if in.ConnectionLimit < 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "connection_limit must be >= 0")
		return
	}

	// Check unique name (among non-deleted templates)
	var exists int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM user_templates WHERE name = $1 AND deleted_at IS NULL`, in.Name).Scan(&exists)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if exists > 0 {
		writeError(w, http.StatusConflict, "duplicate_name", "a template with this name already exists")
		return
	}

	// Validate plan_id exists if provided
	if in.PlanID != nil {
		var planExists int
		err := s.DB.QueryRow(`SELECT COUNT(*) FROM plans WHERE id = $1`, *in.PlanID).Scan(&planExists)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		if planExists == 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "plan_id does not exist")
			return
		}
	}

	// Default JSON arrays to null if not provided
	radiusChecks := nullableJSON(in.RadiusChecks)
	radiusReplies := nullableJSON(in.RadiusReplies)

	actor, _, _ := s.currentAdmin(r)

	res, err := s.DB.Exec(`INSERT INTO user_templates(name, plan_id, status, connection_limit, radius_checks, radius_replies, created_by) VALUES($1, $2, $3, $4, $5, $6, $7)`,
		in.Name, in.PlanID, in.Status, in.ConnectionLimit, radiusChecks, radiusReplies, actor)
	if err != nil {
		// Handle duplicate name race condition (UNIQUE constraint)
		if strings.Contains(err.Error(), "Duplicate") {
			writeError(w, http.StatusConflict, "duplicate_name", "a template with this name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	id, _ := res.LastInsertId()

	// Fetch the created template to return it
	row := s.DB.QueryRow(`SELECT id, name, plan_id, status, connection_limit, radius_checks, radius_replies, created_by, deleted_at, created_at, updated_at FROM user_templates WHERE id = $1`, id)
	t, err := scanTemplate(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	s.logAudit(actor, "template.created", "template", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "template": t})
}

// updateTemplate updates an existing non-deleted template.
func (s *Server) updateTemplate(w http.ResponseWriter, r *http.Request, id int64) {
	// Check that template exists and is not deleted
	var deletedAt sql.NullTime
	err := s.DB.QueryRow(`SELECT deleted_at FROM user_templates WHERE id = $1`, id).Scan(&deletedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "template not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if deletedAt.Valid {
		writeError(w, http.StatusNotFound, "not_found", "template not found")
		return
	}

	var in struct {
		Name            *string         `json:"name"`
		PlanID          *int64          `json:"plan_id"`
		Status          *string         `json:"status"`
		ConnectionLimit *int            `json:"connection_limit"`
		RadiusChecks    json.RawMessage `json:"radius_checks"`
		RadiusReplies   json.RawMessage `json:"radius_replies"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	// Build dynamic update
	setClauses := []string{}
	args := []any{}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "name cannot be empty")
			return
		}
		// Check unique name (excluding current template)
		var exists int
		err := s.DB.QueryRow(`SELECT COUNT(*) FROM user_templates WHERE name = $1 AND deleted_at IS NULL AND id != $2`, name, id).Scan(&exists)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		if exists > 0 {
			writeError(w, http.StatusConflict, "duplicate_name", "a template with this name already exists")
			return
		}
		setClauses = append(setClauses, "name = $1")
		args = append(args, name)
	}

	if in.PlanID != nil {
		// Validate plan exists
		var planExists int
		err := s.DB.QueryRow(`SELECT COUNT(*) FROM plans WHERE id = $1`, *in.PlanID).Scan(&planExists)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		if planExists == 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "plan_id does not exist")
			return
		}
		setClauses = append(setClauses, "plan_id = $1")
		args = append(args, *in.PlanID)
	}

	if in.Status != nil {
		if *in.Status != "active" && *in.Status != "disabled" {
			writeError(w, http.StatusBadRequest, "bad_request", "status must be 'active' or 'disabled'")
			return
		}
		setClauses = append(setClauses, "status = $1")
		args = append(args, *in.Status)
	}

	if in.ConnectionLimit != nil {
		if *in.ConnectionLimit < 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "connection_limit must be >= 0")
			return
		}
		setClauses = append(setClauses, "connection_limit = $1")
		args = append(args, *in.ConnectionLimit)
	}

	if in.RadiusChecks != nil {
		setClauses = append(setClauses, "radius_checks = $1")
		args = append(args, nullableJSON(in.RadiusChecks))
	}

	if in.RadiusReplies != nil {
		setClauses = append(setClauses, "radius_replies = $1")
		args = append(args, nullableJSON(in.RadiusReplies))
	}

	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "no fields to update")
		return
	}

	// Always update updated_at
	setClauses = append(setClauses, "updated_at = $1")
	args = append(args, time.Now().UTC())
	args = append(args, id)

	query := "UPDATE user_templates SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	if _, err := s.DB.Exec(query, args...); err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			writeError(w, http.StatusConflict, "duplicate_name", "a template with this name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Fetch updated template
	row := s.DB.QueryRow(`SELECT id, name, plan_id, status, connection_limit, radius_checks, radius_replies, created_by, deleted_at, created_at, updated_at FROM user_templates WHERE id = $1`, id)
	t, err := scanTemplate(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "template.updated", "template", strconv.FormatInt(id, 10), nil, map[string]any{"name": t.Name}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "template": t})
}

// deleteTemplate soft-deletes a template by setting deleted_at.
func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request, id int64) {
	res, err := s.DB.Exec(`UPDATE user_templates SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "not_found", "template not found")
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "template.deleted", "template", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

// scanTemplate scans a single row into a UserTemplate struct.
func scanTemplate(scanner interface{ Scan(...any) error }) (UserTemplate, error) {
	var t UserTemplate
	var planID sql.NullInt64
	var deletedAt sql.NullString
	var radiusChecks, radiusReplies []byte

	err := scanner.Scan(
		&t.ID,
		&t.Name,
		&planID,
		&t.Status,
		&t.ConnectionLimit,
		&radiusChecks,
		&radiusReplies,
		&t.CreatedBy,
		&deletedAt,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err != nil {
		return t, err
	}

	if planID.Valid {
		t.PlanID = &planID.Int64
	}
	if deletedAt.Valid {
		t.DeletedAt = &deletedAt.String
	}
	if radiusChecks != nil {
		t.RadiusChecks = json.RawMessage(radiusChecks)
	}
	if radiusReplies != nil {
		t.RadiusReplies = json.RawMessage(radiusReplies)
	}

	return t, nil
}

// nullableJSON returns nil if the input is empty or "null", otherwise returns the raw bytes.
func nullableJSON(data json.RawMessage) any {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	return []byte(data)
}

// radiusAttr represents a single RADIUS attribute from a template's radius_checks or radius_replies JSON array.
type radiusAttr struct {
	Attribute string `json:"attribute"`
	Op        string `json:"op"`
	Value     string `json:"value"`
}
