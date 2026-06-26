package api

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"KorisPanel/panel/internal/auth"
)

// handleCustomerExport handles GET /api/customers/export.
// Exports customer list as CSV with headers:
// username, email, phone, status, plan, node, tags, created_at, expires_at
// Supports same filters as the list endpoint.
func (s *Server) handleCustomerExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	params := r.URL.Query()

	// --- Build WHERE clause ---
	where := "c.deleted_at IS NULL"
	args := []any{}

	if search := strings.TrimSpace(params.Get("search")); search != "" {
		where += " AND (c.username LIKE $1 OR COALESCE(c.display_name,'') LIKE $2 OR COALESCE(c.email,'') LIKE $3)"
		like := "%" + search + "%"
		args = append(args, like, like, like)
	}
	if status := strings.TrimSpace(params.Get("status")); status != "" {
		where += " AND c.status = $1"
		args = append(args, status)
	}
	if planIDStr := params.Get("plan_id"); planIDStr != "" {
		if pid, err := strconv.ParseInt(planIDStr, 10, 64); err == nil && pid > 0 {
			where += " AND c.plan_id = $1"
			args = append(args, pid)
		}
	}

	query := fmt.Sprintf(`SELECT c.id, c.username, COALESCE(c.email,''), c.status, COALESCE(p.name,''),
		COALESCE(n.name,''), c.created_at,
		(SELECT MAX(sub.expires_at) FROM subscriptions sub WHERE sub.customer_id = c.id AND sub.status='active') AS expires_at
		FROM customers c
		LEFT JOIN plans p ON p.id = c.plan_id
		LEFT JOIN nodes n ON n.id = c.preferred_node_id
		WHERE %s
		ORDER BY c.id DESC
		LIMIT %d`, where, maxExportRows)

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		log.Printf("[customers] export query error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	// Build a map of customer_id -> tag names
	tagMap, err := s.loadCustomerTagsMap()
	if err != nil {
		log.Printf("[customers] export tags error: %v", err)
		// Continue without tags
		tagMap = make(map[int64][]string)
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="customers_export.csv"`)

	// Write UTF-8 BOM for Excel compatibility
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"username", "email", "phone", "status", "plan", "node", "tags", "created_at", "expires_at"})

	for rows.Next() {
		var id int64
		var username, email, status, planName, nodeName string
		var createdAt, expiresAt nullableTime
		if err := rows.Scan(&id, &username, &email, &status, &planName, &nodeName, &createdAt, &expiresAt); err != nil {
			continue
		}

		createdStr := ""
		if createdAt.Valid {
			createdStr = createdAt.Time.UTC().Format(time.RFC3339)
		}
		expiresStr := ""
		if expiresAt.Valid {
			expiresStr = expiresAt.Time.UTC().Format(time.RFC3339)
		}

		// Tags: comma-separated
		tags := ""
		if tagNames, ok := tagMap[id]; ok && len(tagNames) > 0 {
			tags = strings.Join(tagNames, ",")
		}

		_ = cw.Write([]string{
			username,
			email,
			"", // phone - not stored in DB, placeholder for import/export round-trip
			status,
			planName,
			nodeName,
			tags,
			createdStr,
			expiresStr,
		})
	}
	cw.Flush()
}

// loadCustomerTagsMap loads all customer->tag associations into a map.
func (s *Server) loadCustomerTagsMap() (map[int64][]string, error) {
	tagMap := make(map[int64][]string)

	rows, err := s.DB.Query(`SELECT ct.customer_id, ut.name FROM customer_tags ct JOIN user_tags ut ON ut.id = ct.tag_id`)
	if err != nil {
		return tagMap, err
	}
	defer rows.Close()

	for rows.Next() {
		var customerID int64
		var tagName string
		if err := rows.Scan(&customerID, &tagName); err != nil {
			continue
		}
		tagMap[customerID] = append(tagMap[customerID], tagName)
	}
	return tagMap, rows.Err()
}

// handleImportPreview handles POST /api/customers/import/preview.
// Validates a CSV file and returns a preview without actually importing.
func (s *Server) handleImportPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	csvReader, err := s.getImportCSVReader(r)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Read header
	header, err := csvReader.Read()
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_csv"})
		return
	}

	colMap := mapColumns(header)

	// Require at least username column
	if _, ok := colMap["username"]; !ok {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_column_username"})
		return
	}

	var totalRows, validRows, invalidRows int
	var errors []map[string]any
	var preview []map[string]string

	lineNum := 1 // header is line 1

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		lineNum++
		totalRows++

		if err != nil {
			invalidRows++
			errors = append(errors, map[string]any{"line": lineNum, "error": "malformed_csv_row"})
			continue
		}

		row := extractImportRow(record, colMap)

		// Validate
		if rowErr := validateImportRow(row); rowErr != "" {
			invalidRows++
			errors = append(errors, map[string]any{"line": lineNum, "error": rowErr})
			continue
		}

		validRows++

		// Add to preview (first 10 rows)
		if len(preview) < 10 {
			previewRow := map[string]string{
				"username": row.username,
				"email":    row.email,
				"phone":    row.phone,
				"status":   row.status,
				"plan":     row.plan,
				"node":     row.node,
				"tags":     row.tags,
			}
			if row.password != "" {
				previewRow["password"] = "***"
			}
			preview = append(preview, previewRow)
		}
	}

	if totalRows == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "empty_csv"})
		return
	}

	writeJSON(w, map[string]any{
		"ok":         true,
		"total_rows": totalRows,
		"valid":      validRows,
		"invalid":    invalidRows,
		"preview":    preview,
		"errors":     errors,
	})
}

// handleCustomerImport handles POST /api/customers/import.
// Accepts CSV and creates customer accounts, skipping invalid rows.
func (s *Server) handleCustomerImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	csvReader, err := s.getImportCSVReader(r)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Read header
	header, err := csvReader.Read()
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_csv"})
		return
	}

	colMap := mapColumns(header)

	if _, ok := colMap["username"]; !ok {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_column_username"})
		return
	}

	// Pre-load plan name -> id map
	planIDMap := s.loadPlanNameMap()

	// Pre-load node name -> id map
	nodeIDMap := s.loadNodeNameMap()

	// Pre-load tag name -> id map
	tagIDMap := s.loadTagNameMap()

	actor, _, _ := s.currentAdmin(r)
	ip := clientIP(r)

	var created, updated, skipped int
	var errors []map[string]any
	lineNum := 1 // header is line 1

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		lineNum++

		if err != nil {
			skipped++
			errors = append(errors, map[string]any{"line": lineNum, "error": "malformed_csv_row"})
			continue
		}

		row := extractImportRow(record, colMap)

		// Validate
		if rowErr := validateImportRow(row); rowErr != "" {
			skipped++
			errors = append(errors, map[string]any{"line": lineNum, "error": rowErr})
			continue
		}

		// Check username uniqueness
		var existingID int64
		err = s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 AND deleted_at IS NULL LIMIT 1`, row.username).Scan(&existingID)
		if err == nil {
			// Username already exists — skip
			skipped++
			errors = append(errors, map[string]any{"line": lineNum, "error": "username_exists"})
			continue
		}

		// Determine password
		password := row.password
		if password == "" {
			password = auth.RandomToken(12) // auto-generate
		}

		// Validate password can be hashed (ensures bcrypt compatibility)
		if _, err := auth.HashPassword(password); err != nil {
			skipped++
			errors = append(errors, map[string]any{"line": lineNum, "error": "hash_error"})
			continue
		}

		// Resolve plan_id
		var planID *int64
		if row.plan != "" {
			if pid, ok := planIDMap[strings.ToLower(row.plan)]; ok {
				planID = &pid
			}
		}

		// Resolve node_id
		var nodeID *int64
		if row.node != "" {
			if nid, ok := nodeIDMap[strings.ToLower(row.node)]; ok {
				nodeID = &nid
			}
		}

		// Determine status
		status := row.status
		if status == "" {
			status = "active"
		}
		if !validCustomerStatus(status) {
			status = "active"
		}

		// Insert customer
		res, err := s.DB.Exec(
			`INSERT INTO customers(username, email, plan_id, preferred_node_id, status, sub_token, created_by) VALUES($1,$2,$3,$4,$5,$6,$7)`,
			row.username, nullString(row.email), planID, nodeID, status, auth.RandomToken(24), actor,
		)
		if err != nil {
			skipped++
			errors = append(errors, map[string]any{"line": lineNum, "error": fmt.Sprintf("insert_error: %v", err)})
			continue
		}

		customerID, _ := res.LastInsertId()

		// Insert wallet
		_, _ = s.DB.Exec(`INSERT INTO wallets(customer_id, username, credit) VALUES($1,$2,0)`, customerID, row.username)

		// Insert radcheck password
		_, _ = s.DB.Exec(`INSERT INTO radcheck(username, attribute, op, value) VALUES($1,'Cleartext-Password',':=',$2)`, row.username, password)

		// Insert radcheck simultaneous-use
		_, _ = s.DB.Exec(`INSERT INTO radcheck(username, attribute, op, value) VALUES($1,'Simultaneous-Use',':=','1')`, row.username)

		// Assign tags
		if row.tags != "" {
			tagNames := strings.Split(row.tags, ",")
			for _, tn := range tagNames {
				tn = strings.TrimSpace(tn)
				if tn == "" {
					continue
				}
				if tagID, ok := tagIDMap[strings.ToLower(tn)]; ok {
					_, _ = s.DB.Exec(`INSERT INTO customer_tags(customer_id, tag_id) VALUES($1,$2) ON CONFLICT (customer_id, tag_id) DO NOTHING`, customerID, tagID)
				}
			}
		}

		created++
	}

	// Audit log
	s.logAudit(actor, "customers.import", "customer", "", nil, map[string]any{
		"created": created,
		"updated": updated,
		"skipped": skipped,
		"errors":  len(errors),
	}, ip)

	log.Printf("[customers] CSV import: created=%d updated=%d skipped=%d errors=%d by=%s", created, updated, skipped, len(errors), actor)

	writeJSON(w, map[string]any{
		"ok":      true,
		"created": created,
		"updated": updated,
		"skipped": skipped,
		"errors":  errors,
	})
}

