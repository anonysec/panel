package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// checkDataWarnings checks the current data usage for the given username against
// configured warning thresholds and the plan data cap. If any threshold is crossed
// and hasn't been warned before, it creates an event and dispatches a notification.
// If usage >= 100% of the cap, it sets the customer status to 'limited'.
//
// This function is called after accounting updates.
func (s *Server) checkDataWarnings(username string) {
	// 1. Query total usage (SUM of acctinputoctets + acctoutputoctets) from radacct
	//    for the customer (active sessions or within current subscription period).
	var totalUsage int64
	err := s.DB.QueryRow(
		`SELECT COALESCE(SUM(acctinputoctets + acctoutputoctets), 0)
		 FROM radacct WHERE username = ?`,
		username,
	).Scan(&totalUsage)
	if err != nil {
		log.Printf("[data-warnings] failed to query usage for %s: %v", username, err)
		return
	}

	if totalUsage == 0 {
		return
	}

	// 2. Query the customer's plan data cap (plans.data_gb * 1024^3) via
	//    customers → subscriptions → plans join.
	//    We look for the active subscription's plan, falling back to the customer's direct plan_id.
	var dataCapBytes int64
	var customerID int64
	err = s.DB.QueryRow(
		`SELECT c.id, COALESCE(p.data_gb, 0) * 1073741824
		 FROM customers c
		 LEFT JOIN subscriptions sub ON sub.username = c.username AND sub.status = 'active'
		 LEFT JOIN plans p ON p.id = COALESCE(sub.plan_id, c.plan_id)
		 WHERE c.username = ?
		 LIMIT 1`,
		username,
	).Scan(&customerID, &dataCapBytes)
	if err != nil {
		log.Printf("[data-warnings] failed to query plan cap for %s: %v", username, err)
		return
	}

	// No data cap configured (unlimited plan) — nothing to warn about
	if dataCapBytes <= 0 {
		return
	}

	// 3. Load thresholds from panel_settings key 'data_warning_thresholds' (JSON array like [80, 95])
	thresholds := []int{80, 95} // default
	var thresholdJSON string
	if err := s.DB.QueryRow(
		`SELECT setting_value FROM panel_settings WHERE setting_key = 'data_warning_thresholds'`,
	).Scan(&thresholdJSON); err == nil && thresholdJSON != "" {
		var parsed []int
		if json.Unmarshal([]byte(thresholdJSON), &parsed) == nil && len(parsed) > 0 {
			thresholds = parsed
		}
	}

	// Calculate usage percentage
	usagePercent := float64(totalUsage) / float64(dataCapBytes) * 100.0

	// 4. For each threshold crossed that hasn't already triggered a warning:
	//    INSERT event (type='data_warning', severity='warning'), dispatch notification
	for _, threshold := range thresholds {
		if usagePercent < float64(threshold) {
			continue
		}

		// Check if we already created a warning for this threshold for this user
		var existingCount int
		err := s.DB.QueryRow(
			`SELECT COUNT(*) FROM events
			 WHERE type = 'data_warning' AND related = ? AND title LIKE ?`,
			username, fmt.Sprintf("%%%d%%", threshold),
		).Scan(&existingCount)
		if err != nil {
			log.Printf("[data-warnings] failed to check existing warnings for %s: %v", username, err)
			continue
		}
		if existingCount > 0 {
			continue
		}

		// Create warning event and dispatch notification
		title := fmt.Sprintf("Data usage at %d%% for %s", threshold, username)
		message := fmt.Sprintf(
			"Customer %s has used %d%% of their data cap (%d bytes of %d bytes allocated).",
			username, threshold, totalUsage, dataCapBytes,
		)
		s.createEvent("data_warning", "warning", title, message, "system", username)
	}

	// 5. If usage >= 100% cap: UPDATE customers SET status='limited', INSERT event
	if totalUsage >= dataCapBytes {
		// Check if we already recorded a data_cap_reached event for this user
		var capReachedCount int
		_ = s.DB.QueryRow(
			`SELECT COUNT(*) FROM events
			 WHERE type = 'data_cap_reached' AND related = ?`,
			username,
		).Scan(&capReachedCount)

		if capReachedCount == 0 {
			// Update customer status to limited
			_, err := s.DB.Exec(
				`UPDATE customers SET status = 'limited' WHERE username = ? AND status != 'limited'`,
				username,
			)
			if err != nil {
				log.Printf("[data-warnings] failed to set customer %s to limited: %v", username, err)
			}

			// Create data cap reached event
			title := fmt.Sprintf("Data cap reached for %s", username)
			message := fmt.Sprintf(
				"Customer %s has exceeded their data cap (used %d bytes, cap %d bytes). Status set to limited.",
				username, totalUsage, dataCapBytes,
			)
			s.createEvent("data_cap_reached", "error", title, message, "system", username)
		}
	}
}



