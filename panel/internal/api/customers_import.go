package api

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"KorisPanel/panel/internal/auth"
)

const (
	maxImportFileSize = 2 << 20 // 2MB
	maxImportRows     = 500
)

// importUsernameRe matches 3-32 chars: alphanumeric + underscore.
var importUsernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)

// importEmailRe is a basic email format check.
var importEmailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// ImportRowError represents a validation error for a specific CSV row.
type ImportRowError struct {
	Row   int    `json:"row"`
	Error string `json:"error"`
}

// adminCustomersImport handles POST /api/admin/customers/import.
// It accepts a multipart form upload with a CSV file, validates each row,
// and creates customers in a batch transaction.
func (s *Server) adminCustomersImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body to 2MB
	r.Body = http.MaxBytesReader(w, r.Body, maxImportFileSize)

	// Parse multipart form
	if err := r.ParseMultipartForm(maxImportFileSize); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "file_too_large"})
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "file_required"})
		return
	}
	defer file.Close()

	// Parse CSV
	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true

	// Read header row
	header, err := reader.Read()
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_csv"})
		return
	}

	// Map header columns to indices
	colMap := make(map[string]int)
	for i, col := range header {
		colMap[strings.TrimSpace(strings.ToLower(col))] = i
	}

	// Validate required header columns
	if _, ok := colMap["username"]; !ok {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_column_username"})
		return
	}
	if _, ok := colMap["password"]; !ok {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_column_password"})
		return
	}

	// Read all rows
	type importRow struct {
		Username    string
		Password    string
		Email       string
		PlanID      *int64
		DisplayName string
	}

	var rows []importRow
	var rowErrors []ImportRowError
	rowNum := 1 // start at 1 (header is row 0)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			rowErrors = append(rowErrors, ImportRowError{Row: rowNum + 1, Error: "malformed_csv_row"})
			rowNum++
			continue
		}
		rowNum++

		if rowNum > maxImportRows+1 { // +1 for header
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "too_many_rows", "max": maxImportRows})
			return
		}

		row := importRow{}

		// Extract fields based on header mapping
		if idx, ok := colMap["username"]; ok && idx < len(record) {
			row.Username = strings.TrimSpace(record[idx])
		}
		if idx, ok := colMap["password"]; ok && idx < len(record) {
			row.Password = strings.TrimSpace(record[idx])
		}
		if idx, ok := colMap["email"]; ok && idx < len(record) {
			row.Email = strings.TrimSpace(record[idx])
		}
		if idx, ok := colMap["plan_id"]; ok && idx < len(record) {
			val := strings.TrimSpace(record[idx])
			if val != "" {
				pid, err := strconv.ParseInt(val, 10, 64)
				if err != nil {
					rowErrors = append(rowErrors, ImportRowError{Row: rowNum, Error: "invalid_plan_id"})
					continue
				}
				row.PlanID = &pid
			}
		}
		if idx, ok := colMap["display_name"]; ok && idx < len(record) {
			row.DisplayName = strings.TrimSpace(record[idx])
		}

		// Validate username
		if row.Username == "" {
			rowErrors = append(rowErrors, ImportRowError{Row: rowNum, Error: "username_required"})
			continue
		}
		if !importUsernameRe.MatchString(row.Username) {
			rowErrors = append(rowErrors, ImportRowError{Row: rowNum, Error: "username_invalid"})
			continue
		}

		// Validate password
		if row.Password == "" {
			rowErrors = append(rowErrors, ImportRowError{Row: rowNum, Error: "password_required"})
			continue
		}
		if len(row.Password) < 6 {
			rowErrors = append(rowErrors, ImportRowError{Row: rowNum, Error: "password_too_short"})
			continue
		}

		// Validate email (optional)
		if row.Email != "" && !importEmailRe.MatchString(row.Email) {
			rowErrors = append(rowErrors, ImportRowError{Row: rowNum, Error: "email_invalid"})
			continue
		}

		rows = append(rows, row)
	}

	if len(rows) == 0 && len(rowErrors) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "empty_csv"})
		return
	}

	// Validate plan_ids reference existing plans
	planIDs := make(map[int64]bool)
	for _, row := range rows {
		if row.PlanID != nil {
			planIDs[*row.PlanID] = false
		}
	}
	for pid := range planIDs {
		var exists int
		if err := s.DB.QueryRow(`SELECT 1 FROM plans WHERE id=$1 AND is_active=TRUE LIMIT 1`, pid).Scan(&exists); err == nil {
			planIDs[pid] = true
		}
	}

	// Filter out rows with invalid plan_ids
	validRows := make([]importRow, 0, len(rows))
	for _, row := range rows {
		if row.PlanID != nil && !planIDs[*row.PlanID] {
			rowErrors = append(rowErrors, ImportRowError{Row: 0, Error: fmt.Sprintf("plan_id_%d_not_found", *row.PlanID)})
			continue
		}
		validRows = append(validRows, row)
	}

	// Check for duplicate usernames within the CSV
	seen := make(map[string]bool)
	deduped := make([]importRow, 0, len(validRows))
	for _, row := range validRows {
		lower := strings.ToLower(row.Username)
		if seen[lower] {
			rowErrors = append(rowErrors, ImportRowError{Row: 0, Error: fmt.Sprintf("duplicate_username_%s", row.Username)})
			continue
		}
		seen[lower] = true
		deduped = append(deduped, row)
	}
	validRows = deduped

	// Check for existing usernames in the database
	finalRows := make([]importRow, 0, len(validRows))
	for _, row := range validRows {
		var existingID int64
		err := s.DB.QueryRow(`SELECT id FROM customers WHERE username=$1 LIMIT 1`, row.Username).Scan(&existingID)
		if err == nil {
			rowErrors = append(rowErrors, ImportRowError{Row: 0, Error: fmt.Sprintf("username_exists_%s", row.Username)})
			continue
		}
		finalRows = append(finalRows, row)
	}

	if len(finalRows) == 0 {
		writeJSON(w, map[string]any{"ok": true, "imported": 0, "errors": rowErrors})
		return
	}

	// Hash passwords
	type preparedRow struct {
		importRow
		PasswordHash string
	}
	prepared := make([]preparedRow, 0, len(finalRows))
	for _, row := range finalRows {
		hash, err := auth.HashPassword(row.Password)
		if err != nil {
			rowErrors = append(rowErrors, ImportRowError{Row: 0, Error: fmt.Sprintf("hash_error_%s", row.Username)})
			continue
		}
		prepared = append(prepared, preparedRow{importRow: row, PasswordHash: hash})
	}

	// Create customers in a transaction
	actor, _, _ := s.currentAdmin(r)
	ip := clientIP(r)

	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer tx.Rollback()

	imported := 0
	for _, row := range prepared {
		// Insert customer
		res, err := tx.Exec(
			`INSERT INTO customers(username, display_name, email, plan_id, sub_token, status, created_by) VALUES($1,$2,$3,$4,$5,'active',$6)`,
			row.Username, row.DisplayName, nullString(row.Email), row.PlanID, auth.RandomToken(24), actor,
		)
		if err != nil {
			rowErrors = append(rowErrors, ImportRowError{Row: 0, Error: fmt.Sprintf("insert_error_%s: %v", row.Username, err)})
			continue
		}
		customerID, _ := res.LastInsertId()

		// Insert wallet
		if _, err := tx.Exec(`INSERT INTO wallets(customer_id, username, credit) VALUES($1,$2,0)`, customerID, row.Username); err != nil {
			rowErrors = append(rowErrors, ImportRowError{Row: 0, Error: fmt.Sprintf("wallet_error_%s", row.Username)})
			continue
		}

		// Insert radcheck password
		if _, err := tx.Exec(`INSERT INTO radcheck(username, attribute, op, value) VALUES($1,'Cleartext-Password',':=',$2)`, row.Username, row.Password); err != nil {
			rowErrors = append(rowErrors, ImportRowError{Row: 0, Error: fmt.Sprintf("radcheck_error_%s", row.Username)})
			continue
		}

		// Insert radcheck simultaneous-use (default: 1)
		if _, err := tx.Exec(`INSERT INTO radcheck(username, attribute, op, value) VALUES($1,'Simultaneous-Use',':=','1')`, row.Username); err != nil {
			rowErrors = append(rowErrors, ImportRowError{Row: 0, Error: fmt.Sprintf("radcheck_error_%s", row.Username)})
			continue
		}

		imported++
	}

	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "commit_error"})
		return
	}

	// Audit log
	s.logAudit(actor, "customers.import", "customer", "", nil, map[string]any{
		"imported": imported,
		"errors":   len(rowErrors),
	}, ip)

	log.Printf("[customers] CSV import: imported=%d errors=%d by=%s", imported, len(rowErrors), actor)

	writeJSON(w, map[string]any{"ok": true, "imported": imported, "errors": rowErrors})
}
