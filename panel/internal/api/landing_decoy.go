//go:build !lite

package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"unicode/utf8"
)

// MaxContentFieldLength is the maximum allowed characters per content field (5000).
const MaxContentFieldLength = 5000

// vpnRelatedTerms are terms that identify VPN-related headers that must be stripped.
var vpnRelatedTerms = []string{
	"vpn", "proxy", "tunnel", "openvpn", "wireguard", "ikev2",
	"l2tp", "xray", "vless", "vmess", "trojan", "mtproto",
	"shadowsocks", "v2ray", "koris", "korispanel", "panel",
}

// Feature represents a single feature item in the landing page.
type Feature struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon,omitempty"`
}

// PricingTier represents a pricing tier on the landing page.
type PricingTier struct {
	Name        string   `json:"name"`
	Price       string   `json:"price"`
	Period      string   `json:"period,omitempty"`
	Features    []string `json:"features"`
	Highlighted bool     `json:"highlighted,omitempty"`
}

// FAQItem represents a FAQ entry on the landing page.
type FAQItem struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// LandingContent holds all configurable content for the decoy landing page.
type LandingContent struct {
	HeroTitle          string        `json:"hero_title"`
	HeroSubtitle       string        `json:"hero_subtitle"`
	Features           []Feature     `json:"features"`
	Pricing            []PricingTier `json:"pricing"`
	FAQ                []FAQItem     `json:"faq"`
	FooterText         string        `json:"footer_text"`
	ShowPanelLink      bool          `json:"show_panel_link"`
	PanelLinkPlacement string        `json:"panel_link_placement"` // "footer" | "nav" | "hidden"
}

// ValidateLandingContent validates all content fields are within the 5000 char limit.
// Returns an error describing the first field that exceeds the limit.
func ValidateLandingContent(c *LandingContent) error {
	if err := validateFieldLength("hero_title", c.HeroTitle); err != nil {
		return err
	}
	if err := validateFieldLength("hero_subtitle", c.HeroSubtitle); err != nil {
		return err
	}
	if err := validateFieldLength("footer_text", c.FooterText); err != nil {
		return err
	}

	for i, f := range c.Features {
		if err := validateFieldLength(fmt.Sprintf("features[%d].title", i), f.Title); err != nil {
			return err
		}
		if err := validateFieldLength(fmt.Sprintf("features[%d].description", i), f.Description); err != nil {
			return err
		}
	}

	for i, p := range c.Pricing {
		if err := validateFieldLength(fmt.Sprintf("pricing[%d].name", i), p.Name); err != nil {
			return err
		}
		if err := validateFieldLength(fmt.Sprintf("pricing[%d].price", i), p.Price); err != nil {
			return err
		}
		for j, feat := range p.Features {
			if err := validateFieldLength(fmt.Sprintf("pricing[%d].features[%d]", i, j), feat); err != nil {
				return err
			}
		}
	}

	for i, faq := range c.FAQ {
		if err := validateFieldLength(fmt.Sprintf("faq[%d].question", i), faq.Question); err != nil {
			return err
		}
		if err := validateFieldLength(fmt.Sprintf("faq[%d].answer", i), faq.Answer); err != nil {
			return err
		}
	}

	// Validate panel_link_placement
	switch c.PanelLinkPlacement {
	case "footer", "nav", "hidden", "":
		// valid
	default:
		return fmt.Errorf("invalid panel_link_placement: %q (must be footer, nav, or hidden)", c.PanelLinkPlacement)
	}

	return nil
}

// validateFieldLength checks that a field does not exceed MaxContentFieldLength characters.
func validateFieldLength(fieldName, value string) error {
	if utf8.RuneCountInString(value) > MaxContentFieldLength {
		return fmt.Errorf("field %q exceeds maximum length of %d characters", fieldName, MaxContentFieldLength)
	}
	return nil
}

// StripIdentifyingHeaders is an HTTP middleware that removes headers that could
// reveal the server's identity or VPN management purpose.
// Strips: Server, X-Powered-By, and any X- header containing VPN-related terms.
func StripIdentifyingHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use a response wrapper to intercept and strip headers before they're sent
		sw := &stripHeadersWriter{ResponseWriter: w, headerWritten: false}
		next.ServeHTTP(sw, r)
	})
}

