//go:build !lite

package api

import (
	"html"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// landingMetaHandler wraps the SPA handler for the landing page root path.
// For the exact "/" request:
//   - If user has an active session, redirect to /dashboard/ or /portal/
//   - If landing is disabled (landing_settings.enabled=0), redirect to login
//   - If the SPA landing page exists, serve it with injected meta tags
//   - Otherwise, fall back to the server-side rendered decoy landing page
//
// For all other paths (assets, SPA routes) it delegates to the normal spaHandler.
// All responses are wrapped with StripIdentifyingHeaders to remove Server,
// X-Powered-By, and any VPN-related X- headers.
func (s *Server) landingMetaHandler() http.Handler {
	fallback := spaHandler(s.Config.LandingWebDir, "/", s.LandingEmbedFS)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only handle the root path specially
		if r.URL.Path != "/" {
			fallback.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}

		// Check for active sessions — redirect authenticated users
		if _, _, ok := s.currentAdmin(r); ok {
			http.Redirect(w, r, "/dashboard/", http.StatusFound)
			return
		}
		if _, ok := s.currentCustomer(r); ok {
			http.Redirect(w, r, "/portal/", http.StatusFound)
			return
		}

		// Check landing_settings enabled flag
		var enabled bool
		err := s.DB.QueryRow(`SELECT enabled FROM landing_settings WHERE id=1`).Scan(&enabled)
		if err != nil || !enabled {
			// Landing disabled — redirect to login
			http.Redirect(w, r, "/portal/", http.StatusFound)
			return
		}

		// Try to serve SPA landing page with meta tags
		// Try to serve from cache
		s.landingMetaMu.RLock()
		cached := s.landingMetaRendered
		cachedMod := s.landingMetaModTime
		s.landingMetaMu.RUnlock()

		if cached != "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			http.ServeContent(w, r, "index.html", cachedMod, strings.NewReader(cached))
			return
		}

		// Read the raw index.html
		rawHTML := s.readLandingIndexHTML()
		if rawHTML == "" {
			// No SPA landing page — fall back to server-side rendered decoy landing
			s.serveDecoyLandingPage(w, r)
			return
		}

		// Build meta tags from DB
		metaTags := s.buildLandingMetaTags()

		// Replace placeholder
		rendered := strings.Replace(rawHTML, "<!--META_TAGS-->", metaTags, 1)

		// Cache the result
		now := time.Now()
		s.landingMetaMu.Lock()
		s.landingMetaRendered = rendered
		s.landingMetaModTime = now
		s.landingMetaMu.Unlock()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		http.ServeContent(w, r, "index.html", now, strings.NewReader(rendered))
	})

	return StripIdentifyingHeaders(inner)
}

// InvalidateLandingMetaCache clears the cached rendered landing HTML.
// Called when admin updates landing page config.
func (s *Server) InvalidateLandingMetaCache() {
	s.landingMetaMu.Lock()
	s.landingMetaRendered = ""
	s.landingMetaMu.Unlock()
}

// readLandingIndexHTML reads index.html from embedded FS or disk.
func (s *Server) readLandingIndexHTML() string {
	dir := s.Config.LandingWebDir
	embedded := s.LandingEmbedFS

	// Determine source: embedded or disk
	useEmbed := embedded != nil
	if dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
			useEmbed = false
		}
	}

	var f fs.File
	var err error
	if useEmbed {
		f, err = embedded.Open("index.html")
	} else {
		if dir == "" {
			return ""
		}
		f, err = os.Open(filepath.Join(dir, "index.html"))
	}
	if err != nil {
		return ""
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return string(data)
}

// buildLandingMetaTags generates the meta tag HTML from panel_settings.
func (s *Server) buildLandingMetaTags() string {
	title := "KorisPanel - VPN Management"
	description := "Fast, secure VPN management panel for service operators"
	ogURL := ""

	// Read landing settings from DB
	var headline, subheadline, domain string
	_ = s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = 'landing_hero_headline'`).Scan(&headline)
	_ = s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = 'landing_hero_subheadline'`).Scan(&subheadline)
	_ = s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = 'panel_domain'`).Scan(&domain)

	if headline != "" {
		title = headline
	}
	if subheadline != "" {
		description = subheadline
	}
	if domain != "" {
		ogURL = "https://" + domain
	}

	// Escape for safe HTML embedding
	safeTitle := html.EscapeString(title)
	safeDesc := html.EscapeString(description)

	var b strings.Builder
	b.WriteString("<title>")
	b.WriteString(safeTitle)
	b.WriteString("</title>\n")
	b.WriteString(`    <meta name="description" content="`)
	b.WriteString(safeDesc)
	b.WriteString("\">\n")
	b.WriteString(`    <meta property="og:title" content="`)
	b.WriteString(safeTitle)
	b.WriteString("\">\n")
	b.WriteString(`    <meta property="og:description" content="`)
	b.WriteString(safeDesc)
	b.WriteString("\">\n")
	b.WriteString(`    <meta property="og:type" content="website">`)
	if ogURL != "" {
		b.WriteString("\n")
		b.WriteString(`    <meta property="og:url" content="`)
		b.WriteString(html.EscapeString(ogURL))
		b.WriteString(`">`)
	}

	return b.String()
}
