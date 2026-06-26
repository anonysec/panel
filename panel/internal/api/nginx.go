package api

import (
	"encoding/json"
	"net/http"
)

// handleNginxStatusDeprecated handles GET /api/admin/nginx/status
// This endpoint is deprecated and returns HTTP 410 Gone with a pointer to the replacement.
// It will be fully removed after the 30-day deprecation window.
func (s *Server) handleNginxStatusDeprecated(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusGone)
	json.NewEncoder(w).Encode(map[string]any{
		"replacement": "/api/admin/nodes/health",
	})
}
