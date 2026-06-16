package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
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
		// Failover trigger will be implemented in the orchestrator task
		writeJSONCode(w, http.StatusNotImplemented, map[string]any{"ok": false, "error": "not_implemented"})
	case "status":
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		// Status endpoint will be implemented in the orchestrator task
		writeJSONCode(w, http.StatusNotImplemented, map[string]any{"ok": false, "error": "not_implemented"})
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
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM failover_domains WHERE domain = ? AND is_active = 1`, in.Domain).Scan(&existingCount)
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
	err = s.DB.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, in.CurrentNodeID).Scan(&nodeExists)
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
		err = s.DB.QueryRow(`SELECT COUNT(*) FROM dns_providers WHERE id = ? AND is_active = 1`, *in.DNSProviderID).Scan(&providerExists)
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

	res, err := s.DB.Exec(`INSERT INTO failover_domains(domain, current_node_id, dns_provider_id, dns_record_id, ttl, is_active) VALUES(?,?,?,?,?,1)`,
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
		WHERE fd.id = ?`, id).Scan(
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
	err := s.DB.QueryRow(`SELECT id, domain, current_node_id, dns_provider_id, dns_record_id, ttl, is_active FROM failover_domains WHERE id = ?`, id).Scan(
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
		err := s.DB.QueryRow(`SELECT COUNT(*) FROM failover_domains WHERE domain = ? AND is_active = 1 AND id != ?`, domain, id).Scan(&existingCount)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if existingCount > 0 {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "domain_exists", "message": "An active failover domain with this name already exists"})
			return
		}
		sets = append(sets, "domain = ?")
		args = append(args, domain)
	}

	if in.CurrentNodeID != nil {
		var nodeExists int
		err := s.DB.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, *in.CurrentNodeID).Scan(&nodeExists)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if nodeExists == 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_not_found", "message": "Referenced node does not exist"})
			return
		}
		sets = append(sets, "current_node_id = ?")
		args = append(args, *in.CurrentNodeID)
	}

	if in.DNSProviderID != nil {
		if *in.DNSProviderID == 0 {
			// Allow clearing the provider
			sets = append(sets, "dns_provider_id = NULL")
		} else {
			var providerExists int
			err := s.DB.QueryRow(`SELECT COUNT(*) FROM dns_providers WHERE id = ? AND is_active = 1`, *in.DNSProviderID).Scan(&providerExists)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			if providerExists == 0 {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "provider_not_found", "message": "Referenced DNS provider does not exist or is inactive"})
				return
			}
			sets = append(sets, "dns_provider_id = ?")
			args = append(args, *in.DNSProviderID)
		}
	}

	if in.DNSRecordID != nil {
		sets = append(sets, "dns_record_id = ?")
		args = append(args, *in.DNSRecordID)
	}

	if in.TTL != nil {
		ttl := normalizeFailoverTTL(*in.TTL)
		sets = append(sets, "ttl = ?")
		args = append(args, ttl)
	}

	if in.IsActive != nil {
		sets = append(sets, "is_active = ?")
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
	err := s.DB.QueryRow(`SELECT domain FROM failover_domains WHERE id = ?`, id).Scan(&domain)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if _, err := s.DB.Exec(`DELETE FROM failover_domains WHERE id = ?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "failover_domain.deleted", "failover_domain", strconv.FormatInt(id, 10), map[string]any{"domain": domain}, nil, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}
