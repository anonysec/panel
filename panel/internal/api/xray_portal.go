//go:build !lite

package api

import (
	"database/sql"
	"log"
	"net/http"

	"KorisPanel/panel/internal/xray"
)

// handleXraySubscription handles GET /api/portal/xray/subscription
// Returns the customer's Xray inbound links as a base64 subscription.
// If ?format=json query param is provided, returns JSON with subscription and links array.
// Otherwise returns plain text base64-encoded subscription.
func (s *Server) handleXraySubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	// Get customer ID from username.
	var customerID int64
	err := s.DB.QueryRow(
		`SELECT id FROM customers WHERE username = ? AND deleted_at IS NULL LIMIT 1`,
		username,
	).Scan(&customerID)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}
	if err != nil {
		log.Printf("[xray-portal] lookup customer %s: %v", username, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// Query active xray_inbounds for this customer joined with nodes for host IP.
	rows, err := s.DB.Query(`
		SELECT xi.id, xi.uuid, xi.protocol, xi.transport, xi.security, xi.port,
		       COALESCE(xi.server_name, ''), COALESCE(xi.public_key, ''),
		       COALESCE(xi.short_id, ''), COALESCE(xi.path, ''),
		       COALESCE(xi.service_name, ''),
		       n.public_ip, COALESCE(n.name, '')
		FROM xray_inbounds xi
		JOIN nodes n ON n.id = xi.node_id
		WHERE xi.customer_id = ? AND xi.status = 'active'
		ORDER BY xi.id ASC`, customerID)
	if err != nil {
		log.Printf("[xray-portal] query inbounds for customer %d: %v", customerID, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	var links []string
	for rows.Next() {
		var (
			id          int64
			uuid        string
			protocol    string
			transport   string
			security    string
			port        int
			serverName  string
			publicKey   string
			shortID     string
			path        string
			serviceName string
			nodeIP      string
			nodeName    string
		)
		if err := rows.Scan(&id, &uuid, &protocol, &transport, &security, &port,
			&serverName, &publicKey, &shortID, &path, &serviceName,
			&nodeIP, &nodeName); err != nil {
			log.Printf("[xray-portal] scan inbound row: %v", err)
			continue
		}

		cfg := xray.InboundConfig{
			UUID:        uuid,
			Protocol:    protocol,
			Transport:   transport,
			Security:    security,
			Port:        port,
			ServerName:  serverName,
			PublicKey:   publicKey,
			ShortID:     shortID,
			Path:        path,
			ServiceName: serviceName,
		}

		remark := nodeName + "-" + protocol
		link := xray.GenerateShareLink(cfg, nodeIP, remark)
		if link != "" {
			links = append(links, link)
		}
	}

	// Generate base64 subscription from all links.
	subscription := xray.GenerateSubscription(links)

	// Check if JSON format requested.
	format := r.URL.Query().Get("format")
	if format == "json" {
		writeJSON(w, map[string]any{
			"ok":           true,
			"subscription": subscription,
			"links":        links,
		})
		return
	}

	// Default: return plain text subscription (compatible with v2rayN/Clash clients).
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Profile-Update-Interval", "12")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(subscription))
}

// handleXrayLinks handles GET /api/portal/xray/links
// Returns individual share links for all active inbounds of the customer.
func (s *Server) handleXrayLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	// Get customer ID from username.
	var customerID int64
	err := s.DB.QueryRow(
		`SELECT id FROM customers WHERE username = ? AND deleted_at IS NULL LIMIT 1`,
		username,
	).Scan(&customerID)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	}
	if err != nil {
		log.Printf("[xray-portal] lookup customer %s: %v", username, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// Query active xray_inbounds for this customer joined with nodes.
	rows, err := s.DB.Query(`
		SELECT xi.id, xi.uuid, xi.protocol, xi.transport, xi.security, xi.port,
		       COALESCE(xi.server_name, ''), COALESCE(xi.public_key, ''),
		       COALESCE(xi.short_id, ''), COALESCE(xi.path, ''),
		       COALESCE(xi.service_name, ''),
		       n.public_ip, COALESCE(n.name, '')
		FROM xray_inbounds xi
		JOIN nodes n ON n.id = xi.node_id
		WHERE xi.customer_id = ? AND xi.status = 'active'
		ORDER BY xi.id ASC`, customerID)
	if err != nil {
		log.Printf("[xray-portal] query inbounds for customer %d: %v", customerID, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type linkEntry struct {
		ID       int64  `json:"id"`
		Protocol string `json:"protocol"`
		Link     string `json:"link"`
		NodeName string `json:"node_name"`
	}

	var linkEntries []linkEntry
	for rows.Next() {
		var (
			id          int64
			uuid        string
			protocol    string
			transport   string
			security    string
			port        int
			serverName  string
			publicKey   string
			shortID     string
			path        string
			serviceName string
			nodeIP      string
			nodeName    string
		)
		if err := rows.Scan(&id, &uuid, &protocol, &transport, &security, &port,
			&serverName, &publicKey, &shortID, &path, &serviceName,
			&nodeIP, &nodeName); err != nil {
			log.Printf("[xray-portal] scan inbound row: %v", err)
			continue
		}

		cfg := xray.InboundConfig{
			UUID:        uuid,
			Protocol:    protocol,
			Transport:   transport,
			Security:    security,
			Port:        port,
			ServerName:  serverName,
			PublicKey:   publicKey,
			ShortID:     shortID,
			Path:        path,
			ServiceName: serviceName,
		}

		remark := nodeName + "-" + protocol
		link := xray.GenerateShareLink(cfg, nodeIP, remark)

		linkEntries = append(linkEntries, linkEntry{
			ID:       id,
			Protocol: protocol,
			Link:     link,
			NodeName: nodeName,
		})
	}

	// Return empty array if no inbounds (not null).
	if linkEntries == nil {
		linkEntries = []linkEntry{}
	}

	writeJSON(w, map[string]any{
		"ok":    true,
		"links": linkEntries,
	})
}
