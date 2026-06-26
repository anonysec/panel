package api

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) customers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listCustomers(w, r)
	case http.MethodPost:
		s.createCustomer(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listCustomers(w http.ResponseWriter, r *http.Request) {
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

	// Search — ?search= or legacy ?q= (search across username, email, notes, display_name)
	search := strings.TrimSpace(params.Get("search"))
	if search == "" {
		search = strings.TrimSpace(params.Get("q"))
	}
	if search != "" {
		where += " AND (c.username LIKE $1 OR COALESCE(c.display_name,'') LIKE $2 OR COALESCE(c.email,'') LIKE $3 OR COALESCE(c.notes,'') LIKE $4 OR COALESCE(p.name,'') LIKE $5 OR CAST(c.id AS CHAR) LIKE $6)"
		like := "%" + search + "%"
		args = append(args, like, like, like, like, like, like)
	}

	// Filter: status
	if status := strings.TrimSpace(params.Get("status")); status != "" {
		where += " AND c.status = $1"
		args = append(args, status)
	}

	// Filter: plan_id
	if planIDStr := params.Get("plan_id"); planIDStr != "" {
		if pid, err := strconv.ParseInt(planIDStr, 10, 64); err == nil && pid > 0 {
			where += " AND c.plan_id = $1"
			args = append(args, pid)
		}
	}

	// Filter: created_after / created_before
	if ca := params.Get("created_after"); ca != "" {
		if _, err := time.Parse("2006-01-02", ca); err == nil {
			where += " AND c.created_at >= $1"
			args = append(args, ca)
		}
	}
	if cb := params.Get("created_before"); cb != "" {
		if _, err := time.Parse("2006-01-02", cb); err == nil {
			where += " AND c.created_at <= $1"
			args = append(args, cb+" 23:59:59")
		}
	}

	// Filter: expires_after / expires_before (requires subscription join)
	needSubJoin := false
	if ea := params.Get("expires_after"); ea != "" {
		if _, err := time.Parse("2006-01-02", ea); err == nil {
			needSubJoin = true
			where += " AND sub.expires_at >= $1"
			args = append(args, ea)
		}
	}
	if eb := params.Get("expires_before"); eb != "" {
		if _, err := time.Parse("2006-01-02", eb); err == nil {
			needSubJoin = true
			where += " AND sub.expires_at <= $1"
			args = append(args, eb+" 23:59:59")
		}
	}

	// --- Build sort clause ---
	orderBy := s.buildCustomerSortClause(params.Get("sort"))

	// --- Subscription join (for expires filter/sort) ---
	subJoin := ""
	if needSubJoin || strings.Contains(orderBy, "sub.") {
		subJoin = ` LEFT JOIN (
			SELECT customer_id, MAX(expires_at) AS expires_at
			FROM subscriptions WHERE status = 'active'
			GROUP BY customer_id
		) sub ON sub.customer_id = c.id`
	}

	// --- Data usage join (for sort by data_used) ---
	usageJoin := ""
	if strings.Contains(orderBy, "usage_bytes") {
		usageJoin = ` LEFT JOIN (
			SELECT username, COALESCE(SUM(acctinputoctets + acctoutputoctets), 0) AS usage_bytes
			FROM radacct GROUP BY username
		) ra ON ra.username = c.username`
	}

	// --- Count query ---
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM customers c
		LEFT JOIN plans p ON p.id=c.plan_id%s%s
		WHERE %s`, subJoin, usageJoin, where)

	var total int
	if err := s.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		log.Printf("[customers] count query error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// --- Data query ---
	dataQuery := fmt.Sprintf(`SELECT c.id,c.username,COALESCE(c.display_name,''),c.status,c.plan_id,COALESCE(p.name,''),COALESCE(w.credit,0),COALESCE(c.created_by,''),COALESCE(c.avatar, a.avatar, ''),c.created_at
		FROM customers c
		LEFT JOIN plans p ON p.id=c.plan_id
		LEFT JOIN wallets w ON w.username=c.username
		LEFT JOIN admins a ON a.username=c.created_by AND a.role='reseller'%s%s
		WHERE %s
		ORDER BY %s
		LIMIT $1 OFFSET $2`, subJoin, usageJoin, where, orderBy)

	dataArgs := append(args, limit, offset)
	rows, err := s.DB.Query(dataQuery, dataArgs...)
	if err != nil {
		log.Printf("[customers] list query error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	out := []Customer{}
	for rows.Next() {
		var c Customer
		var planID sql.NullInt64
		var created sql.NullTime
		if err := rows.Scan(&c.ID, &c.Username, &c.DisplayName, &c.Status, &planID, &c.Plan, &c.Credit, &c.CreatedBy, &c.Avatar, &created); err != nil {
			log.Printf("[customers] scan error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		if planID.Valid {
			c.PlanID = &planID.Int64
		}
		if created.Valid {
			c.CreatedAt = created.Time.Format(time.RFC3339)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[customers] rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	writeJSON(w, map[string]any{
		"ok":        true,
		"customers": out,
		"total":     total,
		"page":      page,
		"limit":     limit,
	})
}

// allowedCustomerSortFields maps user-facing sort field names to SQL expressions.
var allowedCustomerSortFields = map[string]string{
	"username":   "c.username",
	"created_at": "c.created_at",
	"expires_at": "sub.expires_at",
	"status":     "c.status",
	"data_used":  "ra.usage_bytes",
}

// buildCustomerSortClause parses the sort param (e.g. "username:asc,created_at:desc")
// and returns a safe ORDER BY expression. Falls back to "c.created_at DESC".
func (s *Server) buildCustomerSortClause(sortParam string) string {
	sortParam = strings.TrimSpace(sortParam)
	if sortParam == "" {
		return "c.created_at DESC"
	}

	parts := strings.Split(sortParam, ",")
	clauses := []string{}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tokens := strings.SplitN(part, ":", 2)
		field := strings.TrimSpace(tokens[0])
		dir := "ASC"
		if len(tokens) == 2 {
			d := strings.ToUpper(strings.TrimSpace(tokens[1]))
			if d == "DESC" {
				dir = "DESC"
			}
		}
		col, ok := allowedCustomerSortFields[field]
		if !ok {
			continue
		}
		clauses = append(clauses, col+" "+dir)
	}

	if len(clauses) == 0 {
		return "c.created_at DESC"
	}
	return strings.Join(clauses, ", ")
}

func (s *Server) deletedCustomers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`SELECT c.id,c.username,COALESCE(c.display_name,''),c.status,c.plan_id,COALESCE(p.name,''),COALESCE(w.credit,0),c.created_at,c.deleted_at
		FROM customers c
		LEFT JOIN plans p ON p.id=c.plan_id
		LEFT JOIN wallets w ON w.username=c.username
		WHERE c.deleted_at IS NOT NULL
		ORDER BY c.deleted_at DESC LIMIT 500`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	out := []DeletedCustomer{}
	for rows.Next() {
		var c DeletedCustomer
		var planID sql.NullInt64
		var created, deleted sql.NullTime
		if err := rows.Scan(&c.ID, &c.Username, &c.DisplayName, &c.Status, &planID, &c.Plan, &c.Credit, &created, &deleted); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if planID.Valid {
			c.PlanID = &planID.Int64
		}
		if created.Valid {
			c.CreatedAt = created.Time.Format(time.RFC3339)
		}
		if deleted.Valid {
			c.DeletedAt = deleted.Time.Format(time.RFC3339)
		}
		out = append(out, c)
	}
	writeJSON(w, map[string]any{"ok": true, "customers": out})
}
