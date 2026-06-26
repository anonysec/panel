//go:build !lite

package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ─── Tag CRUD ────────────────────────────────────────────────────────────────

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listTags(w, r)
	case http.MethodPost:
		s.createTag(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTagByID(w http.ResponseWriter, r *http.Request) {
	id, _, ok := pathID(r.URL.Path, "/api/tags/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch r.Method {
	case http.MethodDelete:
		s.deleteTag(w, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listTags(w http.ResponseWriter, _ *http.Request) {
	rows, err := s.DB.Query("SELECT id, name, color, created_at FROM user_tags ORDER BY name")
	if err != nil {
		log.Printf("[usertags] list query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type tag struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		Color     string `json:"color"`
		CreatedAt string `json:"created_at"`
	}
	var tags []tag
	for rows.Next() {
		var t tag
		var createdAt time.Time
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &createdAt); err != nil {
			log.Printf("[usertags] scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		t.CreatedAt = createdAt.Format(time.RFC3339)
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []tag{}
	}
	writeJSON(w, map[string]any{"ok": true, "tags": tags})
}

func (s *Server) createTag(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
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
	if in.Color == "" {
		in.Color = "#3b82f6"
	}

	result, err := s.DB.Exec("INSERT INTO user_tags (name, color) VALUES (?, ?)", in.Name, in.Color)
	if err != nil {
		log.Printf("[usertags] insert failed: %v", err)
		if strings.Contains(err.Error(), "Duplicate") {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "tag_already_exists"})
			return
		}
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) deleteTag(w http.ResponseWriter, id int64) {
	// CASCADE on FK will remove customer_tags associations automatically
	result, err := s.DB.Exec("DELETE FROM user_tags WHERE id = ?", id)
	if err != nil {
		log.Printf("[usertags] delete failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ─── Tag Assignment ──────────────────────────────────────────────────────────

func (s *Server) handleCustomerTags(w http.ResponseWriter, r *http.Request, customerID int64) {
	switch r.Method {
	case http.MethodPost:
		s.assignCustomerTags(w, r, customerID)
	case http.MethodDelete:
		s.removeCustomerTag(w, r, customerID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) assignCustomerTags(w http.ResponseWriter, r *http.Request, customerID int64) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		TagIDs []int64 `json:"tag_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if len(in.TagIDs) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "tag_ids_required"})
		return
	}

	// Verify customer exists
	var exists int
	if err := s.DB.QueryRow("SELECT 1 FROM customers WHERE id = ? AND deleted_at IS NULL", customerID).Scan(&exists); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}

	// INSERT IGNORE to avoid duplicates
	for _, tagID := range in.TagIDs {
		_, err := s.DB.Exec("INSERT INTO customer_tags (customer_id, tag_id) VALUES ($1, $2) ON CONFLICT (customer_id, tag_id) DO NOTHING", customerID, tagID)
		if err != nil {
			log.Printf("[usertags] assign tag %d to customer %d failed: %v", tagID, customerID, err)
		}
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) removeCustomerTag(w http.ResponseWriter, r *http.Request, customerID int64) {
	// Parse tag ID from the URL path: /api/customers/{id}/tags/{tagId}
	// The full path looks like /api/customers/5/tags/3
	// We need to extract the tagId from the remainder of the path
	rest := strings.TrimPrefix(r.URL.Path, "/api/customers/")
	parts := strings.Split(rest, "/")
	// parts should be: ["5", "tags", "3"]
	if len(parts) < 3 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "tag_id_required"})
		return
	}
	tagID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || tagID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_tag_id"})
		return
	}

	result, err2 := s.DB.Exec("DELETE FROM customer_tags WHERE customer_id = ? AND tag_id = ?", customerID, tagID)
	if err2 != nil {
		log.Printf("[usertags] remove tag %d from customer %d failed: %v", tagID, customerID, err2)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ─── Filter Presets ──────────────────────────────────────────────────────────

func (s *Server) handleFilterPresets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listFilterPresets(w, r)
	case http.MethodPost:
		s.createFilterPreset(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleFilterPresetByID(w http.ResponseWriter, r *http.Request) {
	id, _, ok := pathID(r.URL.Path, "/api/filter-presets/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch r.Method {
	case http.MethodDelete:
		s.deleteFilterPreset(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listFilterPresets(w http.ResponseWriter, r *http.Request) {
	admin, _, _ := s.currentAdmin(r)

	rows, err := s.DB.Query("SELECT id, name, filters_json, created_at FROM filter_presets WHERE admin_username = ? ORDER BY name", admin)
	if err != nil {
		log.Printf("[usertags] list presets failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type preset struct {
		ID        int64           `json:"id"`
		Name      string          `json:"name"`
		Filters   json.RawMessage `json:"filters"`
		CreatedAt string          `json:"created_at"`
	}
	var presets []preset
	for rows.Next() {
		var p preset
		var filtersJSON string
		var createdAt time.Time
		if err := rows.Scan(&p.ID, &p.Name, &filtersJSON, &createdAt); err != nil {
			log.Printf("[usertags] preset scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		p.Filters = json.RawMessage(filtersJSON)
		p.CreatedAt = createdAt.Format(time.RFC3339)
		presets = append(presets, p)
	}
	if presets == nil {
		presets = []preset{}
	}
	writeJSON(w, map[string]any{"ok": true, "presets": presets})
}

func (s *Server) createFilterPreset(w http.ResponseWriter, r *http.Request) {
	admin, _, _ := s.currentAdmin(r)

	limitBody(w, r, maxJSONBody)
	var in struct {
		Name    string          `json:"name"`
		Filters json.RawMessage `json:"filters"`
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
	if len(in.Filters) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "filters_required"})
		return
	}

	filtersJSON := string(in.Filters)
	result, err := s.DB.Exec("INSERT INTO filter_presets (admin_username, name, filters_json) VALUES (?, ?, ?)", admin, in.Name, filtersJSON)
	if err != nil {
		log.Printf("[usertags] create preset failed: %v", err)
		if strings.Contains(err.Error(), "Duplicate") {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "preset_already_exists"})
			return
		}
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) deleteFilterPreset(w http.ResponseWriter, r *http.Request, id int64) {
	admin, _, _ := s.currentAdmin(r)

	// Verify ownership by admin_username
	result, err := s.DB.Exec("DELETE FROM filter_presets WHERE id = ? AND admin_username = ?", id, admin)
	if err != nil {
		log.Printf("[usertags] delete preset failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// ─── Advanced Customer Filtering ─────────────────────────────────────────────

func (s *Server) handleCustomersFiltered(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.listCustomersFiltered(w, r)
}

func (s *Server) listCustomersFiltered(w http.ResponseWriter, r *http.Request) {
	actor, role, _ := s.currentAdmin(r)
	params := r.URL.Query()

	// --- Pagination ---
	page, _ := strconv.Atoi(params.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(params.Get("limit"))
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	// --- Build WHERE clause ---
	where := "c.deleted_at IS NULL"
	args := []any{}

	// Reseller scoping
	if role == "reseller" {
		where += " AND c.created_by = $1"
		args = append(args, actor)
	}

	// Search
	search := strings.TrimSpace(params.Get("search"))
	if search != "" {
		where += " AND (c.username LIKE $1 OR COALESCE(c.display_name,'') LIKE $2 OR COALESCE(c.email,'') LIKE $3)"
		like := "%" + search + "%"
		args = append(args, like, like, like)
	}

	// Filter: status (active/expired/disabled/suspended)
	if status := strings.TrimSpace(params.Get("status")); status != "" {
		where += " AND c.status = $1"
		args = append(args, status)
	}

	// Filter: plan (plan name or ID)
	if planParam := strings.TrimSpace(params.Get("plan")); planParam != "" {
		if pid, err := strconv.ParseInt(planParam, 10, 64); err == nil && pid > 0 {
			where += " AND c.plan_id = $1"
			args = append(args, pid)
		} else {
			where += " AND p.name = $1"
			args = append(args, planParam)
		}
	}

	// Filter: node_id
	if nodeIDStr := params.Get("node_id"); nodeIDStr != "" {
		if nid, err := strconv.ParseInt(nodeIDStr, 10, 64); err == nil && nid > 0 {
			where += " AND c.node_id = $1"
			args = append(args, nid)
		}
	}

	// Filter: group_id
	if groupIDStr := params.Get("group_id"); groupIDStr != "" {
		if gid, err := strconv.ParseInt(groupIDStr, 10, 64); err == nil && gid > 0 {
			where += " AND n.group_id = $1"
			args = append(args, gid)
		}
	}

	// Filter: creation date range
	if dateFrom := params.Get("date_from"); dateFrom != "" {
		if _, err := time.Parse("2006-01-02", dateFrom); err == nil {
			where += " AND c.created_at >= $1"
			args = append(args, dateFrom)
		}
	}
	if dateTo := params.Get("date_to"); dateTo != "" {
		if _, err := time.Parse("2006-01-02", dateTo); err == nil {
			where += " AND c.created_at <= $1"
			args = append(args, dateTo+" 23:59:59")
		}
	}

	// Filter: expiry date range (requires subscription join)
	needSubJoin := false
	if expiryFrom := params.Get("expiry_from"); expiryFrom != "" {
		if _, err := time.Parse("2006-01-02", expiryFrom); err == nil {
			needSubJoin = true
			where += " AND sub.expires_at >= $1"
			args = append(args, expiryFrom)
		}
	}
	if expiryTo := params.Get("expiry_to"); expiryTo != "" {
		if _, err := time.Parse("2006-01-02", expiryTo); err == nil {
			needSubJoin = true
			where += " AND sub.expires_at <= $1"
			args = append(args, expiryTo+" 23:59:59")
		}
	}

	// Filter: bandwidth usage percentage range
	needUsageJoin := false
	needPlanJoin := true // we always have plan join
	if bwMin := params.Get("bandwidth_min_pct"); bwMin != "" {
		if pct, err := strconv.ParseFloat(bwMin, 64); err == nil && pct >= 0 {
			needUsageJoin = true
			where += " AND (CASE WHEN p.data_gb > 0 THEN (COALESCE(ra.usage_bytes, 0) / (p.data_gb * 1073741824)) * 100 ELSE 0 END) >= $1"
			args = append(args, pct)
		}
	}
	if bwMax := params.Get("bandwidth_max_pct"); bwMax != "" {
		if pct, err := strconv.ParseFloat(bwMax, 64); err == nil && pct >= 0 {
			needUsageJoin = true
			where += " AND (CASE WHEN p.data_gb > 0 THEN (COALESCE(ra.usage_bytes, 0) / (p.data_gb * 1073741824)) * 100 ELSE 100 END) <= $1"
			args = append(args, pct)
		}
	}

	// Filter: tags (comma-separated IDs, AND logic — customer must have ALL specified tags)
	needTagJoin := false
	var tagIDs []int64
	if tagsParam := strings.TrimSpace(params.Get("tags")); tagsParam != "" {
		for _, t := range strings.Split(tagsParam, ",") {
			t = strings.TrimSpace(t)
			if tid, err := strconv.ParseInt(t, 10, 64); err == nil && tid > 0 {
				tagIDs = append(tagIDs, tid)
			}
		}
		if len(tagIDs) > 0 {
			needTagJoin = true
		}
	}

	// --- Build JOINs ---
	joins := " LEFT JOIN plans p ON p.id = c.plan_id"
	joins += " LEFT JOIN nodes n ON n.id = c.node_id"

	if needSubJoin {
		joins += ` LEFT JOIN (
			SELECT customer_id, MAX(expires_at) AS expires_at
			FROM subscriptions WHERE status = 'active'
			GROUP BY customer_id
		) sub ON sub.customer_id = c.id`
	}

	if needUsageJoin {
		joins += ` LEFT JOIN (
			SELECT username, COALESCE(SUM(acctinputoctets + acctoutputoctets), 0) AS usage_bytes
			FROM radacct GROUP BY username
		) ra ON ra.username = c.username`
	}

	if needTagJoin {
		// AND logic: customer must have ALL specified tags
		// Use subquery: customer_id IN (SELECT customer_id FROM customer_tags WHERE tag_id IN (...) GROUP BY customer_id HAVING COUNT(DISTINCT tag_id) = ?)
		placeholders := make([]string, len(tagIDs))
		for i, tid := range tagIDs {
			placeholders[i] = strconv.FormatInt(tid, 10)
		}
		where += fmt.Sprintf(` AND c.id IN (SELECT customer_id FROM customer_tags WHERE tag_id IN (%s) GROUP BY customer_id HAVING COUNT(DISTINCT tag_id) = $1)`, strings.Join(placeholders, ","))
		args = append(args, len(tagIDs))
	}

	_ = needPlanJoin // plan join is always added

	// --- Count query ---
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM customers c%s WHERE %s", joins, where)
	var total int
	if err := s.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		log.Printf("[usertags] filtered count query error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// --- Data query ---
	dataQuery := fmt.Sprintf(`SELECT c.id, c.username, COALESCE(c.display_name,''), c.status, c.plan_id, COALESCE(p.name,''), COALESCE(c.created_by,''), COALESCE(c.avatar,''), c.created_at
		FROM customers c%s
		WHERE %s
		ORDER BY c.created_at DESC
		LIMIT $1 OFFSET $2`, joins, where)

	dataArgs := append(args, limit, offset)
	rows, err := s.DB.Query(dataQuery, dataArgs...)
	if err != nil {
		log.Printf("[usertags] filtered list query error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type filteredCustomer struct {
		ID          int64  `json:"id"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Status      string `json:"status"`
		PlanID      *int64 `json:"plan_id,omitempty"`
		Plan        string `json:"plan"`
		CreatedBy   string `json:"created_by"`
		Avatar      string `json:"avatar"`
		CreatedAt   string `json:"created_at"`
	}

	var customers []filteredCustomer
	for rows.Next() {
		var c filteredCustomer
		var planID sql.NullInt64
		var createdAt sql.NullTime
		if err := rows.Scan(&c.ID, &c.Username, &c.DisplayName, &c.Status, &planID, &c.Plan, &c.CreatedBy, &c.Avatar, &createdAt); err != nil {
			log.Printf("[usertags] filtered scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		if planID.Valid {
			c.PlanID = &planID.Int64
		}
		if createdAt.Valid {
			c.CreatedAt = createdAt.Time.Format(time.RFC3339)
		}
		customers = append(customers, c)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[usertags] filtered rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	if customers == nil {
		customers = []filteredCustomer{}
	}

	writeJSON(w, map[string]any{
		"ok":        true,
		"customers": customers,
		"total":     total,
		"count":     total,
		"page":      page,
		"limit":     limit,
	})
}
