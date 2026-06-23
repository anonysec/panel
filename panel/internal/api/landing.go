//go:build !lite

package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"

	"KorisPanel/panel/internal/auth"
)

// handleAdminLanding handles GET/PATCH /api/admin/landing.
// Uses the landing_settings table (migration 060).
func (s *Server) handleAdminLanding(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.getAdminLanding(w, r)
	case http.MethodPatch:
		s.patchAdminLanding(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getAdminLanding returns landing_settings row as JSON.
func (s *Server) getAdminLanding(w http.ResponseWriter, r *http.Request) {
	var enabled int
	var title string
	var description, logoURL, heroContent sql.NullString

	err := s.DB.QueryRow(`SELECT enabled, title, COALESCE(description,''), COALESCE(logo_url,''), COALESCE(hero_content,'') FROM landing_settings WHERE id=1`).
		Scan(&enabled, &title, &description, &logoURL, &heroContent)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	writeJSON(w, map[string]any{
		"ok": true,
		"settings": map[string]any{
			"enabled":      enabled == 1,
			"title":        title,
			"description":  description.String,
			"logo_url":     logoURL.String,
			"hero_content": heroContent.String,
		},
	})
}

// patchAdminLanding partially updates landing_settings.
func (s *Server) patchAdminLanding(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var in struct {
		Enabled     *bool   `json:"enabled"`
		Title       *string `json:"title"`
		Description *string `json:"description"`
		LogoURL     *string `json:"logo_url"`
		HeroContent *string `json:"hero_content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Build dynamic UPDATE
	var sets []string
	var args []any

	if in.Enabled != nil {
		sets = append(sets, "enabled = ?")
		if *in.Enabled {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if in.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *in.Title)
	}
	if in.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *in.Description)
	}
	if in.LogoURL != nil {
		sets = append(sets, "logo_url = ?")
		args = append(args, *in.LogoURL)
	}
	if in.HeroContent != nil {
		sets = append(sets, "hero_content = ?")
		args = append(args, *in.HeroContent)
	}

	if len(sets) == 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "empty_body"})
		return
	}

	query := fmt.Sprintf("UPDATE landing_settings SET %s WHERE id=1", strings.Join(sets, ", "))
	if _, err := s.DB.Exec(query, args...); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// Invalidate the landing meta cache so the root page reflects changes
	s.InvalidateLandingMetaCache()

	writeJSON(w, map[string]any{"ok": true})
}

// serveLandingPage handles GET / — checks session, enabled flag, and renders HTML.
// If user has an active session, redirect to dashboard/portal.
// If landing is disabled, redirect to login (/portal/).
// If landing is enabled, serve server-side rendered HTML with title, description, pricing cards.
func (s *Server) serveLandingPage(w http.ResponseWriter, r *http.Request) {
	// Check for active admin session
	if _, _, ok := s.currentAdmin(r); ok {
		http.Redirect(w, r, "/dashboard/", http.StatusFound)
		return
	}
	// Check for active customer session
	if _, ok := s.currentCustomer(r); ok {
		http.Redirect(w, r, "/portal/", http.StatusFound)
		return
	}

	// Check landing_settings enabled flag
	var enabled int
	var title string
	var description, logoURL, heroContent sql.NullString

	err := s.DB.QueryRow(`SELECT enabled, title, COALESCE(description,''), COALESCE(logo_url,''), COALESCE(hero_content,'') FROM landing_settings WHERE id=1`).
		Scan(&enabled, &title, &description, &logoURL, &heroContent)
	if err != nil || enabled == 0 {
		// Landing disabled — redirect to login
		http.Redirect(w, r, "/portal/", http.StatusFound)
		return
	}

	// Fetch active plans for pricing cards
	type PricingPlan struct {
		Name     string
		Price    float64
		Features []string
	}
	var plans []PricingPlan

	rows, err := s.DB.Query(`SELECT name, price, features FROM plans WHERE is_active = 1 ORDER BY price ASC`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p PricingPlan
			var featuresRaw sql.NullString
			if err := rows.Scan(&p.Name, &p.Price, &featuresRaw); err != nil {
				continue
			}
			p.Features = []string{}
			if featuresRaw.Valid && featuresRaw.String != "" {
				var parsed []string
				if err := json.Unmarshal([]byte(featuresRaw.String), &parsed); err == nil {
					p.Features = parsed
				}
			}
			plans = append(plans, p)
		}
	}

	// Render server-side HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	safeTitle := html.EscapeString(title)
	safeDesc := html.EscapeString(description.String)
	safeLogo := html.EscapeString(logoURL.String)
	safeHero := heroContent.String // hero_content is raw HTML by design

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="en" dir="ltr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>`)
	b.WriteString(safeTitle)
	b.WriteString(`</title>
<meta name="description" content="`)
	b.WriteString(safeDesc)
	b.WriteString(`">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;line-height:1.6;color:#1a1a2e;background:#f8f9fa}
.container{max-width:1200px;margin:0 auto;padding:0 1.5rem}
header{background:#fff;border-bottom:1px solid #e9ecef;padding:1rem 0}
header .container{display:flex;align-items:center;justify-content:space-between}
.logo{display:flex;align-items:center;gap:.75rem;font-size:1.25rem;font-weight:700;color:#1a1a2e;text-decoration:none}
.logo img{height:36px;width:auto}
.nav-actions a{display:inline-block;padding:.5rem 1.25rem;border-radius:6px;text-decoration:none;font-weight:500;font-size:.9rem}
.btn-login{color:#4361ee;border:1px solid #4361ee;margin-right:.5rem}
.btn-login:hover{background:#4361ee;color:#fff}
.btn-register{background:#4361ee;color:#fff}
.btn-register:hover{background:#3a56d4}
.hero{padding:4rem 0;text-align:center;background:linear-gradient(135deg,#667eea 0%,#764ba2 100%);color:#fff}
.hero h1{font-size:2.5rem;margin-bottom:1rem}
.hero p{font-size:1.2rem;opacity:.9;max-width:600px;margin:0 auto 2rem}
.hero-content{margin-top:1.5rem}
.pricing{padding:4rem 0}
.pricing h2{text-align:center;font-size:2rem;margin-bottom:2rem;color:#1a1a2e}
.pricing-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:1.5rem}
.plan-card{background:#fff;border:1px solid #e9ecef;border-radius:12px;padding:2rem;text-align:center;transition:transform .2s,box-shadow .2s}
.plan-card:hover{transform:translateY(-4px);box-shadow:0 8px 24px rgba(0,0,0,.1)}
.plan-card h3{font-size:1.25rem;margin-bottom:.5rem;color:#1a1a2e}
.plan-card .price{font-size:2rem;font-weight:700;color:#4361ee;margin-bottom:1rem}
.plan-card .price small{font-size:.875rem;color:#6c757d;font-weight:400}
.plan-card ul{list-style:none;text-align:left;margin-bottom:1.5rem}
.plan-card ul li{padding:.4rem 0;border-bottom:1px solid #f1f3f5;font-size:.9rem}
.plan-card ul li:before{content:"✓ ";color:#4361ee;font-weight:700}
.plan-card .btn-select{display:inline-block;padding:.6rem 1.5rem;background:#4361ee;color:#fff;border-radius:6px;text-decoration:none;font-weight:500}
.plan-card .btn-select:hover{background:#3a56d4}
footer{padding:2rem 0;text-align:center;color:#6c757d;font-size:.85rem;border-top:1px solid #e9ecef;margin-top:2rem}
@media(max-width:768px){
.hero h1{font-size:1.75rem}
.hero p{font-size:1rem}
.pricing-grid{grid-template-columns:1fr}
header .container{flex-wrap:wrap;gap:.5rem}
}
</style>
</head>
<body>
<header>
<div class="container">
<a href="/" class="logo">`)
	if safeLogo != "" {
		b.WriteString(`<img src="`)
		b.WriteString(safeLogo)
		b.WriteString(`" alt="`)
		b.WriteString(safeTitle)
		b.WriteString(`">`)
	}
	b.WriteString(safeTitle)
	b.WriteString(`</a>
<nav class="nav-actions">
<a href="/portal/" class="btn-login">Login</a>
<a href="/portal/" class="btn-register">Register</a>
</nav>
</div>
</header>
<section class="hero">
<div class="container">
<h1>`)
	b.WriteString(safeTitle)
	b.WriteString(`</h1>
<p>`)
	b.WriteString(safeDesc)
	b.WriteString(`</p>`)
	if safeHero != "" {
		b.WriteString(`<div class="hero-content">`)
		b.WriteString(safeHero)
		b.WriteString(`</div>`)
	}
	b.WriteString(`
</div>
</section>
`)
	// Pricing section
	if len(plans) > 0 {
		b.WriteString(`<section class="pricing">
<div class="container">
<h2>Plans &amp; Pricing</h2>
<div class="pricing-grid">
`)
		for _, p := range plans {
			b.WriteString(`<div class="plan-card">
<h3>`)
			b.WriteString(html.EscapeString(p.Name))
			b.WriteString(`</h3>
<div class="price">`)
			b.WriteString(fmt.Sprintf("%.0f", p.Price))
			b.WriteString(`<small>/mo</small></div>
<ul>`)
			for _, f := range p.Features {
				b.WriteString(`<li>`)
				b.WriteString(html.EscapeString(f))
				b.WriteString(`</li>`)
			}
			b.WriteString(`</ul>
<a href="/portal/" class="btn-select">Get Started</a>
</div>
`)
		}
		b.WriteString(`</div>
</div>
</section>
`)
	}

	b.WriteString(`<footer>
<div class="container">
<p>&copy; `)
	b.WriteString(safeTitle)
	b.WriteString(` — All rights reserved.</p>
</div>
</footer>
</body>
</html>`)

	w.Write([]byte(b.String()))
}

// hasActiveSession checks if the request has either an admin or customer session cookie.
// This is a lightweight check (cookie presence only, no DB validation) for routing decisions.
func (s *Server) hasActiveSession(r *http.Request) bool {
	if _, ok := auth.ReadSession(r, auth.AdminCookieName, s.Config.SessionSecret); ok {
		return true
	}
	if _, ok := auth.ReadSession(r, auth.CustomerCookieName, s.Config.SessionSecret); ok {
		return true
	}
	return false
}