// stripHeadersWriter wraps http.ResponseWriter to strip identifying headers
// before they are sent to the client.
type stripHeadersWriter struct {
	http.ResponseWriter
	headerWritten bool
}

func (sw *stripHeadersWriter) WriteHeader(statusCode int) {
	if !sw.headerWritten {
		sw.stripHeaders()
		sw.headerWritten = true
	}
	sw.ResponseWriter.WriteHeader(statusCode)
}

func (sw *stripHeadersWriter) Write(b []byte) (int, error) {
	if !sw.headerWritten {
		sw.stripHeaders()
		sw.headerWritten = true
	}
	return sw.ResponseWriter.Write(b)
}

func (sw *stripHeadersWriter) stripHeaders() {
	h := sw.ResponseWriter.Header()
	h.Del("Server")
	h.Del("X-Powered-By")

	// Remove any X- header containing VPN-related terms
	for key := range h {
		if strings.HasPrefix(key, "X-") || strings.HasPrefix(key, "x-") {
			keyLower := strings.ToLower(key)
			valueLower := strings.ToLower(strings.Join(h.Values(key), " "))
			for _, term := range vpnRelatedTerms {
				if strings.Contains(keyLower, term) || strings.Contains(valueLower, term) {
					h.Del(key)
					break
				}
			}
		}
	}
}

// DefaultLandingContent returns the default decoy landing page content
// for when no custom content is configured. Uses generic business terminology.
func DefaultLandingContent() *LandingContent {
	return &LandingContent{
		HeroTitle:    "Modern Business Solutions",
		HeroSubtitle: "Empowering teams with innovative tools for growth and collaboration.",
		Features: []Feature{
			{Title: "Cloud Infrastructure", Description: "Scalable and reliable cloud services tailored to your business needs."},
			{Title: "Data Analytics", Description: "Gain insights from your data with our advanced analytics platform."},
			{Title: "Team Collaboration", Description: "Work together seamlessly with integrated communication tools."},
		},
		Pricing: []PricingTier{
			{Name: "Starter", Price: "$9", Period: "/month", Features: []string{"5 users", "10 GB storage", "Email support"}},
			{Name: "Business", Price: "$29", Period: "/month", Features: []string{"25 users", "100 GB storage", "Priority support", "API access"}, Highlighted: true},
			{Name: "Enterprise", Price: "Custom", Period: "", Features: []string{"Unlimited users", "Unlimited storage", "Dedicated support", "Custom integrations"}},
		},
		FAQ: []FAQItem{
			{Question: "How do I get started?", Answer: "Sign up for a free trial and explore our platform with no commitment."},
			{Question: "What payment methods do you accept?", Answer: "We accept all major credit cards, PayPal, and bank transfers."},
			{Question: "Can I cancel anytime?", Answer: "Yes, you can cancel your subscription at any time with no penalties."},
		},
		FooterText:         "© 2024 Business Solutions Inc. All rights reserved.",
		ShowPanelLink:      false,
		PanelLinkPlacement: "hidden",
	}
}

