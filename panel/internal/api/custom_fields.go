//go:build !lite

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// adminCustomFields handles GET (list) and POST (create) for custom field definitions.
func (s *Server) adminCustomFields(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listCustomFields(w, r)
	case http.MethodPost:
		s.createCustomField(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// adminCustomFieldByID handles DELETE /api/admin/custom-fields/:id
func (s *Server) adminCustomFieldByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	// Extract ID from path: /api/admin/custom-fields/{id}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/admin/custom-fields/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_id"})
		return
	}

	res, err := s.DB.Exec(`DELETE FROM custom_fields WHERE id=$1`, id)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "custom_field.deleted", "custom_field", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) listCustomFields(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(`SELECT id, field_name, field_type, COALESCE(field_options,''), required, display_order, created_at FROM custom_fields ORDER BY display_order, id`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type CustomField struct {
		ID           int64  `json:"id"`
		FieldName    string `json:"field_name"`
		FieldType    string `json:"field_type"`
		FieldOptions string `json:"field_options"`
		Required     bool   `json:"required"`
		DisplayOrder int    `json:"display_order"`
		CreatedAt    string `json:"created_at"`
	}

	fields := []CustomField{}
	for rows.Next() {
		var f CustomField
		if err := rows.Scan(&f.ID, &f.FieldName, &f.FieldType, &f.FieldOptions, &f.Required, &f.DisplayOrder, &f.CreatedAt); err != nil {
			continue
		}
		fields = append(fields, f)
	}
	writeJSON(w, map[string]any{"ok": true, "fields": fields})
}

func (s *Server) createCustomField(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		FieldName    string `json:"field_name"`
		FieldType    string `json:"field_type"`
		FieldOptions string `json:"field_options"`
		Required     bool   `json:"required"`
		DisplayOrder int    `json:"display_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.FieldName = strings.TrimSpace(in.FieldName)
	if in.FieldName == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "field_name_required"})
		return
	}

	validTypes := map[string]bool{"text": true, "number": true, "boolean": true, "date": true, "select": true}
	if in.FieldType == "" {
		in.FieldType = "text"
	}
	if !validTypes[in.FieldType] {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_field_type"})
		return
	}

	res, err := s.DB.Exec(`INSERT INTO custom_fields(field_name, field_type, field_options, required, display_order) VALUES($1,$2,$3,$4,$5)`,
		in.FieldName, in.FieldType, in.FieldOptions, in.Required, in.DisplayOrder)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "duplicate_field_name"})
			return
		}
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	id, _ := res.LastInsertId()

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "custom_field.created", "custom_field", strconv.FormatInt(id, 10), nil, map[string]any{"field_name": in.FieldName, "field_type": in.FieldType}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// adminCustomerCustomFields handles GET and POST for per-customer custom field values.
// Route: /api/admin/customers/{id}/custom-fields
func (s *Server) adminCustomerCustomFields(w http.ResponseWriter, r *http.Request, customerID int64) {
	switch r.Method {
	case http.MethodGet:
		s.getCustomerCustomFields(w, r, customerID)
	case http.MethodPost:
		s.setCustomerCustomFields(w, r, customerID)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// adminCustomerSubresource routes /api/admin/customers/{id}/{action} subresources.
func (s *Server) adminCustomerSubresource(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/admin/customers/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch action {
	case "custom-fields":
		s.adminCustomerCustomFields(w, r, id)
	case "notes":
		s.adminCustomerNotes(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) getCustomerCustomFields(w http.ResponseWriter, r *http.Request, customerID int64) {
	// Verify customer exists
	var exists int
	if err := s.DB.QueryRow(`SELECT 1 FROM customers WHERE id=$1 AND deleted_at IS NULL`, customerID).Scan(&exists); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	rows, err := s.DB.Query(`
		SELECT cf.id, cf.field_name, cf.field_type, COALESCE(cf.field_options,''), cf.required, cf.display_order,
		       COALESCE(ccv.field_value,'')
		FROM custom_fields cf
		LEFT JOIN customer_custom_values ccv ON ccv.field_id=cf.id AND ccv.customer_id=$1
		ORDER BY cf.display_order, cf.id`, customerID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type CustomerFieldValue struct {
		FieldID      int64  `json:"field_id"`
		FieldName    string `json:"field_name"`
		FieldType    string `json:"field_type"`
		FieldOptions string `json:"field_options"`
		Required     bool   `json:"required"`
		DisplayOrder int    `json:"display_order"`
		Value        string `json:"value"`
	}

	values := []CustomerFieldValue{}
	for rows.Next() {
		var v CustomerFieldValue
		if err := rows.Scan(&v.FieldID, &v.FieldName, &v.FieldType, &v.FieldOptions, &v.Required, &v.DisplayOrder, &v.Value); err != nil {
			continue
		}
		values = append(values, v)
	}
	writeJSON(w, map[string]any{"ok": true, "fields": values})
}

func (s *Server) setCustomerCustomFields(w http.ResponseWriter, r *http.Request, customerID int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Fields map[string]string `json:"fields"` // field_id (as string key) -> value
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if len(in.Fields) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "fields_required"})
		return
	}

	// Verify customer exists
	var exists int
	if err := s.DB.QueryRow(`SELECT 1 FROM customers WHERE id=$1 AND deleted_at IS NULL`, customerID).Scan(&exists); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	for fieldIDStr, value := range in.Fields {
		fieldID, err := strconv.ParseInt(fieldIDStr, 10, 64)
		if err != nil || fieldID <= 0 {
			continue
		}
		_, err = s.DB.Exec(`INSERT INTO customer_custom_values(customer_id, field_id, field_value) VALUES($1,$2,$3) ON CONFLICT (customer_id, field_id) DO UPDATE SET field_value = EXCLUDED.field_value`,
			customerID, fieldID, value)
		if err != nil {
			// Skip invalid field IDs (FK constraint will reject them)
			continue
		}
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "customer.custom_fields_updated", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{"fields_count": len(in.Fields)}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}
