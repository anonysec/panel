package api

import (
	"bytes"
	"database/sql"
	"io/fs"
	"net/http"
	"regexp"
	"sync"
	"time"

	"KorisPanel/panel/internal/antidpi"
	"KorisPanel/panel/internal/auth"
	"KorisPanel/panel/internal/backup"
	"KorisPanel/panel/internal/billing"
	"KorisPanel/panel/internal/cache"
	"KorisPanel/panel/internal/config"
	"KorisPanel/panel/internal/dbstore"
	"KorisPanel/panel/internal/grpcclient"
	"KorisPanel/panel/internal/health"
	"KorisPanel/panel/internal/noderegistry"
	"KorisPanel/panel/internal/notify"
	"KorisPanel/panel/internal/payment"
	"KorisPanel/panel/internal/support"
	"KorisPanel/panel/internal/teleproxy"
	"KorisPanel/panel/internal/tui"
)

type Server struct {
	DB                   *sql.DB
	Config               config.Config
	Auth                 auth.Service
	Notify               *notify.Notifier
	HealthEngine         *health.DiagnosticsEngine
	BackupService        *backup.Service
	Billing              *billing.BillingEngine
	Support              *support.TicketService
	TeleProxy            *teleproxy.ProxyService
	AntiDPI              *antidpi.AntiDPIService
	Cache                *cache.QueryCache
	PaymentRegistry      *payment.Registry
	FirewallMgr          *grpcclient.FirewallManager
	CoreMgr              *grpcclient.CoreManager
	TunnelMgr            *grpcclient.TunnelManager
	CertMgr              *grpcclient.CertManager
	SessionMgr           *grpcclient.SessionManager
	UserSync             *grpcclient.UserSyncService
	TrafficCollector     *grpcclient.TrafficCollector
	GRPCPool             grpcclient.Pool
	GRPCStore            dbstore.Store
	NodeRegistry         noderegistry.Registry
	AdminEmbedFS         fs.FS // Embedded admin frontend (nil = use disk)
	PortalEmbedFS        fs.FS // Embedded portal frontend (nil = use disk)
	LandingEmbedFS       fs.FS // Embedded landing page (nil = use disk)
	LogEntries           func(n int) []tui.LogEntry
	StartedAt            time.Time // Process start time for uptime reporting
	failoverOrchestrator *FailoverOrchestrator
	prevSessionBytes     map[int64]SessionBytes
	sessionMutex         sync.RWMutex
	wsNotifMu            sync.RWMutex
	wsNotifChans         []chan map[string]any
	stmts                PreparedStmts
	landingMetaMu        sync.RWMutex
	landingMetaRendered  string
	landingMetaModTime   time.Time
}

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,64}$`)

var resellerEmojis = []string{"🔵", "🟢", "🟡", "🔴", "🟣", "🟠", "⭐", "💎", "🌙", "🔥", "🌊", "🍀", "🎯", "🦋", "🐬", "🌸", "🎭", "🌈", "⚡", "🎪"}

var defaultEmojis = []string{"🦊", "🐻", "🐼", "🐨", "🦁", "🐯", "🐸", "🐙", "🦋", "🌟", "🔥", "💎", "🎯", "🚀", "⚡", "🌈", "🎪", "🎭", "🏆", "👑"}

func randomEmoji(reserved []string) string {
	pool := make([]string, 0, len(defaultEmojis))
	for _, e := range defaultEmojis {
		excluded := false
		for _, r := range reserved {
			if e == r {
				excluded = true
				break
			}
		}
		if !excluded {
			pool = append(pool, e)
		}
	}
	if len(pool) == 0 {
		pool = defaultEmojis // fallback if all taken
	}
	return pool[time.Now().UnixNano()%int64(len(pool))]
}

// reservedEmojis returns emojis currently assigned to resellers (reserved from general pool).
func (s *Server) reservedEmojis() []string {
	rows, err := s.DB.Query(`SELECT DISTINCT avatar FROM admins WHERE role='reseller' AND avatar IS NOT NULL AND avatar != ''`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var emoji string
		if err := rows.Scan(&emoji); err == nil && emoji != "" {
			result = append(result, emoji)
		}
	}
	return result
}

// reservedEmojisEndpoint returns the list of emojis reserved by resellers.
func (s *Server) reservedEmojisEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	// Only owner/admin can see reserved emojis (used by emoji picker filtering)
	_, role, _ := s.currentAdmin(r)
	if role != "owner" && role != "admin" {
		writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "forbidden"})
		return
	}

	type ReservedEmoji struct {
		Emoji    string `json:"emoji"`
		Reseller string `json:"reseller"`
	}

	rows, err := s.DB.Query(`SELECT COALESCE(avatar,''), username FROM admins WHERE role='reseller' AND avatar IS NOT NULL AND avatar != ''`)
	if err != nil {
		writeJSON(w, map[string]any{"ok": true, "reserved": []any{}})
		return
	}
	defer rows.Close()

	reserved := []ReservedEmoji{}
	for rows.Next() {
		var re ReservedEmoji
		if err := rows.Scan(&re.Emoji, &re.Reseller); err == nil && re.Emoji != "" {
			reserved = append(reserved, re)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "reserved": reserved})
}

func New(db *sql.DB, cfg config.Config) *Server {
	analyzer := health.NewAnalyzer()
	notifier := notify.New()
	return &Server{
		DB:                   db,
		Config:               cfg,
		Auth:                 auth.Service{DB: db},
		Notify:               notifier,
		HealthEngine:         health.NewDiagnosticsEngine(db, analyzer, notifier),
		Cache:                cache.NewQueryCache(500, 60*time.Second),
		StartedAt:            time.Now(),
		failoverOrchestrator: NewFailoverOrchestrator(db, notifier, GetPropagationTimeoutFromDB(db), 10*time.Second),
		prevSessionBytes:     make(map[int64]SessionBytes),
		wsNotifChans:         make([]chan map[string]any, 0),
	}
}

func (s *Server) addWSSubscriber() chan map[string]any {
	ch := make(chan map[string]any, 16)
	s.wsNotifMu.Lock()
	s.wsNotifChans = append(s.wsNotifChans, ch)
	s.wsNotifMu.Unlock()
	return ch
}

func (s *Server) removeWSSubscriber(ch chan map[string]any) {
	s.wsNotifMu.Lock()
	for i, c := range s.wsNotifChans {
		if c == ch {
			s.wsNotifChans = append(s.wsNotifChans[:i], s.wsNotifChans[i+1:]...)
			break
		}
	}
	s.wsNotifMu.Unlock()
	close(ch)
}

func (s *Server) broadcastNotification(notif map[string]any) {
	s.wsNotifMu.RLock()
	defer s.wsNotifMu.RUnlock()
	for _, ch := range s.wsNotifChans {
		select {
		case ch <- notif:
		default:
		}
	}
}

// cachedQuery checks the cache for a key; on miss it calls fetchFn, stores the
// result, and returns it. This encapsulates the cache-aside pattern used by
// read-heavy handlers.
func (s *Server) cachedQuery(key string, fetchFn func() (any, error)) (any, error) {
	if s.Cache == nil {
		return fetchFn()
	}
	if val, ok := s.Cache.Get(key); ok {
		return val, nil
	}
	result, err := fetchFn()
	if err != nil {
		return nil, err
	}
	s.Cache.Set(key, result)
	return result, nil
}

func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/info", s.handleInfo)
	mux.HandleFunc("/internal/status", s.internalStatus)
	mux.HandleFunc("/internal/nodes", s.internalNodes)
	mux.HandleFunc("/internal/users", s.internalUsers)
	mux.HandleFunc("/internal/cleanup", s.internalCleanup)
	mux.HandleFunc("/internal/workers", s.internalWorkers)
	mux.HandleFunc("/internal/logs", s.internalLogs)
	mux.HandleFunc("/internal/update/check", s.internalUpdateCheck)
	mux.HandleFunc("/internal/update/apply", s.internalUpdateApply)
	mux.HandleFunc("/api/setup/status", s.setupStatus)
	mux.HandleFunc("/api/setup/owner", s.setupOwner)
	mux.HandleFunc("/api/auth/admin", s.adminLogin)
	mux.HandleFunc("/api/auth/me", s.adminMe)
	mux.HandleFunc("/api/auth/logout", s.adminLogout)
	mux.HandleFunc("/api/auth/customer", s.customerLogin)
	mux.HandleFunc("/api/auth/customer/logout", s.customerLogout)
	mux.HandleFunc("/api/dashboard/stats", s.requireAdmin(s.dashboardStats))
	mux.HandleFunc("/api/customers", s.requireAdmin(s.customers))
	mux.HandleFunc("/api/customers/bulk", s.requireAdmin(s.customersBulk))
	mux.HandleFunc("/api/admin/customers/bulk", s.requireFullAdmin(s.adminCustomersBulk))
	mux.HandleFunc("/api/customers/export", s.requireAdmin(s.handleCustomerExport))
	mux.HandleFunc("/api/customers/import/preview", s.requireAdmin(s.handleImportPreview))
	mux.HandleFunc("/api/customers/import", s.requireAdmin(s.handleCustomerImport))
	mux.HandleFunc("/api/customers/", s.requireAdmin(s.customerByID))
	mux.HandleFunc("/api/deleted/customers", s.requireAdmin(s.deletedCustomers))
	mux.HandleFunc("/api/plans", s.requireAdmin(s.plans))
	mux.HandleFunc("/api/plans/", s.requireAdmin(s.planByID))
	mux.HandleFunc("/api/nodes", s.requireFullAdmin(s.nodes))
	mux.HandleFunc("/api/admin/nodes/bulk", s.requireFullAdmin(s.nodeBulk))
	mux.HandleFunc("/api/admin/nodes/provision", s.requireFullAdmin(s.nodeProvision))
	mux.HandleFunc("/api/admin/nodes/provision/status", s.requireFullAdmin(s.handleProvisionStatus))
	mux.HandleFunc("/api/admin/nodes/migrate/status", s.requireFullAdmin(s.handleMigrationStatus))
	mux.HandleFunc("/api/admin/nodes/tags", s.requireFullAdmin(s.nodeTagsAll))
	mux.HandleFunc("/api/admin/nodes/", s.requireFullAdmin(s.nodeTagsByID))
	mux.HandleFunc("/api/node/install.sh", s.nodeInstallScript)
	mux.HandleFunc("/api/nodes/", s.requireFullAdmin(s.nodeByID))
	mux.HandleFunc("/api/vpn/settings", s.requireFullAdmin(s.vpnSettings))
	mux.HandleFunc("/api/realtime", s.requireAdmin(s.realtimeWS))
	mux.HandleFunc("/api/portal/me", s.requireCustomer(s.portalMe))
	mux.HandleFunc("/api/portal/impersonate-login", s.portalImpersonateLogin)
	mux.HandleFunc("/api/portal/usage", s.requireCustomer(s.portalUsage))
	mux.HandleFunc("/api/portal/nodes", s.requireCustomer(s.portalNodes))
	mux.HandleFunc("/api/portal/profiles", s.requireCustomer(s.portalProfiles))
	mux.HandleFunc("/api/portal/profiles/", s.requireCustomer(s.portalProfileDownload))
	mux.HandleFunc("/api/portal/plans", s.requireCustomer(s.portalPlans))
	mux.HandleFunc("/api/portal/renew", s.requireCustomer(s.portalRenew))
	mux.HandleFunc("/api/portal/password", s.requireCustomer(s.portalPassword))
	mux.HandleFunc("/api/portal/preferred-node", s.requireCustomer(s.portalPreferredNode))
	mux.HandleFunc("/api/portal/payments", s.requireCustomer(s.portalPayments))
	mux.HandleFunc("/api/portal/payment-methods", s.requireCustomer(s.portalPaymentMethods))
	mux.HandleFunc("/api/portal/wireguard/peers", s.requireCustomer(s.portalWireguardPeers))
	mux.HandleFunc("/api/portal/wireguard/peers/", s.requireCustomer(s.portalWireguardPeerByID))
	mux.HandleFunc("/api/portal/connections", s.requireCustomer(s.handlePortalConnections))
	mux.HandleFunc("/api/portal/configs/", s.requireCustomer(s.handlePortalConfigDownload))

	mux.HandleFunc("/api/node/agent/version", s.agentVersion)
	mux.HandleFunc("/api/node/agent/download", s.agentDownload)
	mux.HandleFunc("/api/audit-logs", s.requireFullAdmin(s.auditLogs))
	mux.HandleFunc("/api/diagnostics", s.requireFullAdmin(s.diagnostics))
	mux.HandleFunc("/api/reserved-emojis", s.requireAdmin(s.reservedEmojisEndpoint))
	mux.HandleFunc("/api/sessions/kill", s.requireFullAdmin(s.killSession))
	mux.HandleFunc("/portal/sub/", s.subscriptionLink)
	mux.HandleFunc("/portal/sub", s.subscriptionLink)
	mux.HandleFunc("/api/nodes/vpn-config/", s.requireFullAdmin(s.nodeVPNConfig))
	mux.HandleFunc("/api/wireguard/peers", s.requireFullAdmin(s.wireguardPeers))
	mux.HandleFunc("/api/wireguard/peers/", s.requireFullAdmin(s.wireguardPeerByID))
	mux.HandleFunc("/api/certificates", s.requireFullAdmin(s.certificates))
	mux.HandleFunc("/api/certificates/", s.requireFullAdmin(s.certificateByID))
	mux.HandleFunc("/api/panel-settings", s.requireAdmin(s.panelSettings))
	mux.HandleFunc("/api/public-settings", s.publicSettings)
	mux.HandleFunc("/api/export/customers.csv", s.requireFullAdmin(s.exportCustomersCSV))
	mux.HandleFunc("/api/export/radacct.csv", s.requireFullAdmin(s.exportRadacctCSV))
	mux.HandleFunc("/api/backup/export", s.requireFullAdmin(s.backupExport))
	mux.HandleFunc("/api/backup/import", s.requireFullAdmin(s.backupImport))
	mux.HandleFunc("/api/events", s.requireFullAdmin(s.events))
	mux.HandleFunc("/api/events/", s.requireFullAdmin(s.eventByID))
	mux.HandleFunc("/api/portal/events", s.requireCustomer(s.portalEvents))
	mux.HandleFunc("/api/portal/events/", s.requireCustomer(s.portalEventByID))
	mux.HandleFunc("/api/portal/warnings", s.requireCustomer(s.portalWarnings))
	mux.HandleFunc("/api/templates", s.requireFullAdmin(s.templates))
	mux.HandleFunc("/api/templates/", s.requireFullAdmin(s.templateByID))
	mux.HandleFunc("/api/settings/data-warning-thresholds", s.requireFullAdmin(s.dataWarningThresholds))
	mux.HandleFunc("/api/settings/warning-config", s.requireFullAdmin(s.warningConfig))
	mux.HandleFunc("/api/portal/app-links", s.portalAppLinks)
	mux.HandleFunc("/api/failover/providers", s.requireFullAdmin(s.failoverProviders))
	mux.HandleFunc("/api/failover/providers/", s.requireFullAdmin(s.failoverProviderByID))
	mux.HandleFunc("/api/failover/domains", s.requireFullAdmin(s.failoverDomains))
	mux.HandleFunc("/api/failover/domains/", s.requireFullAdmin(s.failoverDomainByID))
	mux.HandleFunc("/api/diagnostics/ai", s.requireFullAdmin(s.aiDiagnostics))
	mux.HandleFunc("/api/diagnostics/ai/history", s.requireFullAdmin(s.aiDiagnosticsHistory))
	mux.HandleFunc("/api/diagnostics/ai/rules", s.requireFullAdmin(s.aiHealingRules))
	mux.HandleFunc("/api/diagnostics/ai/rules/", s.requireFullAdmin(s.aiHealingRuleByID))
	mux.HandleFunc("/api/diagnostics/ai/healing-log", s.requireFullAdmin(s.aiHealingLog))
	mux.HandleFunc("/api/diagnostics/logs", s.requireFullAdmin(s.serverLogs))
	mux.HandleFunc("/api/diagnostics/status", s.requireFullAdmin(s.serverStatus))
	mux.HandleFunc("/api/admin/cache-stats", s.requireAdmin(s.cacheStats))
	mux.HandleFunc("/api/admin/backups/restore", s.requireFullAdmin(s.backupRestore))
	mux.HandleFunc("/api/admin/backups/settings", s.requireFullAdmin(s.backupSettings))
	mux.HandleFunc("/api/admin/backups/", s.requireFullAdmin(s.backupByID))
	mux.HandleFunc("/api/admin/backups", s.requireFullAdmin(s.backupRoot))
	mux.HandleFunc("/api/admin/theme", s.requireFullAdmin(s.handleTheme))
	mux.HandleFunc("/api/admin/branding", s.requireFullAdmin(s.brandingPost))
	mux.HandleFunc("/api/admin/update/check", s.requireFullAdmin(s.handleUpdateCheck))
	mux.HandleFunc("/api/admin/update/apply", s.requireFullAdmin(s.handleUpdateApply))
	mux.HandleFunc("/api/admin/update/rollback", s.requireFullAdmin(s.handleUpdateRollback))
	mux.HandleFunc("/api/admin/settings", s.requireFullAdmin(s.handleUpdateSettings))
	mux.HandleFunc("/api/node/update", s.handleNodeUpdate)
	mux.HandleFunc("/api/admin/node-update", s.requireFullAdmin(s.handleAdminNodeUpdate))
	mux.HandleFunc("/api/admin/nodes/update/bulk", s.requireFullAdmin(s.handleNodeBulkAgentUpdate))
	mux.HandleFunc("/api/admin/nodes/update", s.requireFullAdmin(s.handleNodeAgentUpdate))
	mux.HandleFunc("/api/admin/customers/export", s.requireFullAdmin(s.adminCustomersExport))
	mux.HandleFunc("/api/admin/customers/import", s.requireFullAdmin(s.adminCustomersImport))
	mux.HandleFunc("/api/customer/configs/cisco-ipsec", s.requireCustomer(s.customerCiscoIPSecConfig))
	mux.HandleFunc("/api/admin/proxy-configs", s.requireAdmin(s.handleProxyConfigs))
	mux.HandleFunc("/api/admin/nginx/status", s.handleNginxStatusDeprecated)
	mux.HandleFunc("/api/admin/reorder", s.requireFullAdmin(s.adminReorder))
	mux.HandleFunc("/api/node-groups/", s.requireFullAdmin(s.handleNodeGroupByID))
	mux.HandleFunc("/api/node-groups", s.requireFullAdmin(s.handleNodeGroups))
	mux.HandleFunc("/api/portal/node-groups", s.requireCustomer(s.handlePortalNodeGroups))
	mux.HandleFunc("/api/cores", s.requireFullAdmin(s.handleCores))

	// Firewall management via gRPC (knode)
	mux.HandleFunc("/api/admin/nodes/firewall", s.requireFullAdmin(s.handleNodeFirewall))

	// Node management via gRPC (knode connection registry)
	mux.HandleFunc("/api/admin/knode/nodes", s.requireFullAdmin(s.handleKnodeNodes))
	mux.HandleFunc("/api/admin/knode/nodes/", s.requireFullAdmin(s.handleKnodeNodeByID))

	// Settings management (overview, alerts, gRPC, TLS)
	mux.HandleFunc("/api/admin/settings/overview", s.requireFullAdmin(s.handleSettingsOverview))
	mux.HandleFunc("/api/admin/settings/alerts", s.requireFullAdmin(s.handleSettingsAlerts))
	mux.HandleFunc("/api/admin/settings/grpc", s.requireFullAdmin(s.handleSettingsGrpc))
	mux.HandleFunc("/api/admin/settings/tls/upload", s.requireFullAdmin(s.handleSettingsTLSUpload))

	mux.HandleFunc("/dashboard", redirectTo("/dashboard/"))
	mux.Handle("/dashboard/", spaHandler(s.Config.AdminWebDir, "/dashboard/", s.AdminEmbedFS))
	mux.HandleFunc("/portal", redirectTo("/portal/"))
	mux.Handle("/portal/", spaHandler(s.Config.PortalWebDir, "/portal/", s.PortalEmbedFS))

	// Excluded-feature routes (no-op in lite build)
	s.registerExcludedRoutes(mux)

	return mux
}
