package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// failoverDomains handles GET (list) and POST (create) for /api/failover/domains.
func (s *Server) failoverDomains(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listFailoverDomains(w, r)
	case http.MethodPost:
		s.createFailoverDomain(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// failoverDomainByID handles PATCH (update), DELETE, and sub-path routing for
// /api/failover/domains/{id}, /api/failover/domains/{id}/failover, /api/failover/domains/{id}/status.
func (s *Server) failoverDomainByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/failover/domains/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	switch action {
	case "":
		switch r.Method {
		case http.MethodGet:
			s.getFailoverDomain(w, id)
		case http.MethodPatch:
			s.updateFailoverDomain(w, r, id)
		case http.MethodDelete:
			s.deleteFailoverDomain(w, r, id)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	case "failover":
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.triggerFailover(w, r, id)
	case "status":
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.failoverDomainStatus(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

// listFailoverDomains returns all failover domains with joined node and provider info.
func (s *Server) listFailoverDomains(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(`
		SELECT fd.id, fd.domain, fd.current_node_id, fd.dns_provider_id, fd.dns_record_id,
		       fd.ttl, fd.is_active, fd.last_failover_at, fd.created_at, fd.updated_at,
		       COALESCE(n.name, ''), COALESCE(n.public_ip, ''),
		       COALESCE(dp.name, '')
		FROM failover_domains fd
		LEFT JOIN nodes n ON n.id = fd.current_node_id
		LEFT JOIN dns_providers dp ON dp.id = fd.dns_provider_id
		ORDER BY fd.id DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	domains := []FailoverDomain{}
	for rows.Next() {
		var d FailoverDomain
		var providerID sql.NullInt64
		var lastFailover sql.NullString
		var dnsRecordID sql.NullString
		if err := rows.Scan(
			&d.ID, &d.Domain, &d.CurrentNodeID, &providerID, &dnsRecordID,
			&d.TTL, &d.IsActive, &lastFailover, &d.CreatedAt, &d.UpdatedAt,
			&d.CurrentNodeName, &d.CurrentNodeIP,
			&d.ProviderName,
		); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if providerID.Valid {
			d.DNSProviderID = &providerID.Int64
		}
		if lastFailover.Valid {
			d.LastFailoverAt = &lastFailover.String
		}
		if dnsRecordID.Valid {
			d.DNSRecordID = dnsRecordID.String
		}
		domains = append(domains, d)
	}
	if err := rows.Err(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "domains": domains})
}

// createFailoverDomain validates and creates a new failover domain.
func (s *Server) createFailoverDomain(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Domain        string `json:"domain"`
		CurrentNodeID int64  `json:"current_node_id"`
		DNSProviderID *int64 `json:"dns_provider_id"`
		DNSRecordID   string `json:"dns_record_id"`
		TTL           int    `json:"ttl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	in.Domain = strings.TrimSpace(strings.ToLower(in.Domain))
	in.Domain = strings.TrimSuffix(in.Domain, ".")

	// Validate FQDN
	if !isValidFQDN(in.Domain) {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_domain", "message": "Domain must be a valid FQDN"})
		return
	}

	// Validate domain uniqueness among active domains
	var existingCount int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM failover_domains WHERE domain = $1 AND is_active = TRUE`, in.Domain).Scan(&existingCount)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if existingCount > 0 {
		writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "domain_exists", "message": "An active failover domain with this name already exists"})
		return
	}

	// Validate node existence
	var nodeExists int
	err = s.DB.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = $1`, in.CurrentNodeID).Scan(&nodeExists)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if nodeExists == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_not_found", "message": "Referenced node does not exist"})
		return
	}

	// Normalize TTL
	ttl := normalizeFailoverTTL(in.TTL)

	// Validate optional dns_provider_id reference
	if in.DNSProviderID != nil && *in.DNSProviderID > 0 {
		var providerExists int
		err = s.DB.QueryRow(`SELECT COUNT(*) FROM dns_providers WHERE id = $1 AND is_active = TRUE`, *in.DNSProviderID).Scan(&providerExists)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if providerExists == 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "provider_not_found", "message": "Referenced DNS provider does not exist or is inactive"})
			return
		}
	} else {
		in.DNSProviderID = nil
	}

	res, err := s.DB.Exec(`INSERT INTO failover_domains(domain, current_node_id, dns_provider_id, dns_record_id, ttl, is_active) VALUES($1,$2,$3,$4,$5,1)`,
		in.Domain, in.CurrentNodeID, in.DNSProviderID, in.DNSRecordID, ttl)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "failover_domain.created", "failover_domain", strconv.FormatInt(id, 10), nil, map[string]any{"domain": in.Domain, "node_id": in.CurrentNodeID}, clientIP(r))

	// Return the created domain
	d := FailoverDomain{
		ID:            id,
		Domain:        in.Domain,
		CurrentNodeID: in.CurrentNodeID,
		DNSProviderID: in.DNSProviderID,
		DNSRecordID:   in.DNSRecordID,
		TTL:           ttl,
		IsActive:      true,
	}
	writeJSON(w, map[string]any{"ok": true, "domain": d})
}