// portalWarnings returns active data warning events for the authenticated customer.
// GET /api/portal/warnings
func (s *Server) portalWarnings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	rows, err := s.DB.Query(
		`SELECT id, type, severity, title, message, created_at
		 FROM events
		 WHERE related = ? AND type IN ('data_warning', 'data_cap_reached')
		 ORDER BY created_at DESC LIMIT 20`,
		username,
	)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type Warning struct {
		ID        int64  `json:"id"`
		Type      string `json:"type"`
		Severity  string `json:"severity"`
		Title     string `json:"title"`
		Message   string `json:"message"`
		CreatedAt string `json:"created_at"`
	}

	warnings := []Warning{}
	for rows.Next() {
		var w Warning
		var createdAt sql.NullTime
		if err := rows.Scan(&w.ID, &w.Type, &w.Severity, &w.Title, &w.Message, &createdAt); err != nil {
			continue
		}
		if createdAt.Valid {
			w.CreatedAt = createdAt.Time.Format(time.RFC3339)
		}
		warnings = append(warnings, w)
	}

	writeJSON(w, map[string]any{"ok": true, "warnings": warnings})
}

// dataWarningThresholds handles PUT /api/settings/data-warning-thresholds
// to allow admins to configure the data usage warning threshold percentages.
func (s *Server) dataWarningThresholds(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getDataWarningThresholds(w, r)
	case http.MethodPut:
		s.putDataWarningThresholds(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getDataWarningThresholds(w http.ResponseWriter, r *http.Request) {
	thresholds := []int{80, 95}
	var thresholdJSON string
	if err := s.DB.QueryRow(
		`SELECT setting_value FROM panel_settings WHERE setting_key = 'data_warning_thresholds'`,
	).Scan(&thresholdJSON); err == nil && thresholdJSON != "" {
		var parsed []int
		if json.Unmarshal([]byte(thresholdJSON), &parsed) == nil && len(parsed) > 0 {
			thresholds = parsed
		}
	}
	writeJSON(w, map[string]any{"ok": true, "thresholds": thresholds})
}

func (s *Server) putDataWarningThresholds(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Thresholds []int `json:"thresholds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	// Validate the array is non-empty
	if len(in.Thresholds) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "thresholds array must not be empty")
		return
	}

	// Validate the array contains no more than 10 thresholds
	if len(in.Thresholds) > 10 {
		writeError(w, http.StatusBadRequest, "bad_request", "thresholds array must contain no more than 10 items")
		return
	}

	// Validate each threshold is between 0 and 100 inclusive
	for _, t := range in.Thresholds {
		if t < 0 || t > 100 {
			writeError(w, http.StatusBadRequest, "bad_request", "each threshold must be between 0 and 100")
			return
		}
	}

	// Serialize thresholds to JSON
	thresholdJSON, err := json.Marshal(in.Thresholds)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to serialize thresholds")
		return
	}

	// Update panel_settings
	_, err = s.DB.Exec(
		`INSERT INTO panel_settings(setting_key, setting_value) VALUES('data_warning_thresholds', ?)
		 ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value)`,
		string(thresholdJSON),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update thresholds")
		return
	}

	writeJSON(w, map[string]any{"ok": true, "thresholds": in.Thresholds})
}

// warningConfig handles GET/PUT /api/settings/warning-config for expiry, connection, and webhook settings.
func (s *Server) warningConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getWarningConfig(w, r)
	case http.MethodPut:
		s.putWarningConfig(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getWarningConfig(w http.ResponseWriter, r *http.Request) {
	config := map[string]any{
		"expiry_days":     []int{7, 3, 1},
		"conn_thresholds": []int{80, 95},
		"webhook_url":     "",
	}
	var cfgJSON string
	if err := s.DB.QueryRow(
		`SELECT setting_value FROM panel_settings WHERE setting_key = 'warning_config'`,
	).Scan(&cfgJSON); err == nil && cfgJSON != "" {
		var parsed map[string]any
		if json.Unmarshal([]byte(cfgJSON), &parsed) == nil {
			config = parsed
		}
	}
	writeJSON(w, map[string]any{"ok": true, "config": config})
}

func (s *Server) putWarningConfig(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ExpiryDays     []int  `json:"expiry_days"`
		ConnThresholds []int  `json:"conn_thresholds"`
		WebhookURL     string `json:"webhook_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	// Validate webhook URL if provided
	in.WebhookURL = strings.TrimSpace(in.WebhookURL)
	if in.WebhookURL != "" {
		parsed, err := url.Parse(in.WebhookURL)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid webhook URL")
			return
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			writeError(w, http.StatusBadRequest, "bad_request", "webhook URL must use http or https scheme")
			return
		}
		if parsed.Host == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "webhook URL must have a valid host")
			return
		}
	}

	cfgJSON, err := json.Marshal(map[string]any{
		"expiry_days":     in.ExpiryDays,
		"conn_thresholds": in.ConnThresholds,
		"webhook_url":     in.WebhookURL,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to serialize config")
		return
	}

	_, err = s.DB.Exec(
		`INSERT INTO panel_settings(setting_key, setting_value) VALUES('warning_config', ?)
		 ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value)`,
		string(cfgJSON),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to update warning config")
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// checkExpiryWarnings checks if a user's subscription is about to expire.
func (s *Server) checkExpiryWarnings(username string) {
	// Load expiry warning config
	expiryDays := []int{7, 3, 1}
	var cfgJSON string
	if err := s.DB.QueryRow(
		`SELECT setting_value FROM panel_settings WHERE setting_key = 'warning_config'`,
	).Scan(&cfgJSON); err == nil && cfgJSON != "" {
		var parsed struct {
			ExpiryDays []int `json:"expiry_days"`
		}
		if json.Unmarshal([]byte(cfgJSON), &parsed) == nil && len(parsed.ExpiryDays) > 0 {
			expiryDays = parsed.ExpiryDays
		}
	}

	// Get subscription expiry
	var expiresAt sql.NullTime
	err := s.DB.QueryRow(
		`SELECT expires_at FROM subscriptions WHERE username = ? AND status = 'active' ORDER BY id DESC LIMIT 1`,
		username,
	).Scan(&expiresAt)
	if err != nil || !expiresAt.Valid {
		return
	}

	daysLeft := int(time.Until(expiresAt.Time).Hours() / 24)

	for _, threshold := range expiryDays {
		if daysLeft > threshold {
			continue
		}

		// Check if we already warned
		var count int
		_ = s.DB.QueryRow(
			`SELECT COUNT(*) FROM events WHERE type = 'expiry_warning' AND related = ? AND title LIKE ?`,
			username, fmt.Sprintf("%%%d day%%", threshold),
		).Scan(&count)
		if count > 0 {
			continue
		}

		title := fmt.Sprintf("Subscription expiry in %d day(s) for %s", threshold, username)
		message := fmt.Sprintf("Customer %s subscription expires in %d day(s) on %s.",
			username, daysLeft, expiresAt.Time.Format("2006-01-02"))
		s.createEvent("expiry_warning", "warning", title, message, "system", username)
		s.dispatchWebhook("expiry_warning", title, message, username)
	}
}

// checkConnectionWarnings checks if a user's concurrent sessions are near the limit.
func (s *Server) checkConnectionWarnings(username string) {
	// Load connection warning config
	connThresholds := []int{80, 95}
	var cfgJSON string
	if err := s.DB.QueryRow(
		`SELECT setting_value FROM panel_settings WHERE setting_key = 'warning_config'`,
	).Scan(&cfgJSON); err == nil && cfgJSON != "" {
		var parsed struct {
			ConnThresholds []int `json:"conn_thresholds"`
		}
		if json.Unmarshal([]byte(cfgJSON), &parsed) == nil && len(parsed.ConnThresholds) > 0 {
			connThresholds = parsed.ConnThresholds
		}
	}

	// Get max sessions from radcheck (Simultaneous-Use attribute)
	var maxSessions int
	err := s.DB.QueryRow(
		`SELECT CAST(value AS UNSIGNED) FROM radcheck WHERE username = ? AND attribute = 'Simultaneous-Use' ORDER BY id DESC LIMIT 1`,
		username,
	).Scan(&maxSessions)
	if err != nil || maxSessions <= 0 {
		return
	}

	// Get current session count
	var currentSessions int
	_ = s.DB.QueryRow(
		`SELECT COUNT(*) FROM radacct WHERE username = ? AND acctstoptime IS NULL`,
		username,
	).Scan(&currentSessions)

	if currentSessions == 0 {
		return
	}

	usagePercent := float64(currentSessions) / float64(maxSessions) * 100.0

	for _, threshold := range connThresholds {
		if usagePercent < float64(threshold) {
			continue
		}

		var count int
		_ = s.DB.QueryRow(
			`SELECT COUNT(*) FROM events WHERE type = 'conn_warning' AND related = ? AND title LIKE ? AND created_at > NOW() - INTERVAL 1 HOUR`,
			username, fmt.Sprintf("%%%d%%", threshold),
		).Scan(&count)
		if count > 0 {
			continue
		}

		title := fmt.Sprintf("Connection usage at %d%% for %s", threshold, username)
		message := fmt.Sprintf("Customer %s is using %d of %d allowed connections (%d%%).",
			username, currentSessions, maxSessions, int(usagePercent))
		s.createEvent("conn_warning", "warning", title, message, "system", username)
		s.dispatchWebhook("conn_warning", title, message, username)
	}
}

// dispatchWebhook sends a warning event to the configured webhook URL.
func (s *Server) dispatchWebhook(eventType, title, message, username string) {
	var cfgJSON string
	if err := s.DB.QueryRow(
		`SELECT setting_value FROM panel_settings WHERE setting_key = 'warning_config'`,
	).Scan(&cfgJSON); err != nil || cfgJSON == "" {
		return
	}
	var parsed struct {
		WebhookURL string `json:"webhook_url"`
	}
	if json.Unmarshal([]byte(cfgJSON), &parsed) != nil || parsed.WebhookURL == "" {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"type":     eventType,
		"title":    title,
		"message":  message,
		"username": username,
		"time":     time.Now().UTC().Format(time.RFC3339),
	})

	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(parsed.WebhookURL, "application/json", bytes.NewReader(payload))
		if err != nil {
			log.Printf("[webhook] failed to dispatch to %s: %v", parsed.WebhookURL, err)
			return
		}
		resp.Body.Close()
	}()
}

// portalAppLinks returns the configured app download links (public endpoint).
// GET /api/portal/app-links
func (s *Server) portalAppLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	var linksJSON string
	err := s.DB.QueryRow(
		`SELECT setting_value FROM panel_settings WHERE setting_key = 'app_links'`,
	).Scan(&linksJSON)
	if err != nil || linksJSON == "" {
		writeJSON(w, map[string]any{"ok": true, "links": []any{}})
		return
	}
	var links []map[string]string
	if json.Unmarshal([]byte(linksJSON), &links) != nil {
		writeJSON(w, map[string]any{"ok": true, "links": []any{}})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "links": links})
}
