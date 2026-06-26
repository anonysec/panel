//go:build !lite

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// SegmentRules represents the JSON rules structure stored in user_segments.rules_json.
type SegmentRules struct {
	Conditions []SegmentCondition `json:"conditions"`
}

// SegmentCondition represents a single rule condition for segment matching.
type SegmentCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

// allowedSegmentFields restricts which columns can be used in segment rules.
var allowedSegmentFields = map[string]string{
	"status":  "c.status",
	"plan_id": "c.plan_id",
}

// allowedSegmentOperators maps operator names to SQL operators.
var allowedSegmentOperators = map[string]string{
	"eq":  "=",
	"neq": "!=",
	"gt":  ">",
	"gte": ">=",
	"lt":  "<",
	"lte": "<=",
}

// adminSegments handles GET (list) and POST (create) for user segments.
// Route: /api/admin/segments
func (s *Server) adminSegments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listSegments(w, r)
	case http.MethodPost:
		s.createSegment(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// adminSegmentByID handles DELETE /api/admin/segments/:id and sub-routes like /customers and /refresh.
// Route: /api/admin/segments/{id}[/{action}]
func (s *Server) adminSegmentByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/admin/segments/")
	if !ok {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_id"})
		return
	}

	switch action {
	case "":
		if r.Method != http.MethodDelete {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.deleteSegment(w, r, id)
	case "customers":
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.segmentCustomers(w, r, id)
	case "refresh":
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.refreshSegment(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) listSegments(w http.ResponseWriter, _ *http.Request) {
	rows, err := s.DB.Query(`SELECT id, name, COALESCE(description,''), rules_json, customer_count, created_at, updated_at FROM user_segments ORDER BY id`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type Segment struct {
		ID            int64           `json:"id"`
		Name          string          `json:"name"`
		Description   string          `json:"description"`
		Rules         json.RawMessage `json:"rules"`
		CustomerCount int             `json:"customer_count"`
		CreatedAt     string          `json:"created_at"`
		UpdatedAt     string          `json:"updated_at"`
	}

	segments := []Segment{}
	for rows.Next() {
		var seg Segment
		if err := rows.Scan(&seg.ID, &seg.Name, &seg.Description, &seg.Rules, &seg.CustomerCount, &seg.CreatedAt, &seg.UpdatedAt); err != nil {
			continue
		}
		segments = append(segments, seg)
	}
	writeJSON(w, map[string]any{"ok": true, "segments": segments})
}

func (s *Server) createSegment(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Rules       json.RawMessage `json:"rules"`
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
	if len(in.Rules) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "rules_required"})
		return
	}

	// Validate rules structure
	var rules SegmentRules
	if err := json.Unmarshal(in.Rules, &rules); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_rules"})
		return
	}
	if len(rules.Conditions) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "conditions_required"})
		return
	}
	for _, cond := range rules.Conditions {
		if _, ok := allowedSegmentFields[cond.Field]; !ok {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_field"})
			return
		}
		if _, ok := allowedSegmentOperators[cond.Operator]; !ok {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_operator"})
			return
		}
		if cond.Value == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "value_required"})
			return
		}
	}

	res, err := s.DB.Exec(`INSERT INTO user_segments(name, description, rules_json) VALUES($1,$2,$3)`,
		in.Name, in.Description, string(in.Rules))
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	id, _ := res.LastInsertId()

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "segment.created", "segment", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) deleteSegment(w http.ResponseWriter, r *http.Request, id int64) {
	res, err := s.DB.Exec(`DELETE FROM user_segments WHERE id=$1`, id)
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
	s.logAudit(actor, "segment.deleted", "segment", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

// segmentCustomers evaluates segment rules dynamically and returns matching customers.
func (s *Server) segmentCustomers(w http.ResponseWriter, _ *http.Request, id int64) {
	// Load segment rules
	var rulesJSON string
	err := s.DB.QueryRow(`SELECT rules_json FROM user_segments WHERE id=$1`, id).Scan(&rulesJSON)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	var rules SegmentRules
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "invalid_rules"})
		return
	}

	whereClause, params, err := buildSegmentWhere(rules)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "invalid_rules"})
		return
	}

	query := fmt.Sprintf(`SELECT c.id, c.username, COALESCE(c.display_name,''), c.status, COALESCE(c.plan_id,0), c.created_at FROM customers c WHERE c.deleted_at IS NULL AND %s ORDER BY c.id`, whereClause)
	rows, err := s.DB.Query(query, params...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type SegmentCustomer struct {
		ID          int64  `json:"id"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Status      string `json:"status"`
		PlanID      int64  `json:"plan_id"`
		CreatedAt   string `json:"created_at"`
	}

	customers := []SegmentCustomer{}
	for rows.Next() {
		var c SegmentCustomer
		if err := rows.Scan(&c.ID, &c.Username, &c.DisplayName, &c.Status, &c.PlanID, &c.CreatedAt); err != nil {
			continue
		}
		customers = append(customers, c)
	}
	writeJSON(w, map[string]any{"ok": true, "customers": customers, "count": len(customers)})
}

// refreshSegment recalculates the customer_count for a segment.
func (s *Server) refreshSegment(w http.ResponseWriter, r *http.Request, id int64) {
	// Load segment rules
	var rulesJSON string
	err := s.DB.QueryRow(`SELECT rules_json FROM user_segments WHERE id=$1`, id).Scan(&rulesJSON)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	var rules SegmentRules
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "invalid_rules"})
		return
	}

	whereClause, params, err := buildSegmentWhere(rules)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "invalid_rules"})
		return
	}

	query := fmt.Sprintf(`SELECT COUNT(*) FROM customers c WHERE c.deleted_at IS NULL AND %s`, whereClause)
	var count int
	if err := s.DB.QueryRow(query, params...).Scan(&count); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	_, err = s.DB.Exec(`UPDATE user_segments SET customer_count=$1 WHERE id=$2`, count, id)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "segment.refreshed", "segment", strconv.FormatInt(id, 10), nil, map[string]any{"customer_count": count}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "customer_count": count})
}

// buildSegmentWhere constructs a WHERE clause from segment conditions using parameterized queries.
func buildSegmentWhere(rules SegmentRules) (string, []any, error) {
	if len(rules.Conditions) == 0 {
		return "1=1", nil, nil
	}

	clauses := make([]string, 0, len(rules.Conditions))
	params := make([]any, 0, len(rules.Conditions))

	for _, cond := range rules.Conditions {
		col, ok := allowedSegmentFields[cond.Field]
		if !ok {
			return "", nil, fmt.Errorf("invalid field: %s", cond.Field)
		}
		op, ok := allowedSegmentOperators[cond.Operator]
		if !ok {
			return "", nil, fmt.Errorf("invalid operator: %s", cond.Operator)
		}

		clauses = append(clauses, fmt.Sprintf("%s %s ?", col, op))
		params = append(params, cond.Value)
	}

	return strings.Join(clauses, " AND "), params, nil
}
