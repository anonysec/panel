//go:build !lite

package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"KorisPanel/panel/internal/payment"
)

// handleGatewayList dispatches /api/gateways requests.
func (s *Server) handleGatewayList(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listGateways(w, r)
	case http.MethodPost:
		s.createGateway(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGatewayByID dispatches /api/gateways/{id} requests.
func (s *Server) handleGatewayByID(w http.ResponseWriter, r *http.Request) {
	id, _, ok := pathID(r.URL.Path, "/api/gateways/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	switch r.Method {
	case http.MethodPatch:
		s.updateGateway(w, r, id)
	case http.MethodDelete:
		s.deleteGateway(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listGateways returns all payment gateways ordered by created_at.
// GET /api/gateways
func (s *Server) listGateways(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(`
		SELECT id, name, display_name, COALESCE(config_json, '{}'), is_active, created_at
		FROM payment_gateways
		ORDER BY created_at`)
	if err != nil {
		log.Printf("[payment] list gateways query failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}
	defer rows.Close()

	type gateway struct {
		ID          int64           `json:"id"`
		Name        string          `json:"name"`
		DisplayName string          `json:"display_name"`
		ConfigJSON  json.RawMessage `json:"config_json"`
		IsActive    bool            `json:"is_active"`
		CreatedAt   string          `json:"created_at"`
	}

	var gateways []gateway
	for rows.Next() {
		var g gateway
		var isActive bool
		var configStr string
		if err := rows.Scan(&g.ID, &g.Name, &g.DisplayName, &configStr, &isActive, &g.CreatedAt); err != nil {
			log.Printf("[payment] scan gateway error: %v", err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
			return
		}
		g.IsActive = isActive
		g.ConfigJSON = json.RawMessage(configStr)
		gateways = append(gateways, g)
	}
	if err := rows.Err(); err != nil {
		log.Printf("[payment] rows error: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	if gateways == nil {
		gateways = []gateway{}
	}

	writeJSON(w, map[string]any{"ok": true, "gateways": gateways})
}

// createGateway registers a new payment gateway.
// POST /api/gateways
func (s *Server) createGateway(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		Name        string          `json:"name"`
		DisplayName string          `json:"display_name"`
		ConfigJSON  json.RawMessage `json:"config_json"`
		IsActive    *bool           `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	if in.Name == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_required"})
		return
	}
	if in.DisplayName == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "display_name_required"})
		return
	}

	isActive := 1
	if in.IsActive != nil && !*in.IsActive {
		isActive = 0
	}

	configStr := "{}"
	if len(in.ConfigJSON) > 0 {
		configStr = string(in.ConfigJSON)
	}

	result, err := s.DB.Exec(
		`INSERT INTO payment_gateways (name, display_name, config_json, is_active) VALUES ($1, $2, $3, $4)`,
		in.Name, in.DisplayName, configStr, isActive,
	)
	if err != nil {
		log.Printf("[payment] insert gateway failed: %v", err)
		if strings.Contains(err.Error(), "Duplicate") {
			writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "gateway_already_exists"})
			return
		}
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	id, _ := result.LastInsertId()

	// Register in gateway registry if it matches a known gateway type
	if s.PaymentRegistry != nil {
		s.registerKnownGateway(in.Name, configStr)
	}

	writeJSON(w, map[string]any{"ok": true, "id": id})
}

// updateGateway updates an existing payment gateway.
// PATCH /api/gateways/{id}
func (s *Server) updateGateway(w http.ResponseWriter, r *http.Request, id int64) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		DisplayName *string         `json:"display_name"`
		ConfigJSON  json.RawMessage `json:"config_json"`
		IsActive    *bool           `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Build dynamic UPDATE query
	var setClauses []string
	var args []any

	if in.DisplayName != nil {
		setClauses = append(setClauses, "display_name = $1")
		args = append(args, *in.DisplayName)
	}
	if len(in.ConfigJSON) > 0 {
		setClauses = append(setClauses, "config_json = $1")
		args = append(args, string(in.ConfigJSON))
	}
	if in.IsActive != nil {
		active := 0
		if *in.IsActive {
			active = 1
		}
		setClauses = append(setClauses, "is_active = $1")
		args = append(args, active)
	}

	if len(setClauses) == 0 {
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	args = append(args, id)
	query := "UPDATE payment_gateways SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"

	result, err := s.DB.Exec(query, args...)
	if err != nil {
		log.Printf("[payment] update gateway failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	writeJSON(w, map[string]any{"ok": true})
}

// deleteGateway removes a payment gateway.
// DELETE /api/gateways/{id}
func (s *Server) deleteGateway(w http.ResponseWriter, r *http.Request, id int64) {
	// Get the gateway name before deletion for registry deregistration
	var name string
	err := s.DB.QueryRow(`SELECT name FROM payment_gateways WHERE id = $1`, id).Scan(&name)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	_, err = s.DB.Exec(`DELETE FROM payment_gateways WHERE id = $1`, id)
	if err != nil {
		log.Printf("[payment] delete gateway failed: %v", err)
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// Deregister from gateway registry
	if s.PaymentRegistry != nil {
		s.PaymentRegistry.Deregister(name)
	}

	writeJSON(w, map[string]any{"ok": true})
}

// registerKnownGateway registers a gateway instance in the registry based on its name.
func (s *Server) registerKnownGateway(name, configJSON string) {
	switch name {
	case "zarinpal":
		var cfg struct {
			MerchantID string `json:"merchant_id"`
			Sandbox    bool   `json:"sandbox"`
		}
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			log.Printf("[payment] failed to parse zarinpal config: %v", err)
			return
		}
		if cfg.MerchantID == "" {
			log.Printf("[payment] zarinpal config missing merchant_id")
			return
		}
		gw := payment.NewZarinpal(cfg.MerchantID, cfg.Sandbox)
		s.PaymentRegistry.Register(gw)
		log.Printf("[payment] registered zarinpal gateway (sandbox=%v)", cfg.Sandbox)
	default:
		// Unknown gateway type — stored in DB but not registered in runtime registry
		log.Printf("[payment] gateway '%s' stored but no runtime registration available", name)
	}
}
