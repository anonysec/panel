package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// failoverProviders handles GET (list) and POST (create) for /api/failover/providers.
func (s *Server) failoverProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listFailoverProviders(w)
	case http.MethodPost:
		s.createFailoverProvider(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// failoverProviderByID handles PATCH, DELETE, and action sub-paths for /api/failover/providers/{id}.
func (s *Server) failoverProviderByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/failover/providers/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodPatch:
			s.updateFailoverProvider(w, r, id)
		case http.MethodDelete:
			s.deleteFailoverProvider(w, r, id)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "test":
		s.testFailoverProvider(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

// testFailoverProvider tests connectivity for a DNS provider.
// POST /api/failover/providers/{id}/test
func (s *Server) testFailoverProvider(w http.ResponseWriter, r *http.Request, id int64) {
	// Look up the provider
	var providerType, apiTokenEncrypted, zoneID string
	var isActive int
	err := s.DB.QueryRow(
		`SELECT type, api_token_encrypted, zone_id, is_active FROM dns_providers WHERE id = ? LIMIT 1`, id,
	).Scan(&providerType, &apiTokenEncrypted, &zoneID, &isActive)
	if err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found", "message": "Provider not found"})
		return
	}

	// Manual providers don't need connection testing
	if providerType == "manual" {
		writeJSON(w, map[string]any{"ok": true, "message": "Manual provider — no connection test needed"})
		return
	}

	// For Cloudflare: decrypt token and test API access
	if providerType == "cloudflare" {
		apiToken, err := decryptToken(apiTokenEncrypted)
		if err != nil {
			log.Printf("[failover] failed to decrypt API token for provider %d: %v", id, err)
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{
				"ok":      false,
				"error":   "decrypt_error",
				"message": "Failed to decrypt stored API token",
			})
			return
		}

		// Call Cloudflare GET /zones/{zone_id} to verify access
		cfURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s", zoneID)
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, cfURL, nil)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{
				"ok":      false,
				"error":   "request_error",
				"message": "Failed to create request",
			})
			return
		}
		req.Header.Set("Authorization", "Bearer "+apiToken)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			writeJSONCode(w, http.StatusBadGateway, map[string]any{
				"ok":      false,
				"error":   "connection_error",
				"message": fmt.Sprintf("Failed to connect to Cloudflare API: %v", err),
			})
			return
		}
		defer resp.Body.Close()

		// Read response body for error details
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

		if resp.StatusCode == http.StatusOK {
			writeJSON(w, map[string]any{"ok": true, "message": "Connection successful"})
			return
		}

		// On 401/403, mark provider inactive
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			_, dbErr := s.DB.Exec(`UPDATE dns_providers SET is_active = 0, updated_at = ? WHERE id = ?`, time.Now().UTC().Format("2006-01-02 15:04:05"), id)
			if dbErr != nil {
				log.Printf("[failover] failed to mark provider %d inactive: %v", id, dbErr)
			}

			// Parse Cloudflare error for detail
			var cfResp struct {
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors"`
			}
			detail := "API token is invalid or lacks required permissions"
			if json.Unmarshal(body, &cfResp) == nil && len(cfResp.Errors) > 0 {
				detail = cfResp.Errors[0].Message
			}

			writeJSONCode(w, http.StatusOK, map[string]any{
				"ok":      false,
				"error":   "invalid_token",
				"message": detail,
			})
			return
		}

		// Other error codes
		writeJSONCode(w, http.StatusOK, map[string]any{
			"ok":      false,
			"error":   "api_error",
			"message": fmt.Sprintf("Cloudflare API returned status %d: %s", resp.StatusCode, string(body)),
		})
		return
	}

	// Unknown provider type
	writeJSONCode(w, http.StatusBadRequest, map[string]any{
		"ok":      false,
		"error":   "unsupported_type",
		"message": fmt.Sprintf("Provider type %q does not support connection testing", providerType),
	})
}