// getFailoverDomain returns a single failover domain by ID with joined fields.
func (s *Server) getFailoverDomain(w http.ResponseWriter, id int64) {
	var d FailoverDomain
	var providerID sql.NullInt64
	var lastFailover sql.NullString
	var dnsRecordID sql.NullString
	err := s.DB.QueryRow(`
		SELECT fd.id, fd.domain, fd.current_node_id, fd.dns_provider_id, fd.dns_record_id,
		       fd.ttl, fd.is_active, fd.last_failover_at, fd.created_at, fd.updated_at,
		       COALESCE(n.name, ''), COALESCE(n.public_ip, ''),
		       COALESCE(dp.name, '')
		FROM failover_domains fd
		LEFT JOIN nodes n ON n.id = fd.current_node_id
		LEFT JOIN dns_providers dp ON dp.id = fd.dns_provider_id
		WHERE fd.id = $1`, id).Scan(
		&d.ID, &d.Domain, &d.CurrentNodeID, &providerID, &dnsRecordID,
		&d.TTL, &d.IsActive, &lastFailover, &d.CreatedAt, &d.UpdatedAt,
		&d.CurrentNodeName, &d.CurrentNodeIP,
		&d.ProviderName,
	)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if providerID.Valid {
		d.DNSProviderID = &providerID.Int64
	}
	if lastFailover.Valid {
		d.LastFailoverAt = &lastFailover.String
	}
	if dnsRecordID.Valid {
		d.DNSRecordID = dnsRecordID.String
	}
	writeJSON(w, map[string]any{"ok": true, "domain": d})
}