// --- Helper types and functions ---

// nullableTime wraps sql.NullTime for scanning nullable datetime columns.
type nullableTime struct {
	Time  time.Time
	Valid bool
}

func (nt *nullableTime) Scan(value any) error {
	if value == nil {
		nt.Valid = false
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		nt.Time = v
		nt.Valid = true
	case []byte:
		t, err := time.Parse("2006-01-02 15:04:05", string(v))
		if err != nil {
			nt.Valid = false
			return nil
		}
		nt.Time = t
		nt.Valid = true
	case string:
		t, err := time.Parse("2006-01-02 15:04:05", v)
		if err != nil {
			nt.Valid = false
			return nil
		}
		nt.Time = t
		nt.Valid = true
	default:
		nt.Valid = false
	}
	return nil
}

// importRowData holds parsed CSV row data.
type importRowData struct {
	username string
	password string
	email    string
	phone    string
	status   string
	plan     string
	node     string
	tags     string
}

// getImportCSVReader extracts CSV content from the request.
// Supports multipart form upload (field "file") or JSON body with csv_data string.
func (s *Server) getImportCSVReader(r *http.Request) (*csv.Reader, error) {
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/") {
		r.Body = http.MaxBytesReader(nil, r.Body, maxImportFileSize)
		if err := r.ParseMultipartForm(maxImportFileSize); err != nil {
			return nil, fmt.Errorf("file_too_large")
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			return nil, fmt.Errorf("file_required")
		}
		// Read all content to handle BOM
		data, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("read_error")
		}
		data = stripUTF8BOM(data)
		reader := csv.NewReader(bytes.NewReader(data))
		reader.TrimLeadingSpace = true
		reader.LazyQuotes = true
		return reader, nil
	}

	// JSON body with csv_data
	r.Body = http.MaxBytesReader(nil, r.Body, maxImportFileSize)
	var in struct {
		CSVData string `json:"csv_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return nil, fmt.Errorf("bad_json")
	}
	if in.CSVData == "" {
		return nil, fmt.Errorf("csv_data_required")
	}

	data := stripUTF8BOM([]byte(in.CSVData))
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true
	return reader, nil
}

// stripUTF8BOM removes the UTF-8 BOM prefix if present.
func stripUTF8BOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

// mapColumns builds a lowercase column name -> index map from the header row.
func mapColumns(header []string) map[string]int {
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.TrimSpace(strings.ToLower(col))] = i
	}
	return colMap
}

// extractImportRow extracts field values from a CSV record using the column map.
func extractImportRow(record []string, colMap map[string]int) importRowData {
	get := func(name string) string {
		if idx, ok := colMap[name]; ok && idx < len(record) {
			return strings.TrimSpace(record[idx])
		}
		return ""
	}
	return importRowData{
		username: get("username"),
		password: get("password"),
		email:    get("email"),
		phone:    get("phone"),
		status:   get("status"),
		plan:     get("plan"),
		node:     get("node"),
		tags:     get("tags"),
	}
}

// validateImportRow validates a parsed import row. Returns error string or empty if valid.
func validateImportRow(row importRowData) string {
	if row.username == "" {
		return "username_required"
	}
	if !importUsernameRe.MatchString(row.username) {
		return "username_invalid"
	}
	if row.email != "" && !importEmailRe.MatchString(row.email) {
		return "email_invalid"
	}
	if row.password != "" && len(row.password) < 6 {
		return "password_too_short"
	}
	return ""
}

// loadPlanNameMap returns a map of lowercase plan name -> plan ID for active plans.
func (s *Server) loadPlanNameMap() map[string]int64 {
	m := make(map[string]int64)
	rows, err := s.DB.Query(`SELECT id, name FROM plans WHERE is_active=TRUE`)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if rows.Scan(&id, &name) == nil {
			m[strings.ToLower(name)] = id
		}
	}
	return m
}

// loadNodeNameMap returns a map of lowercase node name -> node ID.
func (s *Server) loadNodeNameMap() map[string]int64 {
	m := make(map[string]int64)
	rows, err := s.DB.Query(`SELECT id, name FROM nodes`)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if rows.Scan(&id, &name) == nil {
			m[strings.ToLower(name)] = id
		}
	}
	return m
}

// loadTagNameMap returns a map of lowercase tag name -> tag ID.
func (s *Server) loadTagNameMap() map[string]int64 {
	m := make(map[string]int64)
	rows, err := s.DB.Query(`SELECT id, name FROM user_tags`)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if rows.Scan(&id, &name) == nil {
			m[strings.ToLower(name)] = id
		}
	}
	return m
}