// Placeholder CRUD handlers — these will be fully implemented in task 2.1.
func (s *Server) listFailoverProviders(w http.ResponseWriter) {
	rows, err := s.DB.Query(`SELECT id, name, type, zone_id, COALESCE(account_id,''), is_active, created_at, updated_at FROM dns_providers ORDER BY id DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	providers := []DNSProvider{}
	for rows.Next() {
		var p DNSProvider
		var isActive int
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.ZoneID, &p.AccountID, &isActive, &p.CreatedAt, &p.UpdatedAt); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		p.IsActive = isActive == 1
		providers = append(providers, p)
	}
	writeJSON(w, map[string]any{"ok": true, "providers": providers})
}

func (s *Server) createFailoverProvider(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		APIToken  string `json:"api_token"`
		ZoneID    string `json:"zone_id"`
		AccountID string `json:"account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = trimSpace(in.Name)
	in.Type = trimSpace(in.Type)
	if in.Name == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_required"})
		return
	}
	if in.Type != "cloudflare" && in.Type != "manual" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_type", "message": "Type must be cloudflare or manual"})
		return
	}
	if in.Type == "cloudflare" && (in.APIToken == "" || in.ZoneID == "") {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "credentials_required", "message": "api_token and zone_id required for cloudflare type"})
		return
	}
	encrypted := ""
	if in.APIToken != "" {
		var err error
		encrypted, err = encryptToken(in.APIToken)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "encryption_error"})
			return
		}
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	res, err := s.DB.Exec(`INSERT INTO dns_providers(name, type, api_token_encrypted, zone_id, account_id, is_active, created_at, updated_at) VALUES(?,?,?,?,?,1,?,?)`,
		in.Name, in.Type, encrypted, in.ZoneID, in.AccountID, now, now)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, map[string]any{"ok": true, "provider": DNSProvider{
		ID:        id,
		Name:      in.Name,
		Type:      in.Type,
		ZoneID:    in.ZoneID,
		AccountID: in.AccountID,
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}})
}

func (s *Server) updateFailoverProvider(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		Name      *string `json:"name"`
		APIToken  *string `json:"api_token"`
		ZoneID    *string `json:"zone_id"`
		AccountID *string `json:"account_id"`
		IsActive  *bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	// Verify provider exists
	var exists int
	if err := s.DB.QueryRow(`SELECT 1 FROM dns_providers WHERE id = ?`, id).Scan(&exists); err != nil {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if in.Name != nil {
		s.DB.Exec(`UPDATE dns_providers SET name = ?, updated_at = ? WHERE id = ?`, trimSpace(*in.Name), now, id)
	}
	if in.APIToken != nil && *in.APIToken != "" {
		encrypted, err := encryptToken(*in.APIToken)
		if err == nil {
			s.DB.Exec(`UPDATE dns_providers SET api_token_encrypted = ?, updated_at = ? WHERE id = ?`, encrypted, now, id)
		}
	}
	if in.ZoneID != nil {
		s.DB.Exec(`UPDATE dns_providers SET zone_id = ?, updated_at = ? WHERE id = ?`, *in.ZoneID, now, id)
	}
	if in.AccountID != nil {
		s.DB.Exec(`UPDATE dns_providers SET account_id = ?, updated_at = ? WHERE id = ?`, *in.AccountID, now, id)
	}
	if in.IsActive != nil {
		s.DB.Exec(`UPDATE dns_providers SET is_active = ?, updated_at = ? WHERE id = ?`, boolInt(*in.IsActive), now, id)
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) deleteFailoverProvider(w http.ResponseWriter, r *http.Request, id int64) {
	// Check if referenced by active failover domains
	var refCount int
	s.DB.QueryRow(`SELECT COUNT(*) FROM failover_domains WHERE dns_provider_id = ? AND is_active = 1`, id).Scan(&refCount)
	if refCount > 0 {
		writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "provider_in_use", "message": "Provider is referenced by active failover domains"})
		return
	}
	_, err := s.DB.Exec(`DELETE FROM dns_providers WHERE id = ?`, id)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// trimSpace is a helper (strings.TrimSpace).
func trimSpace(s string) string {
	return strings.TrimSpace(s)
}
