//go:build lite

package api

import (
	"encoding/json"
	"net/http"
)

// registerExcludedRoutes registers 404 handlers for all premium feature routes
// in the lite build. These routes are not available in the lite edition and
// return a structured JSON error response.
func (s *Server) registerExcludedRoutes(mux *http.ServeMux) {
	excluded := []string{
		// Payment methods
		"/api/payment-methods",
		"/api/payment-methods/",
		// Tickets
		"/api/tickets",
		"/api/tickets/",
		"/api/tickets/attachments/",
		"/api/admin/tickets",
		"/api/admin/tickets/",
		"/api/customer/tickets",
		"/api/customer/tickets/",
		// Billing / Payments
		"/api/payments",
		"/api/payments/",
		"/api/wallets/",
		"/api/admin/billing/upgrade",
		"/api/admin/billing/revenue",
		"/api/customer/billing/debt",
		"/api/customer/data-packs",
		"/api/customer/data-packs/buy",
		// Payment gateways
		"/api/gateways",
		"/api/gateways/",
		"/api/portal/pay",
		"/api/gateway/callback/",
		// Promo codes
		"/api/promo-codes",
		"/api/promo-codes/",
		"/api/portal/promo/apply",
		// Statistics / Reports
		"/api/admin/bandwidth-stats",
		"/api/reports/revenue",
		"/api/reports/users",
		"/api/reports/bandwidth",
		"/api/reports/uptime",
		"/api/reports/wallets",
		"/api/admin/reports/pdf",
		"/api/admin/statistics",
		// Load balancing
		"/api/admin/haproxy/apply",
		"/api/admin/haproxy/status",
		"/api/node-groups/load",
		// Reseller
		"/api/resellers",
		"/api/resellers/transactions",
		"/api/resellers/",
		"/api/resellers/checkout",
		"/api/resellers/payments",
		"/api/reseller/transactions",
		"/api/reseller/settings",
		"/api/reseller/dashboard",
		"/api/reseller/plan-prices",
		"/api/reseller/tickets",
		"/api/reseller/users/",
		// Billing exports
		"/api/export/payments.csv",
		"/api/export/wallet-transactions.csv",
		"/api/export/revenue.csv",
		// Xray
		"/api/xray/inbounds",
		"/api/xray/inbounds/",
		"/api/admin/xray/templates",
		"/api/admin/xray/templates/",
		"/api/portal/xray/subscription",
		"/api/portal/xray/links",
		"/api/sub/",
		// MTProto
		"/api/admin/mtproto",
		"/api/admin/mtproto/",
		// AnyConnect
		"/api/admin/anyconnect",
		"/api/admin/anyconnect/",
		"/api/portal/anyconnect/profile",
		// Telegram proxies
		"/api/admin/telegram-proxies",
		"/api/admin/telegram-proxies/",
		"/api/admin/telegram-proxies/rotate",
		"/api/customer/telegram-proxies",
		// Knowledge base
		"/api/admin/kb/articles",
		"/api/admin/kb/articles/",
		"/api/portal/kb",
		"/api/portal/kb/search",
		"/api/portal/kb/",
		// Invoices
		"/api/admin/invoices",
		"/api/admin/invoices/",
		"/api/portal/invoices",
		"/api/portal/invoices/",
		// SLA
		"/api/admin/sla/config",
		"/api/admin/sla/stats",
		// Canned responses
		"/api/admin/canned-responses",
		"/api/admin/canned-responses/",
		"/api/canned-responses",
		"/api/canned-responses/",
		// Custom fields
		"/api/admin/custom-fields",
		"/api/admin/custom-fields/",
		"/api/admin/customers/",
		// User tags / Segments
		"/api/admin/user-tags",
		"/api/admin/user-tags/",
		"/api/tags",
		"/api/tags/",
		"/api/filter-presets",
		"/api/filter-presets/",
		"/api/customers/filtered",
		"/api/admin/segments",
		"/api/admin/segments/",
		// LDAP
		"/api/admin/ldap/settings",
		"/api/admin/ldap/test",
		// Landing page builder
		"/api/public-plans",
	}

	for _, path := range excluded {
		mux.HandleFunc(path, featureNotAvailable)
	}

	// Landing page (root) — serve a minimal placeholder in lite build
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html><body><h1>Koris Panel</h1></body></html>"))
	})
}

// featureNotAvailable returns a 404 JSON response indicating the feature
// is not available in the lite edition.
func featureNotAvailable(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": "feature not available in lite edition",
	})
}