// loadLandingContent loads the landing content from the database.
// Returns default content if no custom content is configured.
// Falls back to the i18n DefaultContent map (task 15.1) with language detection.
func (s *Server) loadLandingContent() *LandingContent {
	var contentJSON sql.NullString
	err := s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key = 'landing_decoy_content'`).Scan(&contentJSON)
	if err != nil || !contentJSON.Valid || contentJSON.String == "" {
		// Use i18n defaults if available, otherwise fallback to English
		if content, ok := DefaultContent["en"]; ok {
			return &content
		}
		return DefaultLandingContent()
	}

	var content LandingContent
	if err := json.Unmarshal([]byte(contentJSON.String), &content); err != nil {
		if c, ok := DefaultContent["en"]; ok {
			return &c
		}
		return DefaultLandingContent()
	}

	// Apply defaults for empty fields
	if content.HeroTitle == "" {
		content.HeroTitle = "Modern Business Solutions"
	}
	if content.PanelLinkPlacement == "" {
		content.PanelLinkPlacement = "hidden"
	}

	return &content
}

// serveDecoyLandingPage renders the decoy landing page HTML for unauthenticated visitors.
// It only includes panel links if ShowPanelLink is true.
func (s *Server) serveDecoyLandingPage(w http.ResponseWriter, r *http.Request) {
	content := s.loadLandingContent()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="en" dir="ltr">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>`)
	b.WriteString(html.EscapeString(content.HeroTitle))
	b.WriteString(`</title>
<meta name="description" content="`)
	b.WriteString(html.EscapeString(content.HeroSubtitle))
	b.WriteString(`">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;line-height:1.6;color:#1a1a2e;background:#f8f9fa}
.container{max-width:1200px;margin:0 auto;padding:0 1.5rem}
header{background:#fff;border-bottom:1px solid #e9ecef;padding:1rem 0}
header .container{display:flex;align-items:center;justify-content:space-between}
.logo{font-size:1.25rem;font-weight:700;color:#1a1a2e;text-decoration:none}
.nav-actions a{display:inline-block;padding:.5rem 1.25rem;border-radius:6px;text-decoration:none;font-weight:500;font-size:.9rem;background:#4361ee;color:#fff}
.nav-actions a:hover{background:#3a56d4}
.hero{padding:4rem 0;text-align:center;background:linear-gradient(135deg,#667eea 0%,#764ba2 100%);color:#fff}
.hero h1{font-size:2.5rem;margin-bottom:1rem}
.hero p{font-size:1.2rem;opacity:.9;max-width:600px;margin:0 auto}
.features{padding:4rem 0}
.features h2{text-align:center;font-size:2rem;margin-bottom:2rem}
.features-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:1.5rem}
.feature-card{background:#fff;border:1px solid #e9ecef;border-radius:12px;padding:2rem;text-align:center}
.feature-card h3{margin-bottom:.5rem;color:#1a1a2e}
.feature-card p{color:#6c757d;font-size:.9rem}
.pricing{padding:4rem 0;background:#f1f3f5}
.pricing h2{text-align:center;font-size:2rem;margin-bottom:2rem}
.pricing-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:1.5rem}
.plan-card{background:#fff;border:1px solid #e9ecef;border-radius:12px;padding:2rem;text-align:center}
.plan-card.highlighted{border-color:#4361ee;box-shadow:0 4px 12px rgba(67,97,238,.2)}
.plan-card h3{font-size:1.25rem;margin-bottom:.5rem}
.plan-card .price{font-size:2rem;font-weight:700;color:#4361ee;margin-bottom:1rem}
.plan-card .price small{font-size:.875rem;color:#6c757d;font-weight:400}
.plan-card ul{list-style:none;text-align:left;margin-bottom:1.5rem}
.plan-card ul li{padding:.4rem 0;border-bottom:1px solid #f1f3f5;font-size:.9rem}
.plan-card ul li:before{content:"\2713 ";color:#4361ee;font-weight:700}
.faq{padding:4rem 0}
.faq h2{text-align:center;font-size:2rem;margin-bottom:2rem}
.faq-list{max-width:800px;margin:0 auto}
.faq-item{background:#fff;border:1px solid #e9ecef;border-radius:8px;padding:1.5rem;margin-bottom:1rem}
.faq-item h4{margin-bottom:.5rem;color:#1a1a2e}
.faq-item p{color:#6c757d;font-size:.9rem}
footer{padding:2rem 0;text-align:center;color:#6c757d;font-size:.85rem;border-top:1px solid #e9ecef;margin-top:2rem}
@media(max-width:768px){
.hero h1{font-size:1.75rem}
.features-grid,.pricing-grid{grid-template-columns:1fr}
}
</style>
</head>
<body>
<header>
<div class="container">
<span class="logo">`)
	b.WriteString(html.EscapeString(content.HeroTitle))
	b.WriteString(`</span>`)

	// Only show nav link if operator explicitly enables panel links
	if content.ShowPanelLink && content.PanelLinkPlacement == "nav" {
		b.WriteString(`<nav class="nav-actions"><a href="/portal/">Sign In</a></nav>`)
	}

	b.WriteString(`
</div>
</header>
<section class="hero">
<div class="container">
<h1>`)
	b.WriteString(html.EscapeString(content.HeroTitle))
	b.WriteString(`</h1>
<p>`)
	b.WriteString(html.EscapeString(content.HeroSubtitle))
	b.WriteString(`</p>
</div>
</section>
`)

	// Features section
	if len(content.Features) > 0 {
		b.WriteString(`<section class="features">
<div class="container">
<h2>Features</h2>
<div class="features-grid">
`)
		for _, f := range content.Features {
			b.WriteString(`<div class="feature-card">
<h3>`)
			b.WriteString(html.EscapeString(f.Title))
			b.WriteString(`</h3>
<p>`)
			b.WriteString(html.EscapeString(f.Description))
			b.WriteString(`</p>
</div>
`)
		}
		b.WriteString(`</div>
</div>
</section>
`)
	}

	// Pricing section
	if len(content.Pricing) > 0 {
		b.WriteString(`<section class="pricing">
<div class="container">
<h2>Pricing</h2>
<div class="pricing-grid">
`)
		for _, p := range content.Pricing {
			cls := "plan-card"
			if p.Highlighted {
				cls = "plan-card highlighted"
			}
			b.WriteString(`<div class="`)
			b.WriteString(cls)
			b.WriteString(`">
<h3>`)
			b.WriteString(html.EscapeString(p.Name))
			b.WriteString(`</h3>
<div class="price">`)
			b.WriteString(html.EscapeString(p.Price))
			if p.Period != "" {
				b.WriteString(`<small>`)
				b.WriteString(html.EscapeString(p.Period))
				b.WriteString(`</small>`)
			}
			b.WriteString(`</div>
<ul>`)
			for _, feat := range p.Features {
				b.WriteString(`<li>`)
				b.WriteString(html.EscapeString(feat))
				b.WriteString(`</li>`)
			}
			b.WriteString(`</ul>
</div>
`)
		}
		b.WriteString(`</div>
</div>
</section>
`)
	}

	// FAQ section
	if len(content.FAQ) > 0 {
		b.WriteString(`<section class="faq">
<div class="container">
<h2>FAQ</h2>
<div class="faq-list">
`)
		for _, faq := range content.FAQ {
			b.WriteString(`<div class="faq-item">
<h4>`)
			b.WriteString(html.EscapeString(faq.Question))
			b.WriteString(`</h4>
<p>`)
			b.WriteString(html.EscapeString(faq.Answer))
			b.WriteString(`</p>
</div>
`)
		}
		b.WriteString(`</div>
</div>
</section>
`)
	}

	// Footer
	b.WriteString(`<footer>
<div class="container">
<p>`)
	b.WriteString(html.EscapeString(content.FooterText))
	b.WriteString(`</p>`)

	// Only show footer panel link if operator explicitly enables and placement is "footer"
	if content.ShowPanelLink && content.PanelLinkPlacement == "footer" {
		b.WriteString(`<p style="margin-top:.5rem"><a href="/portal/" style="color:#4361ee;text-decoration:none;font-size:.8rem">Panel</a></p>`)
	}

	b.WriteString(`
</div>
</footer>
</body>
</html>`)

	w.Write([]byte(b.String()))
}

// handleAdminLandingContent handles GET/PUT /api/admin/landing-content.
// GET  — returns the current decoy landing content.
// PUT  — replaces the decoy landing content (validates field lengths).
func (s *Server) handleAdminLandingContent(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		content := s.loadLandingContent()
		writeJSON(w, map[string]any{"ok": true, "content": content})
	case http.MethodPut:
		s.saveLandingContent(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// saveLandingContent validates and persists new decoy landing content.
func (s *Server) saveLandingContent(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)

	var content LandingContent
	if err := json.NewDecoder(r.Body).Decode(&content); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	// Validate field lengths
	if err := ValidateLandingContent(&content); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Serialize to JSON for storage
	data, err := json.Marshal(content)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "serialize_error"})
		return
	}

	_, err = s.DB.Exec(
		`INSERT INTO panel_settings (setting_key, setting_value) VALUES ('landing_decoy_content', $1) ON CONFLICT (setting_key) DO UPDATE SET setting_value = EXCLUDED.setting_value`,
		string(data),
	)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "db_error"})
		return
	}

	// Invalidate landing page cache
	s.InvalidateLandingMetaCache()

	writeJSON(w, map[string]any{"ok": true})
}
