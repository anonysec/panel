// Package web embeds the pre-built admin and portal frontend assets
// directly into the Go binary. This eliminates the need for external
// www/ directories on the server — the binary is fully self-contained.
package web

import "embed"

// AdminFS contains the built admin dashboard SPA (panel/web/admin/www/).
//
//go:embed all:admin/www
var AdminFS embed.FS

// PortalFS contains the built customer portal SPA (panel/web/portal/www/).
//
//go:embed all:portal/www
var PortalFS embed.FS

// LandingFS contains the built landing page SPA (panel/web/landing/www/).
//
//go:embed all:landing/www
var LandingFS embed.FS
