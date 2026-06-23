//go:build !lite

package billing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRenderInvoiceHTML_BasicContent(t *testing.T) {
	inv := &Invoice{
		ID:            1,
		CustomerID:    42,
		InvoiceNumber: "INV-00001",
		Amount:        99.99,
		Currency:      "IRR",
		Status:        "paid",
		Type:          "subscription",
		Description:   "Monthly Plan - Premium",
		CreatedAt:     time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}
	branding := BrandingInfo{
		AppName: "KorisPanel",
		LogoURL: "https://example.com/logo.png",
		Address: "123 Server Lane",
		Phone:   "+1-555-0100",
		Email:   "billing@example.com",
	}

	html, err := RenderInvoiceHTML(inv, branding)
	if err != nil {
		t.Fatalf("RenderInvoiceHTML failed: %v", err)
	}

	content := string(html)

	checks := []struct {
		name     string
		contains string
	}{
		{"invoice number", "INV-00001"},
		{"amount", "99.99"},
		{"currency", "IRR"},
		{"status", "paid"},
		{"description", "Monthly Plan - Premium"},
		{"brand name", "KorisPanel"},
		{"logo", "https://example.com/logo.png"},
		{"address", "123 Server Lane"},
		{"phone", "555-0100"},
		{"email", "billing@example.com"},
		{"date", "2025-01-15"},
		{"customer id", "Customer #42"},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(content, tc.contains) {
				t.Errorf("expected HTML to contain %q", tc.contains)
			}
		})
	}
}

func TestRenderInvoiceHTML_NilInvoice(t *testing.T) {
	branding := BrandingInfo{AppName: "Test"}
	_, err := GenerateInvoicePDF(nil, branding)
	if err == nil {
		t.Fatal("expected error for nil invoice")
	}
}

func TestRenderInvoiceHTML_EmptyBranding(t *testing.T) {
	inv := &Invoice{
		ID:            1,
		CustomerID:    1,
		InvoiceNumber: "INV-TEST",
		Amount:        10.00,
		Currency:      "USD",
		Status:        "draft",
		Description:   "Test",
		CreatedAt:     time.Now(),
	}

	html, err := RenderInvoiceHTML(inv, BrandingInfo{})
	if err != nil {
		t.Fatalf("RenderInvoiceHTML with empty branding failed: %v", err)
	}

	content := string(html)
	if !strings.Contains(content, "INV-TEST") {
		t.Error("expected HTML to contain invoice number")
	}
	// Should not contain logo img tag when LogoURL is empty
	if strings.Contains(content, "<img") {
		t.Error("expected no img tag when LogoURL is empty")
	}
}

func TestGenerateInvoicePDFWithUsername(t *testing.T) {
	inv := &Invoice{
		ID:            5,
		CustomerID:    100,
		InvoiceNumber: "INV-00005",
		Amount:        250.00,
		Currency:      "IRR",
		Status:        "paid",
		Type:          "subscription",
		Description:   "VPN Premium Plan",
		CreatedAt:     time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	branding := BrandingInfo{
		AppName: "KorisPanel",
		Email:   "admin@korispanel.com",
	}

	result, err := GenerateInvoicePDFWithUsername(inv, branding, "john_doe", "Premium 30-Day")
	if err != nil {
		t.Fatalf("GenerateInvoicePDFWithUsername failed: %v", err)
	}

	content := string(result)
	if !strings.Contains(content, "john_doe") {
		t.Error("expected output to contain username")
	}
	if !strings.Contains(content, "Premium 30-Day") {
		t.Error("expected output to contain plan name")
	}
}

func TestSaveInvoicePDF_NilInvoice(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := New(db)
	branding := BrandingInfo{AppName: "Test"}

	_, err = engine.SaveInvoicePDF(context.Background(), nil, branding)
	if err == nil {
		t.Fatal("expected error for nil invoice")
	}
}

func TestGenerateInvoicePDF_FallbackReturnsHTML(t *testing.T) {
	// On systems without wkhtmltopdf, GenerateInvoicePDF should
	// fall back to returning HTML bytes.
	inv := &Invoice{
		ID:            1,
		CustomerID:    1,
		InvoiceNumber: "INV-FALLBACK",
		Amount:        50.00,
		Currency:      "USD",
		Status:        "draft",
		Description:   "Test fallback",
		CreatedAt:     time.Now(),
	}
	branding := BrandingInfo{AppName: "TestPanel"}

	result, err := GenerateInvoicePDF(inv, branding)
	if err != nil {
		t.Fatalf("GenerateInvoicePDF failed: %v", err)
	}

	// The result should be valid HTML (either PDF if wkhtmltopdf is installed,
	// or HTML fallback). Either way, we should have non-empty output.
	if len(result) == 0 {
		t.Fatal("expected non-empty result from GenerateInvoicePDF")
	}

	content := string(result)
	// If it's the HTML fallback, it should contain our invoice number
	if strings.Contains(content, "<!DOCTYPE html>") {
		if !strings.Contains(content, "INV-FALLBACK") {
			t.Error("HTML fallback should contain invoice number")
		}
	}
	// If it starts with %PDF, it's a valid PDF (wkhtmltopdf was available)
}
