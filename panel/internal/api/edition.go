package api

import "net/http"

// handleInfo returns panel metadata including the active edition.
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"ok":      true,
		"edition": PanelEdition,
		"version": s.Config.Version,
	})
}
