//go:build !lite

package api

import (
	"fmt"
	"strings"
	"testing"
)

// TestDefaultContentPassesBlocklist verifies that all i18n default decoy content
// is free of VPN-related blocklist terms in all supported languages.
func TestDefaultContentPassesBlocklist(t *testing.T) {
	expectedLanguages := []string{"en", "fa", "zh", "ru"}

	for _, lang := range expectedLanguages {
		content, ok := DefaultContent[lang]
		if !ok {
			t.Fatalf("DefaultContent missing language: %s", lang)
		}

		violations := CheckLandingContentBlocklist(&content)
		for field, term := range violations {
			t.Errorf("[%s] field %q contains blocklist term %q",
				lang, field, term)
		}
	}
}

// TestDefaultContentNoPortalLinks verifies that default content does not contain
// links to /portal/ or /dashboard/.
func TestDefaultContentNoPortalLinks(t *testing.T) {
	forbidden := []string{"/portal/", "/dashboard/"}

	for lang, content := range DefaultContent {
		fields := collectContentFields(lang, content)

		for fieldName, text := range fields {
			for _, link := range forbidden {
				if strings.Contains(text, link) {
					t.Errorf("[%s] field %q contains forbidden link %q", lang, fieldName, link)
				}
			}
		}
	}
}

// TestDefaultContentShowPanelLinkDisabled verifies that all default content has
// show_panel_link set to false and panel_link_placement set to "hidden".
func TestDefaultContentShowPanelLinkDisabled(t *testing.T) {
	for lang, content := range DefaultContent {
		if content.ShowPanelLink {
			t.Errorf("[%s] ShowPanelLink should be false in default content", lang)
		}
		if content.PanelLinkPlacement != "hidden" {
			t.Errorf("[%s] PanelLinkPlacement should be \"hidden\", got %q", lang, content.PanelLinkPlacement)
		}
	}
}

// TestDefaultContentAllLanguagesPresent verifies all four required languages are present.
func TestDefaultContentAllLanguagesPresent(t *testing.T) {
	required := []string{"en", "fa", "zh", "ru"}
	for _, lang := range required {
		if _, ok := DefaultContent[lang]; !ok {
			t.Errorf("DefaultContent is missing required language: %s", lang)
		}
	}
}

// TestDefaultContentFieldsNonEmpty verifies that critical fields are not empty.
func TestDefaultContentFieldsNonEmpty(t *testing.T) {
	for lang, content := range DefaultContent {
		if content.HeroTitle == "" {
			t.Errorf("[%s] HeroTitle is empty", lang)
		}
		if content.HeroSubtitle == "" {
			t.Errorf("[%s] HeroSubtitle is empty", lang)
		}
		if len(content.Features) == 0 {
			t.Errorf("[%s] Features list is empty", lang)
		}
		if len(content.Pricing) == 0 {
			t.Errorf("[%s] Pricing list is empty", lang)
		}
		if len(content.FAQ) == 0 {
			t.Errorf("[%s] FAQ list is empty", lang)
		}
		if content.FooterText == "" {
			t.Errorf("[%s] FooterText is empty", lang)
		}
	}
}

// TestCheckBlocklistDetectsViolations ensures the blocklist function correctly
// identifies VPN-related terms.
func TestCheckBlocklistDetectsViolations(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"This is a VPN service", "vpn"},
		{"Use our Proxy server", "proxy"},
		{"OpenVPN configuration", "vpn"}, // "vpn" matches first as substring of "openvpn"
		{"WireGuard protocol", "wireguard"},
		{"Powered by KorisPanel", "koris"},
		{"Clean text with no issues", ""},
		{"Business solutions for growth", ""},
		{"Cloud infrastructure services", ""},
	}

	for _, tc := range cases {
		result := CheckBlocklist(tc.input)
		if result != tc.expected {
			t.Errorf("CheckBlocklist(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

// collectContentFields extracts all text content from a LandingContent struct
// into a flat map for easier validation.
func collectContentFields(_ string, c LandingContent) map[string]string {
	fields := map[string]string{
		"hero_title":    c.HeroTitle,
		"hero_subtitle": c.HeroSubtitle,
		"footer_text":   c.FooterText,
	}

	for i, f := range c.Features {
		fields[fmt.Sprintf("features[%d].title", i)] = f.Title
		fields[fmt.Sprintf("features[%d].description", i)] = f.Description
	}

	for i, p := range c.Pricing {
		fields[fmt.Sprintf("pricing[%d].name", i)] = p.Name
		fields[fmt.Sprintf("pricing[%d].price", i)] = p.Price
		for j, feat := range p.Features {
			fields[fmt.Sprintf("pricing[%d].features[%d]", i, j)] = feat
		}
	}

	for i, faq := range c.FAQ {
		fields[fmt.Sprintf("faq[%d].question", i)] = faq.Question
		fields[fmt.Sprintf("faq[%d].answer", i)] = faq.Answer
	}

	return fields
}
