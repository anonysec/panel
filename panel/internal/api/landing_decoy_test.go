//go:build !lite

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateLandingContent_ValidContent(t *testing.T) {
	content := DefaultLandingContent()
	if err := ValidateLandingContent(content); err != nil {
		t.Fatalf("expected valid content to pass validation, got: %v", err)
	}
}

func TestValidateLandingContent_ExceedsFieldLength(t *testing.T) {
	content := DefaultLandingContent()
	// Set hero_title to exceed 5000 chars
	content.HeroTitle = strings.Repeat("a", MaxContentFieldLength+1)
	err := ValidateLandingContent(content)
	if err == nil {
		t.Fatal("expected validation error for hero_title exceeding max length")
	}
	if !strings.Contains(err.Error(), "hero_title") {
		t.Fatalf("expected error about hero_title, got: %v", err)
	}
}

func TestValidateLandingContent_ExactMaxLength(t *testing.T) {
	content := DefaultLandingContent()
	// Set hero_title to exactly 5000 chars — should pass
	content.HeroTitle = strings.Repeat("b", MaxContentFieldLength)
	if err := ValidateLandingContent(content); err != nil {
		t.Fatalf("expected content at max length to pass, got: %v", err)
	}
}

func TestValidateLandingContent_FeatureFieldExceedsLength(t *testing.T) {
	content := DefaultLandingContent()
	content.Features[0].Description = strings.Repeat("x", MaxContentFieldLength+1)
	err := ValidateLandingContent(content)
	if err == nil {
		t.Fatal("expected validation error for features[0].description")
	}
	if !strings.Contains(err.Error(), "features[0].description") {
		t.Fatalf("expected error about features[0].description, got: %v", err)
	}
}

func TestValidateLandingContent_FAQFieldExceedsLength(t *testing.T) {
	content := DefaultLandingContent()
	content.FAQ[0].Answer = strings.Repeat("y", MaxContentFieldLength+1)
	err := ValidateLandingContent(content)
	if err == nil {
		t.Fatal("expected validation error for faq[0].answer")
	}
	if !strings.Contains(err.Error(), "faq[0].answer") {
		t.Fatalf("expected error about faq[0].answer, got: %v", err)
	}
}

func TestValidateLandingContent_InvalidPlacement(t *testing.T) {
	content := DefaultLandingContent()
	content.PanelLinkPlacement = "invalid"
	err := ValidateLandingContent(content)
	if err == nil {
		t.Fatal("expected validation error for invalid panel_link_placement")
	}
	if !strings.Contains(err.Error(), "panel_link_placement") {
		t.Fatalf("expected error about panel_link_placement, got: %v", err)
	}
}

func TestStripIdentifyingHeaders(t *testing.T) {
	// Create a handler that sets various headers including identifying ones
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "nginx/1.24")
		w.Header().Set("X-Powered-By", "Go")
		w.Header().Set("X-Koris-Version", "1.0")
		w.Header().Set("X-VPN-Node", "us-east-1")
		w.Header().Set("X-Request-Id", "abc123")
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	})

	handler := StripIdentifyingHeaders(inner)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()

	// Server and X-Powered-By should be stripped
	if v := resp.Header.Get("Server"); v != "" {
		t.Errorf("expected Server header to be stripped, got: %q", v)
	}
	if v := resp.Header.Get("X-Powered-By"); v != "" {
		t.Errorf("expected X-Powered-By header to be stripped, got: %q", v)
	}

	// X-Koris-Version should be stripped (contains "koris")
	if v := resp.Header.Get("X-Koris-Version"); v != "" {
		t.Errorf("expected X-Koris-Version header to be stripped, got: %q", v)
	}

	// X-VPN-Node should be stripped (contains "vpn")
	if v := resp.Header.Get("X-VPN-Node"); v != "" {
		t.Errorf("expected X-VPN-Node header to be stripped, got: %q", v)
	}

	// X-Request-Id should remain (no VPN-related terms)
	if v := resp.Header.Get("X-Request-Id"); v != "abc123" {
		t.Errorf("expected X-Request-Id to remain, got: %q", v)
	}

	// Content-Type should remain
	if v := resp.Header.Get("Content-Type"); v != "text/html" {
		t.Errorf("expected Content-Type to remain, got: %q", v)
	}
}

func TestStripIdentifyingHeaders_VPNTermsInValue(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Type", "wireguard-node")
		w.Header().Set("X-Proxy-Info", "tunnel-active")
		w.Header().Set("X-Custom-Info", "safe-value")
		w.WriteHeader(http.StatusOK)
	})

	handler := StripIdentifyingHeaders(inner)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	resp := rec.Result()

	// X-Backend-Type value contains "wireguard" - should be stripped
	if v := resp.Header.Get("X-Backend-Type"); v != "" {
		t.Errorf("expected X-Backend-Type to be stripped (value contains wireguard), got: %q", v)
	}

	// X-Proxy-Info value contains "tunnel" - should be stripped
	if v := resp.Header.Get("X-Proxy-Info"); v != "" {
		t.Errorf("expected X-Proxy-Info to be stripped (value contains tunnel), got: %q", v)
	}

	// X-Custom-Info should remain
	if v := resp.Header.Get("X-Custom-Info"); v != "safe-value" {
		t.Errorf("expected X-Custom-Info to remain, got: %q", v)
	}
}

func TestDefaultLandingContent_NoPanelLinks(t *testing.T) {
	content := DefaultLandingContent()
	if content.ShowPanelLink {
		t.Error("default content should have ShowPanelLink=false")
	}
	if content.PanelLinkPlacement != "hidden" {
		t.Errorf("default content should have PanelLinkPlacement=hidden, got: %q", content.PanelLinkPlacement)
	}
}

func TestDefaultLandingContent_PassesValidation(t *testing.T) {
	content := DefaultLandingContent()
	if err := ValidateLandingContent(content); err != nil {
		t.Fatalf("default content should pass validation, got: %v", err)
	}
}

func TestValidateLandingContent_PricingFeatureExceedsLength(t *testing.T) {
	content := DefaultLandingContent()
	content.Pricing[0].Features[0] = strings.Repeat("z", MaxContentFieldLength+1)
	err := ValidateLandingContent(content)
	if err == nil {
		t.Fatal("expected validation error for pricing feature exceeding length")
	}
	if !strings.Contains(err.Error(), "pricing[0].features[0]") {
		t.Fatalf("expected error about pricing[0].features[0], got: %v", err)
	}
}

func TestValidateLandingContent_EmptyContent(t *testing.T) {
	content := &LandingContent{}
	if err := ValidateLandingContent(content); err != nil {
		t.Fatalf("empty content should pass validation, got: %v", err)
	}
}

func TestValidateLandingContent_ValidPlacements(t *testing.T) {
	validPlacements := []string{"footer", "nav", "hidden", ""}
	for _, placement := range validPlacements {
		content := &LandingContent{PanelLinkPlacement: placement}
		if err := ValidateLandingContent(content); err != nil {
			t.Errorf("placement %q should be valid, got: %v", placement, err)
		}
	}
}
