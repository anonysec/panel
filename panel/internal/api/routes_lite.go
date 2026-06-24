//go:build lite

package api

import "net/http"

// registerExcludedRoutes is a no-op in the lite build.
// Premium feature routes are not available in the lite edition.
func (s *Server) registerExcludedRoutes(_ *http.ServeMux) {}
