package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// processStartTime records when the process started for uptime calculation.
var processStartTime = time.Now()

// aiDiagnostics runs all health checks and returns the diagnostics report.
// GET /api/diagnostics/ai
func (s *Server) aiDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	if s.HealthEngine == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "health engine not initialized"})
		return
	}

	report, err := s.HealthEngine.RunAll(r.Context())
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "report": report})
}

// aiDiagnosticsHistory returns historical health scores within a time range.
// GET /api/diagnostics/ai/history?from=2024-01-01T00:00:00Z&to=2024-01-31T23:59:59Z
func (s *Server) aiDiagnosticsHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse time range from query parameters
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	var from, to time.Time
	var err error

	if fromStr != "" {
		from, err = time.Parse(time.RFC3339, fromStr)
		if err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid 'from' parameter, use RFC3339 format"})
			return
		}
	} else {
		// Default: last 24 hours
		from = time.Now().Add(-24 * time.Hour)
	}

	if toStr != "" {
		to, err = time.Parse(time.RFC3339, toStr)
		if err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid 'to' parameter, use RFC3339 format"})
			return
		}
	} else {
		to = time.Now()
	}

	rows, err := s.DB.QueryContext(r.Context(),
		`SELECT id, score, trend, checks_json, generated_at FROM health_scores WHERE generated_at >= $1 AND generated_at <= $2 ORDER BY generated_at DESC LIMIT 1000`,
		from, to,
	)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type HistoryEntry struct {
		ID          int64           `json:"id"`
		Score       int             `json:"score"`
		Trend       string          `json:"trend"`
		ChecksJSON  json.RawMessage `json:"checks_json"`
		GeneratedAt string          `json:"generated_at"`
	}

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var genAt sql.NullTime
		var checksRaw []byte
		if err := rows.Scan(&e.ID, &e.Score, &e.Trend, &checksRaw, &genAt); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		e.ChecksJSON = checksRaw
		if genAt.Valid {
			e.GeneratedAt = genAt.Time.Format(time.RFC3339)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "history": entries})
}

// aiHealingRules returns all configured healing rules.
// GET /api/diagnostics/ai/rules
func (s *Server) aiHealingRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	rows, err := s.DB.QueryContext(r.Context(),
		`SELECT id, rule_key, display_name, condition_type, action_mode, cooldown_seconds, enabled, thresholds_json, created_at, updated_at FROM healing_rules ORDER BY id ASC`,
	)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type RuleResponse struct {
		ID              int64           `json:"id"`
		RuleKey         string          `json:"rule_key"`
		DisplayName     string          `json:"display_name"`
		ConditionType   string          `json:"condition_type"`
		ActionMode      string          `json:"action_mode"`
		CooldownSeconds int             `json:"cooldown_seconds"`
		Enabled         bool            `json:"enabled"`
		ThresholdsJSON  json.RawMessage `json:"thresholds_json"`
		CreatedAt       string          `json:"created_at"`
		UpdatedAt       string          `json:"updated_at"`
	}

	var rules []RuleResponse
	for rows.Next() {
		var rule RuleResponse
		var enabled bool
		var thresholds []byte
		var createdAt, updatedAt sql.NullTime
		if err := rows.Scan(&rule.ID, &rule.RuleKey, &rule.DisplayName, &rule.ConditionType, &rule.ActionMode, &rule.CooldownSeconds, &enabled, &thresholds, &createdAt, &updatedAt); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		rule.Enabled = enabled
		if thresholds != nil {
			rule.ThresholdsJSON = thresholds
		}
		if createdAt.Valid {
			rule.CreatedAt = createdAt.Time.Format(time.RFC3339)
		}
		if updatedAt.Valid {
			rule.UpdatedAt = updatedAt.Time.Format(time.RFC3339)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "rules": rules})
}

