package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"KorisPanel/panel/internal/noderegistry"
)

// getKnodeNodeDomain handles GET /api/admin/knode/nodes/{id}/domain.
// Returns the stored domain for a knode connection.
func (s *Server) getKnodeNodeDomain(w http.ResponseWriter, r *http.Request, id int64) {
	if s.NodeRegistry == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "node_registry_not_configured"})
		return
	}

	reg, ok := s.NodeRegistry.(*noderegistry.DBRegistry)
	if !ok {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "node_registry_type_mismatch"})
		return
	}

	domain, err := reg.GetDomain(r.Context(), id)
	if err != nil {
		if errors.Is(err, noderegistry.ErrNodeNotFound) {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		log.Printf("[knode-domain] GetDomain failed for node %d: %v", id, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "get_domain_failed"})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "domain": domain})
}

// setKnodeNodeDomain handles PUT /api/admin/knode/nodes/{id}/domain.
// Sets or clears the domain for a knode connection, with optional DNS validation.
func (s *Server) setKnodeNodeDomain(w http.ResponseWriter, r *http.Request, id int64) {
	if s.NodeRegistry == nil {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "node_registry_not_configured"})
		return
	}

	reg, ok := s.NodeRegistry.(*noderegistry.DBRegistry)
	if !ok {
		writeJSONCode(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "node_registry_type_mismatch"})
		return
	}

	limitBody(w, r, maxJSONBody)
	var in struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Set domain in database
	if err := reg.SetDomain(r.Context(), id, in.Domain); err != nil {
		if errors.Is(err, noderegistry.ErrNodeNotFound) {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		log.Printf("[knode-domain] SetDomain failed for node %d: %v", id, err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "set_domain_failed"})
		return
	}

	// DNS validation (non-blocking — only returns warnings)
	var warnings []string
	if in.Domain != "" {
		nodeIP, err := reg.GetNodeAddress(r.Context(), id)
		if err == nil && nodeIP != "" {
			dnsWarnings, validErr := noderegistry.ValidateNodeDomain(in.Domain, nodeIP)
			if validErr != nil {
				warnings = append(warnings, validErr.Error())
			} else {
				warnings = dnsWarnings
			}
		}
	}

	resp := map[string]any{"ok": true}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	writeJSON(w, resp)
}