// updateFailoverDomain handles partial updates to a failover domain.
func (s *Server) updateFailoverDomain(w http.ResponseWriter, r *http.Request, id int64) {
	// Check domain exists
	var current FailoverDomain
	var providerID sql.NullInt64
	err := s.DB.QueryRow(`SELECT id, domain, current_node_id, dns_provider_id, dns_record_id, ttl, is_active FROM failover_domains WHERE id = $1`, id).Scan(
		&current.ID, &current.Domain, &current.CurrentNodeID, &providerID, &current.DNSRecordID, &current.TTL, &current.IsActive,
	)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if providerID.Valid {
		current.DNSProviderID = &providerID.Int64
	}

	// Parse the update payload — allow partial updates
	var in struct {
		Domain        *string `json:"domain"`
		CurrentNodeID *int64  `json:"current_node_id"`
		DNSProviderID *int64  `json:"dns_provider_id"`
		DNSRecordID   *string `json:"dns_record_id"`
		TTL           *int    `json:"ttl"`
		IsActive      *bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Build update fields
	sets := []string{}
	args := []any{}

	if in.Domain != nil {
		domain := strings.TrimSpace(strings.ToLower(*in.Domain))
		domain = strings.TrimSuffix(domain, ".")
		if !isValidFQDN(domain) {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_domain", "message": "Domain must be a valid FQDN"})
			return
		}
		// Check uniqueness among active domains (excluding self)
		var existingCount int
		err := s.DB.QueryRow(`SELECT COUNT(*) FROM failover_domains WHERE domain = $1 AND is_active = TRUE AND id != $2`, domain, id).Scan(&existingCount)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if existingCount > 0 {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "domain_exists", "message": "An active failover domain with this name already exists"})
			return
		}
		sets = append(sets, "domain = $3")
		args = append(args, domain)
	}

	if in.CurrentNodeID != nil {
		var nodeExists int
		err := s.DB.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = $1`, *in.CurrentNodeID).Scan(&nodeExists)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if nodeExists == 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_not_found", "message": "Referenced node does not exist"})
			return
		}
		sets = append(sets, "current_node_id = $3")
		args = append(args, *in.CurrentNodeID)
	}

	if in.DNSProviderID != nil {
		if *in.DNSProviderID == 0 {
			// Allow clearing the provider
			sets = append(sets, "dns_provider_id = NULL")
		} else {
			var providerExists int
			err := s.DB.QueryRow(`SELECT COUNT(*) FROM dns_providers WHERE id = $1 AND is_active = TRUE`, *in.DNSProviderID).Scan(&providerExists)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			if providerExists == 0 {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "provider_not_found", "message": "Referenced DNS provider does not exist or is inactive"})
				return
			}
			sets = append(sets, "dns_provider_id = $3")
			args = append(args, *in.DNSProviderID)
		}
	}

	if in.DNSRecordID != nil {
		sets = append(sets, "dns_record_id = $3")
		args = append(args, *in.DNSRecordID)
	}

	if in.TTL != nil {
		ttl := normalizeFailoverTTL(*in.TTL)
		sets = append(sets, "ttl = $3")
		args = append(args, ttl)
	}

	if in.IsActive != nil {
		sets = append(sets, "is_active = $3")
		args = append(args, boolInt(*in.IsActive))
	}

	if len(sets) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "no_changes"})
		return
	}

	args = append(args, id)
	query := "UPDATE failover_domains SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	if _, err := s.DB.Exec(query, args...); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "failover_domain.updated", "failover_domain", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

// deleteFailoverDomain removes a failover domain record.
func (s *Server) deleteFailoverDomain(w http.ResponseWriter, r *http.Request, id int64) {
	// Check existence
	var domain string
	err := s.DB.QueryRow(`SELECT domain FROM failover_domains WHERE id = $1`, id).Scan(&domain)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if _, err := s.DB.Exec(`DELETE FROM failover_domains WHERE id = $1`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "failover_domain.deleted", "failover_domain", strconv.FormatInt(id, 10), map[string]any{"domain": domain}, nil, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

// triggerFailover handles POST /api/failover/domains/{id}/failover.
// It validates the request, invokes the orchestrator, maps errors, and returns the event.
func (s *Server) triggerFailover(w http.ResponseWriter, r *http.Request, domainID int64) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		ToNodeID int64  `json:"to_node_id"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.ToNodeID == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "to_node_id_required"})
		return
	}

	// Normalize reason: default to "manual" if blank, truncate to 255 chars
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = "manual"
	}
	if len(reason) > 255 {
		reason = reason[:255]
	}

	admin, _, _ := s.currentAdmin(r)

	event, err := s.failoverOrchestrator.TriggerFailover(r.Context(), domainID, in.ToNodeID, reason, admin)
	if err != nil {
		errStr := err.Error()
		switch {
		case strings.Contains(errStr, "domain_not_found"):
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		case strings.Contains(errStr, "same_node"):
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "same_node", "message": "Target node is the same as the current node"})
		case strings.Contains(errStr, "node_offline"):
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_offline", "message": "Target node is offline or unreachable"})
		case strings.Contains(errStr, "failover_in_progress"):
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "failover_in_progress", "message": "A failover is already in progress for this domain"})
		default:
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "failover_failed", "message": errStr})
		}
		return
	}

	s.logAudit(admin, "failover_domain.failover_triggered", "failover_domain", strconv.FormatInt(domainID, 10), nil, map[string]any{
		"to_node_id": in.ToNodeID,
		"reason":     reason,
	}, clientIP(r))

	writeJSON(w, map[string]any{"ok": true, "event": event})
}