// aiHealingRuleByID updates a specific healing rule.
// PUT /api/diagnostics/ai/rules/{id}
func (s *Server) aiHealingRuleByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse rule ID from URL path
	id, _, ok := pathID(r.URL.Path, "/api/diagnostics/ai/rules/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	var in struct {
		ActionMode      *string         `json:"action_mode"`
		CooldownSeconds *int            `json:"cooldown_seconds"`
		Enabled         *bool           `json:"enabled"`
		ThresholdsJSON  json.RawMessage `json:"thresholds_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Build dynamic update
	sets := []string{}
	args := []any{}

	if in.ActionMode != nil {
		mode := strings.TrimSpace(*in.ActionMode)
		if mode != "auto_fix" && mode != "alert_only" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "action_mode must be 'auto_fix' or 'alert_only'"})
			return
		}
		sets = append(sets, "action_mode=$1")
		args = append(args, mode)
	}
	if in.CooldownSeconds != nil {
		if *in.CooldownSeconds < 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "cooldown_seconds must be non-negative"})
			return
		}
		sets = append(sets, "cooldown_seconds=$1")
		args = append(args, *in.CooldownSeconds)
	}
	if in.Enabled != nil {
		enabled := 0
		if *in.Enabled {
			enabled = 1
		}
		sets = append(sets, "enabled=$1")
		args = append(args, enabled)
	}
	if in.ThresholdsJSON != nil {
		sets = append(sets, "thresholds_json=$1")
		args = append(args, string(in.ThresholdsJSON))
	}

	if len(sets) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "no fields to update"})
		return
	}

	args = append(args, id)
	result, err := s.DB.ExecContext(r.Context(),
		`UPDATE healing_rules SET `+strings.Join(sets, ",")+` WHERE id=$1`, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "rule not found"})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// aiHealingLog returns paginated healing action logs with optional filters.
// GET /api/diagnostics/ai/healing-log?page=1&page_size=20&from=...&to=...&rule_key=...&status=...
func (s *Server) aiHealingLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse pagination
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Parse filters
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	ruleKey := strings.TrimSpace(r.URL.Query().Get("rule_key"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	// Build WHERE clause
	where := []string{"1=1"}
	args := []any{}
	argN := 1

	if fromStr != "" {
		from, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid 'from' parameter"})
			return
		}
		where = append(where, fmt.Sprintf("created_at >= $%d", argN))
		args = append(args, from)
		argN++
	}
	if toStr != "" {
		to, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid 'to' parameter"})
			return
		}
		where = append(where, fmt.Sprintf("created_at <= $%d", argN))
		args = append(args, to)
		argN++
	}
	if ruleKey != "" {
		where = append(where, fmt.Sprintf("rule_key = $%d", argN))
		args = append(args, ruleKey)
		argN++
	}
	if status != "" {
		where = append(where, fmt.Sprintf("result_status = $%d", argN))
		args = append(args, status)
		argN++
	}

	whereClause := strings.Join(where, " AND ")

	// Get total count
	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	err := s.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM healing_actions WHERE `+whereClause, countArgs...).Scan(&total)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Query page
	offset := (page - 1) * pageSize
	limitN := len(args) + 1
	offsetN := len(args) + 2
	args = append(args, pageSize, offset)

	rows, err := s.DB.QueryContext(r.Context(),
		`SELECT id, rule_key, resource_type, resource_id, action_performed, result_status, COALESCE(error_message,''), execution_ms, created_at FROM healing_actions WHERE `+whereClause+fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, limitN, offsetN), args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type LogEntry struct {
		ID              int64  `json:"id"`
		RuleKey         string `json:"rule_key"`
		ResourceType    string `json:"resource_type"`
		ResourceID      string `json:"resource_id"`
		ActionPerformed string `json:"action_performed"`
		ResultStatus    string `json:"result_status"`
		ErrorMessage    string `json:"error_message,omitempty"`
		ExecutionMs     int    `json:"execution_ms"`
		CreatedAt       string `json:"created_at"`
	}

	var entries []LogEntry
	for rows.Next() {
		var e LogEntry
		var createdAt sql.NullTime
		if err := rows.Scan(&e.ID, &e.RuleKey, &e.ResourceType, &e.ResourceID, &e.ActionPerformed, &e.ResultStatus, &e.ErrorMessage, &e.ExecutionMs, &createdAt); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if createdAt.Valid {
			e.CreatedAt = createdAt.Time.Format(time.RFC3339)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{
		"ok":        true,
		"actions":   entries,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// allowedUnits is the whitelist of systemd units that can be queried via serverLogs.
var allowedUnits = map[string]bool{
	"panel":      true,
	"nginx":      true,
	"openvpn":    true,
	"xl2tpd":     true,
	"strongswan": true,
	"mariadb":    true,
	"mysql":      true,
	"knode":      true,
}

// serverLogs returns recent journalctl output for a whitelisted systemd unit.
// GET /api/diagnostics/logs?lines=50&unit=panel
func (s *Server) serverLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse lines parameter (default 50, max 500)
	lines := 50
	if l := r.URL.Query().Get("lines"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed < 1 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid 'lines' parameter"})
			return
		}
		lines = parsed
		if lines > 500 {
			lines = 500
		}
	}

	// Parse unit parameter (default "panel", must be whitelisted)
	unit := r.URL.Query().Get("unit")
	if unit == "" {
		unit = "panel"
	}
	if !allowedUnits[unit] {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": fmt.Sprintf("unit '%s' not allowed; allowed: koris, nginx, openvpn, xl2tpd, strongswan, mariadb, mysql, knode", unit)})
		return
	}

	// Execute journalctl
	cmd := exec.Command("journalctl", "-u", unit, "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso")
	output, err := cmd.CombinedOutput()
	if err != nil {
		writeJSON(w, map[string]any{"ok": true, "unit": unit, "lines": lines, "output": string(output), "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "unit": unit, "lines": lines, "output": string(output)})
}

// serverStatus returns quick system status including version, uptime, runtime info,
// database connectivity, and last health score.
// GET /api/diagnostics/status
func (s *Server) serverStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Uptime
	uptime := time.Since(processStartTime).String()

	// Go runtime info
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Database ping
	dbStatus := "ok"
	if err := s.DB.Ping(); err != nil {
		dbStatus = err.Error()
	}

	// Last health score
	var lastScore *int
	var lastScoreAt *string
	var score int
	var generatedAt sql.NullTime
	err := s.DB.QueryRow(`SELECT score, generated_at FROM health_scores ORDER BY id DESC LIMIT 1`).Scan(&score, &generatedAt)
	if err == nil {
		lastScore = &score
		if generatedAt.Valid {
			t := generatedAt.Time.Format(time.RFC3339)
			lastScoreAt = &t
		}
	}

	writeJSON(w, map[string]any{
		"ok":      true,
		"version": s.Config.Version,
		"uptime":  uptime,
		"runtime": map[string]any{
			"go_version": runtime.Version(),
			"goroutines": runtime.NumGoroutine(),
			"mem_alloc":  memStats.Alloc,
			"mem_sys":    memStats.Sys,
		},
		"database": map[string]any{
			"status": dbStatus,
		},
		"last_health_score":    lastScore,
		"last_health_score_at": lastScoreAt,
	})
}
