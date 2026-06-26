//go:build !lite

package api

import "net/http"

// registerExcludedRoutes registers all premium feature routes.
// This method is only compiled in the full build.
func (s *Server) registerExcludedRoutes(mux *http.ServeMux) {
	// Payment methods
	mux.HandleFunc("/api/payment-methods", s.requireFullAdmin(s.paymentMethods))
	mux.HandleFunc("/api/payment-methods/", s.requireFullAdmin(s.paymentMethodByID))

	// Tickets
	mux.HandleFunc("/api/tickets", s.requireFullAdmin(s.tickets))
	mux.HandleFunc("/api/tickets/", s.requireFullAdmin(s.ticketByID))
	mux.HandleFunc("/api/tickets/attachments/", s.serveAttachment)
	mux.HandleFunc("/api/admin/tickets", s.requireFullAdmin(s.adminTickets))
	mux.HandleFunc("/api/admin/tickets/", s.requireFullAdmin(s.adminTicketByID))
	mux.HandleFunc("/api/customer/tickets", s.requireCustomer(s.customerTickets))
	mux.HandleFunc("/api/customer/tickets/", s.requireCustomer(s.customerTicketByID))

	// Billing / Payments
	mux.HandleFunc("/api/payments", s.requireFullAdmin(s.payments))
	mux.HandleFunc("/api/payments/", s.requireFullAdmin(s.paymentByID))
	mux.HandleFunc("/api/wallets/", s.requireFullAdmin(s.walletByUsername))
	mux.HandleFunc("/api/admin/billing/upgrade", s.requireFullAdmin(s.adminUpgradePlan))
	mux.HandleFunc("/api/admin/billing/revenue", s.requireAdmin(s.adminBillingRevenue))
	mux.HandleFunc("/api/customer/billing/debt", s.requireCustomer(s.customerBillingDebt))
	mux.HandleFunc("/api/customer/data-packs", s.requireCustomer(s.customerDataPacks))
	mux.HandleFunc("/api/customer/data-packs/buy", s.requireCustomer(s.customerBuyDataPack))

	// Payment gateways
	mux.HandleFunc("/api/gateways", s.requireFullAdmin(s.handleGatewayList))
	mux.HandleFunc("/api/gateways/", s.requireFullAdmin(s.handleGatewayByID))
	mux.HandleFunc("/api/portal/pay", s.requireCustomer(s.handlePaymentInitiate))
	mux.HandleFunc("/api/gateway/callback/", s.handleGatewayCallback)

	// Promo codes
	mux.HandleFunc("/api/promo-codes", s.requireFullAdmin(s.promoCodes))
	mux.HandleFunc("/api/promo-codes/", s.requireFullAdmin(s.promoCodeByID))
	mux.HandleFunc("/api/portal/promo/apply", s.requireCustomer(s.portalApplyPromo))

	// Statistics / Reports
	mux.HandleFunc("/api/admin/bandwidth-stats", s.requireFullAdmin(s.bandwidthStats))
	mux.HandleFunc("/api/reports/revenue", s.requireFullAdmin(s.revenueReport))
	mux.HandleFunc("/api/reports/users", s.requireFullAdmin(s.userReport))
	mux.HandleFunc("/api/reports/bandwidth", s.requireFullAdmin(s.bandwidthReport))
	mux.HandleFunc("/api/reports/uptime", s.requireFullAdmin(s.uptimeReport))
	mux.HandleFunc("/api/reports/wallets", s.requireFullAdmin(s.walletSummary))
	mux.HandleFunc("/api/admin/reports/pdf", s.requireFullAdmin(s.handleReportPDF))
	mux.HandleFunc("/api/admin/statistics", s.requireFullAdmin(s.statisticsGet))

	// Load balancing
	mux.HandleFunc("/api/admin/haproxy/apply", s.requireFullAdmin(s.haproxyApply))
	mux.HandleFunc("/api/admin/haproxy/status", s.requireFullAdmin(s.haproxyStatus))
	mux.HandleFunc("/api/node-groups/load", s.requireFullAdmin(s.handleNodeGroupsLoad))

	// Reseller
	mux.HandleFunc("/api/resellers", s.requireAdmin(s.resellers))
	mux.HandleFunc("/api/resellers/transactions", s.requireAdmin(s.resellerTransactions))
	mux.HandleFunc("/api/resellers/", s.requireAdmin(s.resellerByID))
	mux.HandleFunc("/api/resellers/checkout", s.requireAdmin(s.resellerCheckout))
	mux.HandleFunc("/api/resellers/payments", s.requireAdmin(s.resellerPayments))
	mux.HandleFunc("/api/reseller/transactions", s.requireAdmin(s.resellerTransactions))
	mux.HandleFunc("/api/reseller/settings", s.requireAdmin(s.resellerSettings))
	mux.HandleFunc("/api/reseller/dashboard", s.requireAdmin(s.resellerDashboard))
	mux.HandleFunc("/api/reseller/plan-prices", s.requireAdmin(s.resellerPlanPrices))
	mux.HandleFunc("/api/reseller/tickets", s.requireAdmin(s.resellerTickets))
	mux.HandleFunc("/api/reseller/users/", s.requireAdmin(s.resellerWalletAdjust))

	// Billing exports
	mux.HandleFunc("/api/export/payments.csv", s.requireFullAdmin(s.exportPaymentsCSV))
	mux.HandleFunc("/api/export/wallet-transactions.csv", s.requireFullAdmin(s.exportWalletTransactionsCSV))
	mux.HandleFunc("/api/export/revenue.csv", s.requireFullAdmin(s.exportRevenueCSV))

	// Landing page
	mux.Handle("/", s.landingMetaHandler())

	// Landing content (decoy) admin API
	mux.HandleFunc("/api/admin/landing-content", s.requireFullAdmin(s.handleAdminLandingContent))
	mux.HandleFunc("/api/admin/landing-page/check-blocklist", s.requireFullAdmin(s.adminLandingBlocklistCheck))

	// Xray
	mux.HandleFunc("/api/xray/inbounds", s.requireFullAdmin(s.handleXrayInbound))
	mux.HandleFunc("/api/xray/inbounds/", s.requireFullAdmin(s.handleXrayInboundByID))
	mux.HandleFunc("/api/admin/xray/templates", s.requireFullAdmin(s.handleXrayTemplates))
	mux.HandleFunc("/api/admin/xray/templates/", s.requireFullAdmin(s.handleXrayTemplateByID))
	mux.HandleFunc("/api/portal/xray/subscription", s.requireCustomer(s.handleXraySubscription))
	mux.HandleFunc("/api/portal/xray/links", s.requireCustomer(s.handleXrayLinks))
	mux.HandleFunc("/api/sub/", s.xraySubscription)

	// MTProto
	mux.HandleFunc("/api/admin/mtproto", s.requireFullAdmin(s.handleMTProto))
	mux.HandleFunc("/api/admin/mtproto/", s.requireFullAdmin(s.handleMTProtoByID))

	// AnyConnect
	mux.HandleFunc("/api/admin/anyconnect", s.requireFullAdmin(s.handleAnyConnect))
	mux.HandleFunc("/api/admin/anyconnect/", s.requireFullAdmin(s.handleAnyConnectByID))
	mux.HandleFunc("/api/portal/anyconnect/profile", s.requireCustomer(s.handleAnyConnectProfile))

	// Anti-DPI (routed via node sub-resource, no separate top-level routes needed)

	// Telegram proxies
	mux.HandleFunc("/api/admin/telegram-proxies", s.requireFullAdmin(s.adminTelegramProxies))
	mux.HandleFunc("/api/admin/telegram-proxies/", s.requireFullAdmin(s.adminTelegramProxyByID))
	mux.HandleFunc("/api/admin/telegram-proxies/rotate", s.requireFullAdmin(s.adminTelegramProxiesRotate))
	mux.HandleFunc("/api/customer/telegram-proxies", s.requireCustomer(s.customerTelegramProxies))

	// Knowledge base
	mux.HandleFunc("/api/admin/kb/articles", s.requireFullAdmin(s.handleKBArticles))
	mux.HandleFunc("/api/admin/kb/articles/", s.requireFullAdmin(s.handleKBArticleByID))
	mux.HandleFunc("/api/portal/kb", s.requireCustomer(s.handlePortalKB))
	mux.HandleFunc("/api/portal/kb/search", s.requireCustomer(s.handlePortalKBSearch))
	mux.HandleFunc("/api/portal/kb/", s.requireCustomer(s.handlePortalKBByID))

	// Invoices
	mux.HandleFunc("/api/admin/invoices", s.requireFullAdmin(s.handleInvoices))
	mux.HandleFunc("/api/admin/invoices/", s.requireFullAdmin(s.handleInvoiceByID))
	mux.HandleFunc("/api/portal/invoices", s.requireCustomer(s.handlePortalInvoices))
	mux.HandleFunc("/api/portal/invoices/", s.requireCustomer(s.handlePortalInvoiceByID))

	// SLA
	mux.HandleFunc("/api/admin/sla/config", s.requireFullAdmin(s.handleSLAConfig))
	mux.HandleFunc("/api/admin/sla/stats", s.requireFullAdmin(s.handleSLAStats))

	// Canned responses
	mux.HandleFunc("/api/admin/canned-responses", s.requireFullAdmin(s.adminCannedResponses))
	mux.HandleFunc("/api/admin/canned-responses/", s.requireFullAdmin(s.adminCannedResponses))
	mux.HandleFunc("/api/canned-responses", s.requireAdmin(s.adminCannedResponses))
	mux.HandleFunc("/api/canned-responses/", s.requireAdmin(s.adminCannedResponses))

	// Custom fields
	mux.HandleFunc("/api/admin/custom-fields", s.requireFullAdmin(s.adminCustomFields))
	mux.HandleFunc("/api/admin/custom-fields/", s.requireFullAdmin(s.adminCustomFieldByID))
	mux.HandleFunc("/api/admin/customers/", s.requireFullAdmin(s.adminCustomerSubresource))

	// User tags
	mux.HandleFunc("/api/admin/user-tags", s.requireFullAdmin(s.handleTags))
	mux.HandleFunc("/api/admin/user-tags/", s.requireFullAdmin(s.handleTagByID))
	mux.HandleFunc("/api/tags", s.requireFullAdmin(s.handleTags))
	mux.HandleFunc("/api/tags/", s.requireFullAdmin(s.handleTagByID))
	mux.HandleFunc("/api/filter-presets", s.requireAdmin(s.handleFilterPresets))
	mux.HandleFunc("/api/filter-presets/", s.requireAdmin(s.handleFilterPresetByID))
	mux.HandleFunc("/api/customers/filtered", s.requireAdmin(s.handleCustomersFiltered))

	// LDAP
	mux.HandleFunc("/api/admin/ldap/settings", s.requireFullAdmin(s.adminLDAPSettings))
	mux.HandleFunc("/api/admin/ldap/test", s.requireFullAdmin(s.adminLDAPTest))

	// Segments
	mux.HandleFunc("/api/admin/segments", s.requireFullAdmin(s.adminSegments))
	mux.HandleFunc("/api/admin/segments/", s.requireFullAdmin(s.adminSegmentByID))

	// Public plans (landing page pricing)
	mux.HandleFunc("/api/public-plans", s.publicPlans)
}