// failoverStatusResponse is the response for GET /api/failover/domains/{id}/status.
type failoverStatusResponse struct {
	Domain          string         `json:"domain"`
	CurrentNodeID   int64          `json:"current_node_id"`
	CurrentNodeName string         `json:"current_node_name"`
	CurrentNodeIP   string         `json:"current_node_ip"`
	IsActive        bool           `json:"is_active"`
	LastFailoverAt  *string        `json:"last_failover_at"`
	LatestEvent     *FailoverEvent `json:"latest_event"`
	DNSHealthy      bool           `json:"dns_healthy"`
}

// failoverDomainStatus returns the current failover health status of a domain,
// including the latest event and a live DNS health check.
func (s *Server) failoverDomainStatus(w http.ResponseWriter, r *http.Request, domainID int64) {
	// 1. Query failover_domains joined with nodes for domain record
	var domain string
	var currentNodeID int64
	var currentNodeName, currentNodeIP string
	var isActive bool
	var lastFailoverAt sql.NullString

	err := s.DB.QueryRow(`
		SELECT fd.domain, fd.current_node_id, COALESCE(n.name, ''), COALESCE(n.public_ip, ''),
		       fd.is_active, fd.last_failover_at
		FROM failover_domains fd
		LEFT JOIN nodes n ON n.id = fd.current_node_id
		WHERE fd.id = $1`, domainID).Scan(
		&domain, &currentNodeID, &currentNodeName, &currentNodeIP,
		&isActive, &lastFailoverAt,
	)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// 2. Query the latest failover_events row
	var latestEvent *FailoverEvent
	var evt FailoverEvent
	var errMsg sql.NullString
	var propStarted sql.NullString
	var propCompleted sql.NullString

	err = s.DB.QueryRow(`
		SELECT id, domain_id, from_node_id, to_node_id, reason, status,
		       dns_propagation_started_at, dns_propagation_completed_at,
		       triggered_by, error_message, created_at
		FROM failover_events
		WHERE domain_id = $1
		ORDER BY id DESC LIMIT 1`, domainID).Scan(
		&evt.ID, &evt.DomainID, &evt.FromNodeID, &evt.ToNodeID,
		&evt.Reason, &evt.Status,
		&propStarted, &propCompleted,
		&evt.TriggeredBy, &errMsg, &evt.CreatedAt,
	)
	if err == nil {
		if propStarted.Valid {
			evt.DNSPropagationStartedAt = &propStarted.String
		}
		if propCompleted.Valid {
			evt.DNSPropagationCompletedAt = &propCompleted.String
		}
		if errMsg.Valid {
			evt.ErrorMessage = &errMsg.String
		}
		latestEvent = &evt
	} else if err != sql.ErrNoRows {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// 3. Perform live DNS health check
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	dnsHealthy := false
	ips, err := net.DefaultResolver.LookupHost(ctx, domain)
	if err == nil {
		for _, ip := range ips {
			if ip == currentNodeIP {
				dnsHealthy = true
				break
			}
		}
	}

	// 4. Build response
	var lastFailoverPtr *string
	if lastFailoverAt.Valid {
		lastFailoverPtr = &lastFailoverAt.String
	}

	resp := failoverStatusResponse{
		Domain:          domain,
		CurrentNodeID:   currentNodeID,
		CurrentNodeName: currentNodeName,
		CurrentNodeIP:   currentNodeIP,
		IsActive:        isActive,
		LastFailoverAt:  lastFailoverPtr,
		LatestEvent:     latestEvent,
		DNSHealthy:      dnsHealthy,
	}

	// 5. Return response
	writeJSON(w, map[string]any{"ok": true, "status": resp})
}
