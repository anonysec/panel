//go:build !lite

package api

import (
	"context"
	"database/sql"
	"encoding/base64"
	"log"
	"net/http"
	"strings"
	"time"

	"KorisPanel/panel/internal/xray"
)

// xraySubscription handles GET /api/sub/{token}
// Returns all Xray config links for a customer, base64-encoded, compatible
// with v2rayN/Clash/Shadowrocket subscription format.
func (s *Server) xraySubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract token from path: /api/sub/{token}
	token := strings.TrimPrefix(r.URL.Path, "/api/sub/")
	token = strings.TrimSpace(token)
	if token == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Look up customer by xray_uuid (used as the subscription token).
	var customerID int64
	var username string
	var status string
	err := s.DB.QueryRow(
		`SELECT id, username, status FROM customers WHERE xray_uuid = $1 AND deleted_at IS NULL LIMIT 1`,
		token,
	).Scan(&customerID, &username, &status)
	if err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("[xray] subscription lookup error: %v", err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Check customer status — must be active.
	if status != "active" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Check subscription is not expired.
	var subStatus string
	var expiresAt sql.NullTime
	err = s.DB.QueryRow(
		`SELECT status, expires_at FROM subscriptions WHERE username = $1 ORDER BY id DESC LIMIT 1`,
		username,
	).Scan(&subStatus, &expiresAt)
	if err == sql.ErrNoRows {
		// No subscription found — deny access.
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("[xray] subscription query error for %s: %v", username, err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Subscription must be active and not expired.
	if subStatus != "active" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Fetch all nodes with enabled xray configs.
	ctx := context.Background()
	xraySvc := xray.New(s.DB)

	rows, err := s.DB.Query(`SELECT node_id FROM xray_configs WHERE enabled = TRUE`)
	if err != nil {
		log.Printf("[xray] query xray nodes error: %v", err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer rows.Close()

	var allLinks []string
	for rows.Next() {
		var nodeID int64
		if err := rows.Scan(&nodeID); err != nil {
			continue
		}

		configs, err := xraySvc.GenerateCustomerConfigs(ctx, token, nodeID)
		if err != nil {
			log.Printf("[xray] generate configs for node %d, user %s: %v", nodeID, username, err)
			continue
		}

		for _, cfg := range configs {
			if cfg.Link != "" {
				allLinks = append(allLinks, cfg.Link)
			}
		}
	}

	// Join all links with newline, then base64 encode.
	joined := strings.Join(allLinks, "\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(joined))

	// Set response headers for v2ray client compatibility.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="sub"`)
	w.Header().Set("Profile-Update-Interval", "12")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(encoded))
}
