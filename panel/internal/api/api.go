package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"KorisPanel/panel/internal/auth"
	"KorisPanel/panel/internal/backup"
	"KorisPanel/panel/internal/config"
	"KorisPanel/panel/internal/health"
	"KorisPanel/panel/internal/notify"
	"KorisPanel/panel/internal/templates"
	"KorisPanel/panel/internal/wireguard"

	"github.com/gorilla/websocket"
)

type Server struct {
	DB               *sql.DB
	Config           config.Config
	Auth             auth.Service
	Notify           *notify.Notifier
	HealthEngine     *health.DiagnosticsEngine
	BackupService    *backup.Service
	prevSessionBytes map[int64]SessionBytes
	sessionMutex     sync.RWMutex
	wsNotifMu        sync.RWMutex
	wsNotifChans     []chan map[string]any
}

type SessionBytes struct {
	InputBytes  int64
	OutputBytes int64
	Timestamp   time.Time
}

type Customer struct {
	ID          int64   `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	Status      string  `json:"status"`
	PlanID      *int64  `json:"plan_id,omitempty"`
	Plan        string  `json:"plan"`
	Credit      float64 `json:"credit"`
	CreatedAt   string  `json:"created_at"`
}

type DeletedCustomer struct {
	Customer
	DeletedAt string `json:"deleted_at"`
}

type RadiusCheck struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Attribute string `json:"attribute"`
	Op        string `json:"op"`
	Value     string `json:"value"`
}

type CustomerDetail struct {
	Customer
	Notes              string                `json:"notes"`
	SubToken           string                `json:"sub_token"`
	RadiusChecks       []RadiusCheck         `json:"radius_checks"`
	RadiusReplies      []RadiusCheck         `json:"radius_replies"`
	Subscription       map[string]any        `json:"subscription,omitempty"`
	Subscriptions      []SubscriptionHistory `json:"subscriptions"`
	WalletTransactions []WalletTransaction   `json:"wallet_transactions"`
}

type Plan struct {
	ID                int64   `json:"id"`
	Name              string  `json:"name"`
	DataGB            float64 `json:"data_gb"`
	SpeedMbps         float64 `json:"speed_mbps"`
	DurationDays      int     `json:"duration_days"`
	Price             float64 `json:"price"`
	BillingType       string  `json:"billing_type"`
	PricePerGB        float64 `json:"price_per_gb"`
	PricePerDay       float64 `json:"price_per_day"`
	DisconnectOnZero  bool    `json:"disconnect_on_zero"`
	AllowPasswordless bool    `json:"allow_passwordless"`
	IsActive          bool    `json:"is_active"`
	SortOrder         int     `json:"sort_order"`
	CreatedAt         string  `json:"created_at"`
}

type NodeUsageSnapshot struct {
	ID          int64  `json:"id"`
	NodeID      int64  `json:"node_id"`
	RxBytes     int64  `json:"rx_bytes"`
	TxBytes     int64  `json:"tx_bytes"`
	OnlineUsers int    `json:"online_users"`
	CreatedAt   string `json:"created_at"`
}

type Node struct {
	ID            int64               `json:"id"`
	Name          string              `json:"name"`
	PublicIP      string              `json:"public_ip"`
	Domain        string              `json:"domain"`
	Status        string              `json:"status"`
	LastSeenAt    string              `json:"last_seen_at"`
	CreatedAt     string              `json:"created_at"`
	ProxyConfig   json.RawMessage     `json:"proxy_config,omitempty"`
	StatusMetrics NodeStatus          `json:"status_metrics"`
	Services      []Service           `json:"services"`
	History       []NodeUsageSnapshot `json:"history,omitempty"`
	Diagnostics   *DiagnosticsReport  `json:"diagnostics,omitempty"`
}

type NodeStatus struct {
	CPUPercent  float64 `json:"cpu_percent"`
	RAMPercent  float64 `json:"ram_percent"`
	DiskPercent float64 `json:"disk_percent"`
	RxBps       int64   `json:"rx_bps"`
	TxBps       int64   `json:"tx_bps"`
	OpenVPN     string  `json:"openvpn_status"`
	L2TP        string  `json:"l2tp_status"`
	IKEv2       string  `json:"ikev2_status"`
	SSH         string  `json:"ssh_status"`
	UpdatedAt   string  `json:"updated_at"`
}

type DiagnosticsReport struct {
	AgentVersion  string `json:"agent_version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	GoVersion     string `json:"go_version"`
	Goroutines    int    `json:"goroutines"`
	MemAllocBytes int64  `json:"mem_alloc_bytes"`
}

type Service struct {
	Service   string `json:"service"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

type NodeTask struct {
	ID          int64           `json:"id"`
	NodeID      int64           `json:"node_id"`
	NodeName    string          `json:"node_name"`
	Action      string          `json:"action"`
	Payload     json.RawMessage `json:"payload_json,omitempty"`
	Status      string          `json:"status"`
	Result      json.RawMessage `json:"result_json,omitempty"`
	Error       string          `json:"error"`
	CreatedBy   string          `json:"created_by"`
	ClaimedAt   string          `json:"claimed_at"`
	CompletedAt string          `json:"completed_at"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

type VPNSettings struct {
	ID                   int    `json:"id"`
	OpenVPNPort          int    `json:"openvpn_port"`
	OpenVPNProtocol      string `json:"openvpn_protocol"`
	OpenVPNNetwork       string `json:"openvpn_network"`
	L2TPNetwork          string `json:"l2tp_network"`
	IKEv2Network         string `json:"ikev2_network"`
	IPSecPSK             string `json:"ipsec_psk"`
	DNS1                 string `json:"dns_1"`
	DNS2                 string `json:"dns_2"`
	UpdatedAt            string `json:"updated_at"`
	OpenVPNServiceStatus string `json:"openvpn_service_status"`
	CAFile               string `json:"ca_file"`
	CAExists             bool   `json:"ca_exists"`
	TLSCryptFile         string `json:"tls_crypt_file"`
	TLSCryptExists       bool   `json:"tls_crypt_exists"`
	RemoteHost           string `json:"remote_host"`
	ActiveNode           string `json:"active_node"`
}

type Payment struct {
	ID          int64   `json:"id"`
	Username    string  `json:"username"`
	Amount      float64 `json:"amount"`
	Method      string  `json:"method"`
	Status      string  `json:"status"`
	IntentType  string  `json:"intent_type"`
	IntentID    *int64  `json:"intent_id,omitempty"`
	IntentLabel string  `json:"intent_label"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type PaymentMethod struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	IsActive     bool   `json:"is_active"`
	SortOrder    int    `json:"sort_order"`
	CreatedAt    string `json:"created_at"`
}

type Ticket struct {
	ID         int64  `json:"id"`
	CustomerID *int64 `json:"customer_id,omitempty"`
	Username   string `json:"username"`
	Subject    string `json:"subject"`
	Status     string `json:"status"`
	Priority   string `json:"priority"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	ClosedAt   string `json:"closed_at"`
}

type TicketMessage struct {
	ID         int64  `json:"id"`
	TicketID   int64  `json:"ticket_id"`
	SenderType string `json:"sender_type"`
	SenderName string `json:"sender_name"`
	Message    string `json:"message"`
	CreatedAt  string `json:"created_at"`
}

type TicketDetail struct {
	Ticket
	Messages []TicketMessage `json:"messages"`
}

type WalletTransaction struct {
	ID            int64   `json:"id"`
	Username      string  `json:"username"`
	Amount        float64 `json:"amount"`
	Type          string  `json:"type"`
	Description   string  `json:"description"`
	Actor         string  `json:"actor"`
	ReferenceType string  `json:"reference_type"`
	ReferenceID   *int64  `json:"reference_id,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

type SubscriptionHistory struct {
	ID           int64   `json:"id"`
	Username     string  `json:"username"`
	Plan         string  `json:"plan"`
	Status       string  `json:"status"`
	StartedAt    string  `json:"started_at"`
	ExpiresAt    string  `json:"expires_at"`
	PaidAmount   float64 `json:"paid_amount"`
	DiscountCode string  `json:"discount_code"`
}

type UsageSession struct {
	ID               int64  `json:"id"`
	Username         string `json:"username"`
	StartTime        string `json:"start_time"`
	UpdateTime       string `json:"update_time"`
	StopTime         string `json:"stop_time"`
	SessionSeconds   int64  `json:"session_seconds"`
	InputBytes       int64  `json:"input_bytes"`
	OutputBytes      int64  `json:"output_bytes"`
	TotalBytes       int64  `json:"total_bytes"`
	FramedIP         string `json:"framed_ip"`
	CallingStationID string `json:"calling_station_id"`
	TerminateCause   string `json:"terminate_cause"`
	Online           bool   `json:"online"`
}

type UsageSummary struct {
	Online             bool           `json:"online"`
	ActiveSessions     int64          `json:"active_sessions"`
	TotalInputBytes    int64          `json:"total_input_bytes"`
	TotalOutputBytes   int64          `json:"total_output_bytes"`
	TotalUsageBytes    int64          `json:"total_usage_bytes"`
	MaxDataBytes       int64          `json:"max_data_bytes"`
	RemainingBytes     *int64         `json:"remaining_bytes,omitempty"`
	LastConnectedAt    string         `json:"last_connected_at"`
	LastDisconnectedAt string         `json:"last_disconnected_at"`
	Sessions           []UsageSession `json:"sessions"`
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,64}$`)

func New(db *sql.DB, cfg config.Config) *Server {
	analyzer := health.NewAnalyzer()
	notifier := notify.New()
	return &Server{
		DB:               db,
		Config:           cfg,
		Auth:             auth.Service{DB: db},
		Notify:           notifier,
		HealthEngine:     health.NewDiagnosticsEngine(db, analyzer, notifier),
		prevSessionBytes: make(map[int64]SessionBytes),
		wsNotifChans:     make([]chan map[string]any, 0),
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

func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", s.health)
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
	mux.HandleFunc("/api/customers/", s.requireAdmin(s.customerByID))
	mux.HandleFunc("/api/deleted/customers", s.requireAdmin(s.deletedCustomers))
	mux.HandleFunc("/api/plans", s.requireAdmin(s.plans))
	mux.HandleFunc("/api/plans/", s.requireAdmin(s.planByID))
	mux.HandleFunc("/api/nodes", s.requireAdmin(s.nodes))
	mux.HandleFunc("/api/nodes/", s.requireAdmin(s.nodeByID))
	mux.HandleFunc("/api/node/tasks", s.requireAdmin(s.nodeTasks))
	mux.HandleFunc("/api/node/tasks/poll", s.nodeTaskPoll)
	mux.HandleFunc("/api/node/tasks/", s.nodeTaskByID)
	mux.HandleFunc("/api/vpn/settings", s.requireAdmin(s.vpnSettings))
	mux.HandleFunc("/api/payment-methods", s.requireAdmin(s.paymentMethods))
	mux.HandleFunc("/api/payment-methods/", s.requireAdmin(s.paymentMethodByID))
	mux.HandleFunc("/api/promo-codes", s.requireAdmin(s.promoCodes))
	mux.HandleFunc("/api/promo-codes/", s.requireAdmin(s.promoCodeByID))
	mux.HandleFunc("/api/portal/apply-promo", s.requireCustomer(s.portalApplyPromo))
	mux.HandleFunc("/api/tickets", s.requireAdmin(s.tickets))
	mux.HandleFunc("/api/tickets/", s.requireAdmin(s.ticketByID))
	mux.HandleFunc("/api/payments", s.requireAdmin(s.payments))
	mux.HandleFunc("/api/payments/", s.requireAdmin(s.paymentByID))
	mux.HandleFunc("/api/wallets/", s.requireAdmin(s.walletByUsername))
	mux.HandleFunc("/api/realtime", s.requireAdmin(s.realtimeWS))
	mux.HandleFunc("/api/portal/me", s.requireCustomer(s.portalMe))
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
	mux.HandleFunc("/api/portal/tickets", s.requireCustomer(s.portalTickets))
	mux.HandleFunc("/api/portal/tickets/", s.requireCustomer(s.portalTicketByID))
	mux.HandleFunc("/api/portal/wireguard/peers", s.requireCustomer(s.portalWireguardPeers))
	mux.HandleFunc("/api/portal/wireguard/peers/", s.requireCustomer(s.portalWireguardPeerByID))
	mux.HandleFunc("/api/node/push", s.nodePush)
	mux.HandleFunc("/api/node/agent/version", s.agentVersion)
	mux.HandleFunc("/api/node/agent/download", s.agentDownload)
	mux.HandleFunc("/api/audit-logs", s.requireAdmin(s.auditLogs))
	mux.HandleFunc("/api/reports/revenue", s.requireAdmin(s.revenueReport))
	mux.HandleFunc("/api/reports/users", s.requireAdmin(s.userReport))
	mux.HandleFunc("/api/reports/bandwidth", s.requireAdmin(s.bandwidthReport))
	mux.HandleFunc("/api/reports/uptime", s.requireAdmin(s.uptimeReport))
	mux.HandleFunc("/api/reports/wallets", s.requireAdmin(s.walletSummary))
	mux.HandleFunc("/api/admin/haproxy/apply", s.requireAdmin(s.haproxyApply))
	mux.HandleFunc("/api/admin/haproxy/status", s.requireAdmin(s.haproxyStatus))
	mux.HandleFunc("/api/diagnostics", s.requireAdmin(s.diagnostics))
	mux.HandleFunc("/api/resellers", s.requireAdmin(s.resellers))
	mux.HandleFunc("/api/resellers/transactions", s.requireAdmin(s.resellerTransactions))
	mux.HandleFunc("/api/resellers/", s.requireAdmin(s.resellerByID))
	mux.HandleFunc("/api/resellers/checkout", s.requireAdmin(s.resellerCheckout))
	mux.HandleFunc("/api/resellers/payments", s.requireAdmin(s.resellerPayments))
	mux.HandleFunc("/api/sessions/kill", s.requireAdmin(s.killSession))
	mux.HandleFunc("/portal/sub", s.subscriptionLink)
	mux.HandleFunc("/api/nodes/vpn-config/", s.requireAdmin(s.nodeVPNConfig))
	mux.HandleFunc("/api/wireguard/peers", s.requireAdmin(s.wireguardPeers))
	mux.HandleFunc("/api/wireguard/peers/", s.requireAdmin(s.wireguardPeerByID))
	mux.HandleFunc("/api/certificates", s.requireAdmin(s.certificates))
	mux.HandleFunc("/api/certificates/", s.requireAdmin(s.certificateByID))
	mux.HandleFunc("/api/panel-settings", s.requireAdmin(s.panelSettings))
	mux.HandleFunc("/api/public-settings", s.publicSettings)
	mux.HandleFunc("/api/export/customers.csv", s.requireAdmin(s.exportCustomersCSV))
	mux.HandleFunc("/api/export/payments.csv", s.requireAdmin(s.exportPaymentsCSV))
	mux.HandleFunc("/api/export/radacct.csv", s.requireAdmin(s.exportRadacctCSV))
	mux.HandleFunc("/api/export/wallet-transactions.csv", s.requireAdmin(s.exportWalletTransactionsCSV))
	mux.HandleFunc("/api/export/revenue.csv", s.requireAdmin(s.exportRevenueCSV))
	mux.HandleFunc("/api/backup/export", s.requireAdmin(s.backupExport))
	mux.HandleFunc("/api/backup/import", s.requireAdmin(s.backupImport))
	mux.HandleFunc("/api/events", s.requireAdmin(s.events))
	mux.HandleFunc("/api/events/", s.requireAdmin(s.eventByID))
	mux.HandleFunc("/api/portal/events", s.requireCustomer(s.portalEvents))
	mux.HandleFunc("/api/portal/events/", s.requireCustomer(s.portalEventByID))
	mux.HandleFunc("/api/portal/warnings", s.requireCustomer(s.portalWarnings))
	mux.HandleFunc("/api/templates", s.requireAdmin(s.templates))
	mux.HandleFunc("/api/templates/", s.requireAdmin(s.templateByID))
	mux.HandleFunc("/api/settings/data-warning-thresholds", s.requireAdmin(s.dataWarningThresholds))
	mux.HandleFunc("/api/settings/warning-config", s.requireAdmin(s.warningConfig))
	mux.HandleFunc("/api/portal/app-links", s.portalAppLinks)
	mux.HandleFunc("/api/failover/providers", s.requireAdmin(s.failoverProviders))
	mux.HandleFunc("/api/failover/providers/", s.requireAdmin(s.failoverProviderByID))
	mux.HandleFunc("/api/failover/domains", s.requireAdmin(s.failoverDomains))
	mux.HandleFunc("/api/failover/domains/", s.requireAdmin(s.failoverDomainByID))
	mux.HandleFunc("/api/diagnostics/ai", s.requireAdmin(s.aiDiagnostics))
	mux.HandleFunc("/api/diagnostics/ai/history", s.requireAdmin(s.aiDiagnosticsHistory))
	mux.HandleFunc("/api/diagnostics/ai/rules", s.requireAdmin(s.aiHealingRules))
	mux.HandleFunc("/api/diagnostics/ai/rules/", s.requireAdmin(s.aiHealingRuleByID))
	mux.HandleFunc("/api/diagnostics/ai/healing-log", s.requireAdmin(s.aiHealingLog))
	mux.HandleFunc("/api/diagnostics/logs", s.requireAdmin(s.serverLogs))
	mux.HandleFunc("/api/diagnostics/status", s.requireAdmin(s.serverStatus))
	mux.HandleFunc("/api/admin/backups/restore", s.requireAdmin(s.backupRestore))
	mux.HandleFunc("/api/admin/backups/settings", s.requireAdmin(s.backupSettings))
	mux.HandleFunc("/api/admin/backups/", s.requireAdmin(s.backupByID))
	mux.HandleFunc("/api/admin/backups", s.requireAdmin(s.backupRoot))

	mux.HandleFunc("/dashboard", redirectTo("/dashboard/"))
	mux.Handle("/dashboard/", spaHandler(s.Config.AdminWebDir, "/dashboard/"))
	mux.HandleFunc("/portal", redirectTo("/portal/"))
	mux.Handle("/portal/", spaHandler(s.Config.PortalWebDir, "/portal/"))
	mux.HandleFunc("/", s.notFound)
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"ok":      true,
		"service": "panel",
		"version": s.Config.Version,
		"time":    time.Now().UTC(),
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	c, err := s.Auth.AdminCount()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{
		"ok":                 true,
		"needs_setup":        c == 0,
		"setup_key_required": s.Config.SetupKey != "",
	})
}

func (s *Server) setupOwner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	limitBody(w, r, maxJSONBody)
	var in struct {
		SetupKey string `json:"setup_key"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if s.Config.SetupKey != "" && in.SetupKey != s.Config.SetupKey {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_setup_key"})
		return
	}
	c, err := s.Auth.AdminCount()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if c > 0 {
		writeJSONCode(w, http.StatusConflict, map[string]any{"ok": false, "error": "already_setup"})
		return
	}
	if err := s.Auth.CreateOwner(in.Username, in.Password); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	auth.SetSession(w, auth.AdminCookieName, in.Username, s.Config.SessionSecret, s.Config.SecureCookies)
	writeJSON(w, map[string]any{"ok": true, "username": in.Username, "role": "owner"})
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	limitBody(w, r, maxJSONBody)
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	ok, err := s.Auth.LoginAdmin(in.Username, in.Password)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid"})
		return
	}
	auth.SetSession(w, auth.AdminCookieName, in.Username, s.Config.SessionSecret, s.Config.SecureCookies)
	role := "admin"
	_ = s.DB.QueryRow(`SELECT role FROM admins WHERE username=? LIMIT 1`, in.Username).Scan(&role)
	writeJSON(w, map[string]any{"ok": true, "username": in.Username, "role": role})
}

func (s *Server) adminMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, role, ok := s.currentAdmin(r)
	credit := 0.00
	if ok {
		_ = s.DB.QueryRow(`SELECT COALESCE(credit, 0) FROM admins WHERE username=?`, username).Scan(&credit)
	}
	writeJSON(w, map[string]any{"ok": true, "authenticated": ok, "username": username, "role": role, "credit": credit})
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	auth.ClearSession(w, auth.AdminCookieName, s.Config.SecureCookies)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) customerLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	limitBody(w, r, maxJSONBody)
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	var pw string
	err := s.DB.QueryRow(`SELECT value FROM radcheck WHERE username=? AND attribute IN('Cleartext-Password','User-Password') ORDER BY id DESC LIMIT 1`, in.Username).Scan(&pw)
	if err != nil {
		// Perform dummy comparison to prevent timing-based user enumeration
		subtle.ConstantTimeCompare([]byte("dummy-value-padding"), []byte(in.Password))
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(pw), []byte(in.Password)) != 1 {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid"})
		return
	}
	_, _ = s.DB.Exec(`INSERT IGNORE INTO customers(username,sub_token) VALUES(?,?)`, in.Username, auth.RandomToken(24))
	_, _ = s.DB.Exec(`INSERT IGNORE INTO wallets(username,credit) VALUES(?,0)`, in.Username)
	auth.SetSession(w, auth.CustomerCookieName, in.Username, s.Config.SessionSecret, s.Config.SecureCookies)
	writeJSON(w, map[string]any{"ok": true, "username": in.Username})
}

func (s *Server) customerLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	auth.ClearSession(w, auth.CustomerCookieName, s.Config.SecureCookies)
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) dashboardStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.dashboardStatsPayload())
}

func (s *Server) dashboardStatsPayload() map[string]any {
	var rx, tx float64
	_ = s.DB.QueryRow(`SELECT COALESCE(SUM(ns.rx_bps),0), COALESCE(SUM(ns.tx_bps),0) FROM node_status ns JOIN nodes n ON n.id=ns.node_id WHERE n.status <> 'disabled' AND ns.updated_at >= NOW() - INTERVAL 5 MINUTE`).Scan(&rx, &tx)

	// Total data usage from radacct (all sessions, including closed ones)
	var totalInput, totalOutput int64
	_ = s.DB.QueryRow(`SELECT COALESCE(SUM(acctinputoctets),0), COALESCE(SUM(acctoutputoctets),0) FROM radacct`).Scan(&totalInput, &totalOutput)

	// Today's data usage
	var todayInput, todayOutput int64
	_ = s.DB.QueryRow(`SELECT COALESCE(SUM(acctinputoctets),0), COALESCE(SUM(acctoutputoctets),0) FROM radacct WHERE acctstarttime >= CURDATE()`).Scan(&todayInput, &todayOutput)

	return map[string]any{
		"ok":                 true,
		"customers":          s.count(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL`),
		"active_customers":   s.count(`SELECT COUNT(*) FROM customers WHERE deleted_at IS NULL AND status='active'`),
		"plans":              s.count(`SELECT COUNT(*) FROM plans WHERE is_active=1`),
		"nodes":              s.count(`SELECT COUNT(*) FROM nodes WHERE status IN('online','stale')`),
		"online_users":       s.count(`SELECT COUNT(DISTINCT username) FROM radacct WHERE acctstoptime IS NULL`),
		"active_sessions":    s.count(`SELECT COUNT(*) FROM radacct WHERE acctstoptime IS NULL`),
		"open_tickets":       s.count(`SELECT COUNT(*) FROM tickets WHERE deleted_at IS NULL AND status='open'`),
		"pending_payments":   s.count(`SELECT COUNT(*) FROM payments WHERE status='pending'`),
		"approved_payments":  s.sum(`SELECT COALESCE(SUM(amount),0) FROM payments WHERE status='approved'`),
		"unseen_events":      s.count(`SELECT COUNT(*) FROM events WHERE seen=0`),
		"total_rx_bps":       rx,
		"total_tx_bps":       tx,
		"total_input_bytes":  totalInput,
		"total_output_bytes": totalOutput,
		"today_input_bytes":  todayInput,
		"today_output_bytes": todayOutput,
	}
}

func (s *Server) customers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listCustomers(w, r)
	case http.MethodPost:
		s.createCustomer(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listCustomers(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	where := "c.deleted_at IS NULL"
	args := []any{}
	if q != "" {
		where += " AND (c.username LIKE ? OR c.display_name LIKE ?)"
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	query := fmt.Sprintf(`SELECT c.id,c.username,COALESCE(c.display_name,''),c.status,c.plan_id,COALESCE(p.name,''),COALESCE(w.credit,0),c.created_at
		FROM customers c
		LEFT JOIN plans p ON p.id=c.plan_id
		LEFT JOIN wallets w ON w.username=c.username
		WHERE %s
		ORDER BY c.id DESC LIMIT 500`, where)
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	out := []Customer{}
	for rows.Next() {
		var c Customer
		var planID sql.NullInt64
		var created sql.NullTime
		if err := rows.Scan(&c.ID, &c.Username, &c.DisplayName, &c.Status, &planID, &c.Plan, &c.Credit, &created); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if planID.Valid {
			c.PlanID = &planID.Int64
		}
		if created.Valid {
			c.CreatedAt = created.Time.Format(time.RFC3339)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "customers": out})
}

func (s *Server) deletedCustomers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`SELECT c.id,c.username,COALESCE(c.display_name,''),c.status,c.plan_id,COALESCE(p.name,''),COALESCE(w.credit,0),c.created_at,c.deleted_at
		FROM customers c
		LEFT JOIN plans p ON p.id=c.plan_id
		LEFT JOIN wallets w ON w.username=c.username
		WHERE c.deleted_at IS NOT NULL
		ORDER BY c.deleted_at DESC LIMIT 500`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	out := []DeletedCustomer{}
	for rows.Next() {
		var c DeletedCustomer
		var planID sql.NullInt64
		var created, deleted sql.NullTime
		if err := rows.Scan(&c.ID, &c.Username, &c.DisplayName, &c.Status, &planID, &c.Plan, &c.Credit, &created, &deleted); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if planID.Valid {
			c.PlanID = &planID.Int64
		}
		if created.Valid {
			c.CreatedAt = created.Time.Format(time.RFC3339)
		}
		if deleted.Valid {
			c.DeletedAt = deleted.Time.Format(time.RFC3339)
		}
		out = append(out, c)
	}
	writeJSON(w, map[string]any{"ok": true, "customers": out})
}

func (s *Server) createCustomer(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r, maxJSONBody)
	var in struct {
		Username          string   `json:"username"`
		Password          string   `json:"password"`
		DisplayName       string   `json:"display_name"`
		PlanID            *int64   `json:"plan_id"`
		DataGB            *float64 `json:"data_gb"`
		SpeedMbps         *float64 `json:"speed_mbps"`
		Days              *int     `json:"days"`
		IPLimit           *int     `json:"ip_limit"`
		ActivateOnConnect bool     `json:"activate_on_connect"`
		TemplateID        *int64   `json:"template_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if !usernamePattern.MatchString(in.Username) || len(in.Password) < 4 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "username_password_required"})
		return
	}

	// Template pre-population: load template and use its values as defaults
	var templateRadiusChecks []radiusAttr
	var templateRadiusReplies []radiusAttr
	if in.TemplateID != nil {
		var tmpl UserTemplate
		row := s.DB.QueryRow(`SELECT id, name, plan_id, status, connection_limit, radius_checks, radius_replies, created_by, deleted_at, created_at, updated_at FROM user_templates WHERE id = ?`, *in.TemplateID)
		var err error
		tmpl, err = scanTemplate(row)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid template_id: template not found")
			return
		}
		if tmpl.DeletedAt != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid template_id: template has been deleted")
			return
		}
		// Pre-populate plan_id from template if not explicitly provided
		if in.PlanID == nil && tmpl.PlanID != nil {
			in.PlanID = tmpl.PlanID
		}
		// Pre-populate connection limit from template if not explicitly provided
		if in.IPLimit == nil && tmpl.ConnectionLimit > 0 {
			in.IPLimit = &tmpl.ConnectionLimit
		}
		// Parse RADIUS check attributes from template
		if len(tmpl.RadiusChecks) > 0 && string(tmpl.RadiusChecks) != "null" {
			if err := json.Unmarshal(tmpl.RadiusChecks, &templateRadiusChecks); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "failed to parse template radius_checks")
				return
			}
		}
		// Parse RADIUS reply attributes from template
		if len(tmpl.RadiusReplies) > 0 && string(tmpl.RadiusReplies) != "null" {
			if err := json.Unmarshal(tmpl.RadiusReplies, &templateRadiusReplies); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "failed to parse template radius_replies")
				return
			}
		}
	}

	if in.PlanID != nil && *in.PlanID == 0 {
		in.PlanID = nil
	}
	dataGB := 0.0
	speedMbps := 0.0
	days := 0
	if in.DataGB != nil {
		dataGB = *in.DataGB
	}
	if in.SpeedMbps != nil {
		speedMbps = *in.SpeedMbps
	}
	if in.Days != nil {
		days = *in.Days
	}
	if dataGB < 0 || speedMbps < 0 || days < 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_limits"})
		return
	}
	actor, role, ok := s.currentAdmin(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	if in.PlanID != nil {
		var planDataGB, planSpeedMbps float64
		var planDays int
		var planPrice float64
		if err := s.DB.QueryRow(`SELECT data_gb,speed_mbps,duration_days,price FROM plans WHERE id=? AND is_active=1 LIMIT 1`, *in.PlanID).Scan(&planDataGB, &planSpeedMbps, &planDays, &planPrice); err == nil {
			if in.DataGB == nil {
				dataGB = planDataGB
			}
			if in.SpeedMbps == nil {
				speedMbps = planSpeedMbps
			}
			if in.Days == nil {
				days = planDays
			}
			if role == "reseller" && planPrice > 0 {
				var resellerCredit float64
				_ = s.DB.QueryRow(`SELECT credit FROM admins WHERE username=?`, actor).Scan(&resellerCredit)
				if resellerCredit < planPrice {
					writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "insufficient_reseller_credit", "credit": resellerCredit, "required": planPrice})
					return
				}
				_, err := s.DB.Exec(`UPDATE admins SET credit = credit - ? WHERE username=?`, planPrice, actor)
				if err != nil {
					writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
					return
				}
				_, _ = s.DB.Exec(`INSERT INTO reseller_transactions(reseller_username, amount, type, description, actor) VALUES(?,?, 'deduction', ?, ?)`, actor, -planPrice, "Created customer "+in.Username, actor)
			}
		}
	}

	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO customers(username,display_name,plan_id,sub_token,created_by) VALUES(?,?,?,?,?)`, in.Username, in.DisplayName, in.PlanID, auth.RandomToken(24), actor)
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	customerID, _ := res.LastInsertId()
	if _, err = tx.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES(?,?,0)`, customerID, in.Username); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err = tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES(?,'Cleartext-Password',':=',?)`, in.Username, in.Password); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err = tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES(?,'Simultaneous-Use',':=',?)`, in.Username, strconv.Itoa(func() int {
		if in.IPLimit != nil && *in.IPLimit > 0 {
			return *in.IPLimit
		}
		return 1
	}())); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = tx.Exec(`DELETE FROM radcheck WHERE username=? AND attribute='Max-Data'`, in.Username)
	if dataGB > 0 {
		bytes := int64(math.Round(dataGB * 1024 * 1024 * 1024))
		if _, err = tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES(?,'Max-Data',':=',?)`, in.Username, bytes); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	_, _ = tx.Exec(`DELETE FROM radreply WHERE username=? AND attribute='Mikrotik-Rate-Limit'`, in.Username)
	if speedMbps > 0 {
		if _, err = tx.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES(?,'Mikrotik-Rate-Limit',':=',?)`, in.Username, speedLimitValue(speedMbps)); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	// Insert template RADIUS check attributes (skip attributes already set by explicit fields)
	for _, attr := range templateRadiusChecks {
		if attr.Attribute == "Cleartext-Password" || attr.Attribute == "Simultaneous-Use" || attr.Attribute == "Max-Data" {
			continue // These are managed by explicit fields above
		}
		if _, err = tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES(?,?,?,?)`, in.Username, attr.Attribute, attr.Op, attr.Value); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	// Insert template RADIUS reply attributes (skip attributes already set by explicit fields)
	for _, attr := range templateRadiusReplies {
		if attr.Attribute == "Mikrotik-Rate-Limit" {
			continue // Managed by speed_mbps field above
		}
		if _, err = tx.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES(?,?,?,?)`, in.Username, attr.Attribute, attr.Op, attr.Value); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if days > 0 {
		if in.ActivateOnConnect {
			// First-connection activation: don't set expires_at yet; auth script will set it on first VPN connect
			if _, err = tx.Exec(`INSERT INTO subscriptions(customer_id,username,plan_id,activate_on_connect) VALUES(?,?,?,1)`, customerID, in.Username, in.PlanID); err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		} else {
			expires := time.Now().AddDate(0, 0, days)
			if _, err = tx.Exec(`INSERT INTO subscriptions(customer_id,username,plan_id,expires_at) VALUES(?,?,?,?)`, customerID, in.Username, in.PlanID, expires); err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		}
	}
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Auto-provision WireGuard peer on WireGuard-enabled nodes
	if in.PlanID != nil && *in.PlanID > 0 {
		s.autoProvisionWireGuardPeer(customerID)
	}
	actor, _, _ = s.currentAdmin(r)
	s.logAudit(actor, "customer.created", "customer", strconv.FormatInt(customerID, 10), nil, map[string]any{"username": in.Username}, clientIP(r))
	s.createEvent("customer", "info", fmt.Sprintf("Customer created: %s", in.Username), fmt.Sprintf("Admin %s created customer %s", actor, in.Username), actor, in.Username)
	writeJSON(w, map[string]any{"ok": true, "id": customerID})
}

func (s *Server) customerByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/customers/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodGet:
			s.getCustomerDetail(w, r, id)
		case http.MethodPatch:
			s.updateCustomer(w, r, id)
		case http.MethodDelete:
			s.archiveCustomer(w, r, id)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
		return
	}
	if action == "usage" {
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.getCustomerUsage(w, id)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "enable":
		s.setCustomerStatus(w, id, "active")
	case "disable":
		s.setCustomerStatus(w, id, "disabled")
	case "reset-password":
		s.resetCustomerPassword(w, r, id)
	case "reset-traffic":
		s.resetCustomerTraffic(w, r, id)
	case "renew":
		s.renewCustomer(w, r, id)
	case "restore":
		s.restoreCustomer(w, r, id)
	case "connection-limit":
		s.setConnectionLimit(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) archiveCustomer(w http.ResponseWriter, r *http.Request, id int64) {
	actor, _, _ := s.currentAdmin(r)
	var username string
	var deletedAt sql.NullTime
	if err := s.DB.QueryRow(`SELECT username,deleted_at FROM customers WHERE id=? LIMIT 1`, id).Scan(&username, &deletedAt); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if deletedAt.Valid {
		writeJSON(w, map[string]any{"ok": true, "already_deleted": true})
		return
	}
	radChecks, _ := s.radiusRows("radcheck", username)
	radReplies, _ := s.radiusRows("radreply", username)
	payloadBytes, _ := json.Marshal(map[string]any{
		"customer_id": id,
		"username":    username,
		"radcheck":    radChecks,
		"radreply":    radReplies,
		"archived_at": time.Now().UTC(),
	})
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO deleted_archive(type,name,archive_key,payload,created_by) VALUES('customer',?,?,?,?)`, username, strconv.FormatInt(id, 10), string(payloadBytes), actor); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if _, err := tx.Exec(`UPDATE customers SET status='deleted',deleted_at=NOW() WHERE id=? AND deleted_at IS NULL`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = tx.Exec(`DELETE FROM radcheck WHERE username=?`, username)
	_, _ = tx.Exec(`DELETE FROM radreply WHERE username=?`, username)
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.logAudit(actor, "customer.archived", "customer", strconv.FormatInt(id, 10), nil, map[string]any{"username": username}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) restoreCustomer(w http.ResponseWriter, r *http.Request, id int64) {
	var username string
	if err := s.DB.QueryRow(`SELECT username FROM customers WHERE id=? LIMIT 1`, id).Scan(&username); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	var archiveID int64
	var payload string
	_ = s.DB.QueryRow(`SELECT id,COALESCE(payload,'') FROM deleted_archive WHERE type='customer' AND archive_key=? ORDER BY id DESC LIMIT 1`, strconv.FormatInt(id, 10)).Scan(&archiveID, &payload)
	var archived struct {
		RadCheck []RadiusCheck `json:"radcheck"`
		RadReply []RadiusCheck `json:"radreply"`
	}
	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &archived)
	}
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE customers SET status='active',deleted_at=NULL WHERE id=?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = tx.Exec(`DELETE FROM radcheck WHERE username=?`, username)
	_, _ = tx.Exec(`DELETE FROM radreply WHERE username=?`, username)
	for _, row := range archived.RadCheck {
		_, _ = tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES(?,?,?,?)`, username, row.Attribute, row.Op, row.Value)
	}
	for _, row := range archived.RadReply {
		_, _ = tx.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES(?,?,?,?)`, username, row.Attribute, row.Op, row.Value)
	}
	if archiveID > 0 {
		_, _ = tx.Exec(`UPDATE deleted_archive SET restored_at=NOW() WHERE id=?`, archiveID)
	}
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "customer.restored", "customer", strconv.FormatInt(id, 10), nil, map[string]any{"username": username}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) getCustomerDetail(w http.ResponseWriter, r *http.Request, id int64) {
	var c CustomerDetail
	var planID sql.NullInt64
	var created sql.NullTime
	err := s.DB.QueryRow(`SELECT c.id,c.username,COALESCE(c.display_name,''),c.status,c.plan_id,COALESCE(p.name,''),COALESCE(w.credit,0),c.created_at,COALESCE(c.notes,''),COALESCE(c.sub_token,'')
		FROM customers c
		LEFT JOIN plans p ON p.id=c.plan_id
		LEFT JOIN wallets w ON w.username=c.username
		WHERE c.id=? AND c.deleted_at IS NULL LIMIT 1`, id).Scan(&c.ID, &c.Username, &c.DisplayName, &c.Status, &planID, &c.Plan, &c.Credit, &created, &c.Notes, &c.SubToken)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if planID.Valid {
		c.PlanID = &planID.Int64
	}
	if created.Valid {
		c.CreatedAt = created.Time.Format(time.RFC3339)
	}

	rows, err := s.DB.Query(`SELECT id,username,attribute,op,value FROM radcheck WHERE username=? ORDER BY id ASC`, c.Username)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var rc RadiusCheck
			if err := rows.Scan(&rc.ID, &rc.Username, &rc.Attribute, &rc.Op, &rc.Value); err == nil {
				if strings.Contains(strings.ToLower(rc.Attribute), "password") {
					rc.Value = "••••••••"
				}
				c.RadiusChecks = append(c.RadiusChecks, rc)
			}
		}
	}
	replyRows, err := s.DB.Query(`SELECT id,username,attribute,op,value FROM radreply WHERE username=? ORDER BY id ASC`, c.Username)
	if err == nil {
		defer replyRows.Close()
		for replyRows.Next() {
			var rr RadiusCheck
			if err := replyRows.Scan(&rr.ID, &rr.Username, &rr.Attribute, &rr.Op, &rr.Value); err == nil {
				c.RadiusReplies = append(c.RadiusReplies, rr)
			}
		}
	}

	var subID int64
	var subPlan, subStatus string
	var started, expires sql.NullTime
	if err := s.DB.QueryRow(`SELECT s.id,COALESCE(p.name,''),s.status,s.started_at,s.expires_at
		FROM subscriptions s
		LEFT JOIN plans p ON p.id=s.plan_id
		WHERE s.username=? ORDER BY s.id DESC LIMIT 1`, c.Username).Scan(&subID, &subPlan, &subStatus, &started, &expires); err == nil {
		sub := map[string]any{"id": subID, "plan": subPlan, "status": subStatus}
		if started.Valid {
			sub["started_at"] = started.Time.Format(time.RFC3339)
		}
		if expires.Valid {
			sub["expires_at"] = expires.Time.Format(time.RFC3339)
		}
		c.Subscription = sub
	}

	subRows, err := s.DB.Query(`SELECT s.id,s.username,COALESCE(p.name,''),s.status,s.started_at,s.expires_at,s.paid_amount,COALESCE(s.discount_code,'')
		FROM subscriptions s
		LEFT JOIN plans p ON p.id=s.plan_id
		WHERE s.username=? ORDER BY s.id DESC LIMIT 100`, c.Username)
	if err == nil {
		defer subRows.Close()
		for subRows.Next() {
			var item SubscriptionHistory
			var startedAt, expiresAt sql.NullTime
			if err := subRows.Scan(&item.ID, &item.Username, &item.Plan, &item.Status, &startedAt, &expiresAt, &item.PaidAmount, &item.DiscountCode); err == nil {
				if startedAt.Valid {
					item.StartedAt = startedAt.Time.Format(time.RFC3339)
				}
				if expiresAt.Valid {
					item.ExpiresAt = expiresAt.Time.Format(time.RFC3339)
				}
				c.Subscriptions = append(c.Subscriptions, item)
			}
		}
	}

	txRows, err := s.DB.Query(`SELECT id,username,amount,type,description,actor,COALESCE(reference_type,''),reference_id,created_at FROM wallet_transactions WHERE username=? ORDER BY id DESC LIMIT 100`, c.Username)
	if err == nil {
		defer txRows.Close()
		for txRows.Next() {
			var item WalletTransaction
			var refID sql.NullInt64
			var createdAt sql.NullTime
			if err := txRows.Scan(&item.ID, &item.Username, &item.Amount, &item.Type, &item.Description, &item.Actor, &item.ReferenceType, &refID, &createdAt); err == nil {
				if refID.Valid {
					item.ReferenceID = &refID.Int64
				}
				if createdAt.Valid {
					item.CreatedAt = createdAt.Time.Format(time.RFC3339)
				}
				c.WalletTransactions = append(c.WalletTransactions, item)
			}
		}
	}

	writeJSON(w, map[string]any{"ok": true, "customer": c})
}

func (s *Server) updateCustomer(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		DisplayName *string  `json:"display_name"`
		Status      *string  `json:"status"`
		PlanID      *int64   `json:"plan_id"`
		Notes       *string  `json:"notes"`
		DataGB      *float64 `json:"data_gb"`
		SpeedMbps   *float64 `json:"speed_mbps"`
		Days        *int     `json:"days"`
		IPLimit     *int     `json:"ip_limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	username, err := s.customerUsername(id)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	sets := []string{}
	args := []any{}
	if in.DisplayName != nil {
		displayName := strings.TrimSpace(*in.DisplayName)
		sets = append(sets, "display_name=?")
		args = append(args, displayName)
	}
	if in.Status != nil {
		status := strings.TrimSpace(*in.Status)
		if !validCustomerStatus(status) {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_status"})
			return
		}
		sets = append(sets, "status=?")
		args = append(args, status)
	}
	if in.PlanID != nil {
		sets = append(sets, "plan_id=?")
		if *in.PlanID == 0 {
			args = append(args, nil)
		} else {
			args = append(args, *in.PlanID)
		}
	}
	if in.Notes != nil {
		sets = append(sets, "notes=?")
		args = append(args, strings.TrimSpace(*in.Notes))
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := s.DB.Exec(`UPDATE customers SET `+strings.Join(sets, ",")+` WHERE id=? AND deleted_at IS NULL`, args...); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if in.DataGB != nil {
		if *in.DataGB < 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_limits"})
			return
		}
		if *in.DataGB == 0 {
			_, _ = s.DB.Exec(`DELETE FROM radcheck WHERE username=? AND attribute='Max-Data'`, username)
		} else {
			bytes := int64(math.Round(*in.DataGB * 1024 * 1024 * 1024))
			if err := s.upsertRadCheck(username, "Max-Data", strconv.FormatInt(bytes, 10)); err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		}
	}
	if in.SpeedMbps != nil {
		if *in.SpeedMbps < 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_limits"})
			return
		}
		if *in.SpeedMbps == 0 {
			_, _ = s.DB.Exec(`DELETE FROM radreply WHERE username=? AND attribute='Mikrotik-Rate-Limit'`, username)
		} else if err := s.upsertRadReply(username, "Mikrotik-Rate-Limit", speedLimitValue(*in.SpeedMbps)); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if in.Days != nil && *in.Days > 0 {
		var planID any
		if in.PlanID != nil && *in.PlanID > 0 {
			planID = *in.PlanID
		}
		expires := time.Now().AddDate(0, 0, *in.Days)
		_, _ = s.DB.Exec(`INSERT INTO subscriptions(customer_id,username,plan_id,expires_at) VALUES(?,?,?,?)`, id, username, planID, expires)
	}
	if in.IPLimit != nil {
		if *in.IPLimit <= 0 {
			_, _ = s.DB.Exec(`DELETE FROM radcheck WHERE username=? AND attribute='Simultaneous-Use'`, username)
		} else {
			_ = s.upsertRadCheck(username, "Simultaneous-Use", strconv.Itoa(*in.IPLimit))
		}
	}

	if in.Status != nil && *in.Status != "active" {
		// Only disconnect if status is actually changing (avoid unnecessary disconnection
		// when updating other fields on an already-disabled/limited customer)
		var currentStatus string
		_ = s.DB.QueryRow(`SELECT status FROM customers WHERE id=? LIMIT 1`, id).Scan(&currentStatus)
		if currentStatus != *in.Status {
			s.disconnectCustomerSessions(username)
		}
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "customer.updated", "customer", strconv.FormatInt(id, 10), nil, map[string]any{"username": username}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) setCustomerStatus(w http.ResponseWriter, id int64, status string) {
	username, err := s.customerUsername(id)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if _, err := s.DB.Exec(`UPDATE customers SET status=? WHERE id=? AND deleted_at IS NULL`, status, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if status != "active" {
		s.disconnectCustomerSessions(username)
	}

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) resetCustomerPassword(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if len(in.Password) < 4 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "password_too_short"})
		return
	}
	username, err := s.customerUsername(id)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	res, err := s.DB.Exec(`UPDATE radcheck SET value=? WHERE username=? AND attribute IN('Cleartext-Password','User-Password')`, in.Password, username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		_, err = s.DB.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES(?,'Cleartext-Password',':=',?)`, username, in.Password)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) resetCustomerTraffic(w http.ResponseWriter, r *http.Request, id int64) {
	username, err := s.customerUsername(id)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Reset traffic by archiving old radacct records (set stop time) so usage counters restart
	result, err := s.DB.Exec(`UPDATE radacct SET acctstoptime=COALESCE(acctstoptime, NOW()), acctterminatecause=COALESCE(acctterminatecause, 'Admin-Reset') WHERE username=?`, username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	affected, _ := result.RowsAffected()

	// If customer was in 'limited' status, re-enable them
	_, _ = s.DB.Exec(`UPDATE customers SET status='active' WHERE username=? AND status='limited' AND deleted_at IS NULL`, username)

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "customer.traffic_reset", "customer", strconv.FormatInt(id, 10), nil, map[string]any{"username": username, "sessions_reset": affected}, clientIP(r))
	s.createEvent("customer", "info", fmt.Sprintf("Traffic reset: %s", username), fmt.Sprintf("Admin %s reset traffic counters for %s (%d sessions archived)", actor, username, affected), actor, username)

	writeJSON(w, map[string]any{"ok": true, "sessions_reset": affected})
}

func (s *Server) renewCustomer(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		PlanID int64 `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.PlanID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "plan_required"})
		return
	}

	var username string
	if err := s.DB.QueryRow(`SELECT username FROM customers WHERE id=? AND deleted_at IS NULL LIMIT 1`, id).Scan(&username); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	var plan Plan
	var active int
	if err := s.DB.QueryRow(`SELECT id,name,data_gb,speed_mbps,duration_days,price,is_active,sort_order,created_at FROM plans WHERE id=? LIMIT 1`, in.PlanID).Scan(&plan.ID, &plan.Name, &plan.DataGB, &plan.SpeedMbps, &plan.DurationDays, &plan.Price, &active, &plan.SortOrder, new(sql.NullTime)); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "plan_not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	plan.IsActive = active == 1
	if active != 1 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "plan_inactive"})
		return
	}

	actor, role, ok := s.currentAdmin(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	if role == "reseller" {
		var resellerCredit float64
		if err := tx.QueryRow(`SELECT COALESCE(credit,0) FROM admins WHERE username=? FOR UPDATE`, actor).Scan(&resellerCredit); err != nil && err != sql.ErrNoRows {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if plan.Price > 0 && resellerCredit < plan.Price {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "insufficient_reseller_credit", "credit": resellerCredit, "required": plan.Price})
			return
		}
	} else {
		var walletCredit float64
		if err := tx.QueryRow(`SELECT COALESCE(credit,0) FROM wallets WHERE username=? FOR UPDATE`, username).Scan(&walletCredit); err != nil && err != sql.ErrNoRows {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if plan.Price > 0 && walletCredit+0.0001 < plan.Price {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "insufficient_wallet", "wallet": walletCredit, "required": plan.Price})
			return
		}
	}

	if _, err := tx.Exec(`UPDATE customers SET plan_id=?,status='active' WHERE id=? AND deleted_at IS NULL`, plan.ID, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = tx.Exec(`DELETE FROM radcheck WHERE username=? AND attribute='Max-Data'`, username)
	if plan.DataGB > 0 {
		bytes := int64(math.Round(plan.DataGB * 1024 * 1024 * 1024))
		if _, err := tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES(?,'Max-Data',':=',?)`, username, bytes); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	_, _ = tx.Exec(`DELETE FROM radreply WHERE username=? AND attribute='Mikrotik-Rate-Limit'`, username)
	if plan.SpeedMbps > 0 {
		if _, err := tx.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES(?,'Mikrotik-Rate-Limit',':=',?)`, username, speedLimitValue(plan.SpeedMbps)); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}

	var expires any
	if plan.DurationDays > 0 {
		expires = time.Now().AddDate(0, 0, plan.DurationDays)
	}
	if _, err := tx.Exec(`INSERT INTO subscriptions(customer_id,username,plan_id,expires_at,paid_amount) VALUES(?,?,?,?,?)`, id, username, plan.ID, expires, plan.Price); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if plan.Price > 0 {
		desc := "plan activated: " + plan.Name
		if role == "reseller" {
			_, err = tx.Exec(`UPDATE admins SET credit = credit - ? WHERE username=?`, plan.Price, actor)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			_, _ = tx.Exec(`INSERT INTO reseller_transactions(reseller_username, amount, type, description, actor) VALUES(?,?, 'deduction', ?, ?)`, actor, -plan.Price, "Renewed plan for "+username, actor)
			paymentRes, err := tx.Exec(`INSERT INTO payments(customer_id,username,amount,method,status,admin_note) VALUES(?,?,?,'reseller','approved',?)`, id, username, plan.Price, desc+" (reseller: "+actor+")")
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			paymentID, _ := paymentRes.LastInsertId()
			if _, err := tx.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES(?,?,?,?,?,?,?,?)`, id, username, 0.0, "purchase", desc+" (reseller paid)", actor, "payment", paymentID); err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		} else {
			paymentRes, err := tx.Exec(`INSERT INTO payments(customer_id,username,amount,method,status,admin_note) VALUES(?,?,?,'wallet','approved',?)`, id, username, plan.Price, desc)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			paymentID, _ := paymentRes.LastInsertId()
			_, err = tx.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES(?,?,?) ON DUPLICATE KEY UPDATE credit=credit+VALUES(credit), customer_id=COALESCE(VALUES(customer_id),customer_id)`, id, username, -plan.Price)
			if err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			if _, err := tx.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES(?,?,?,?,?,?,?,?)`, id, username, -plan.Price, "purchase", desc, "admin", "payment", paymentID); err != nil {
				writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Auto-provision WireGuard peer on subscription renewal/activation
	s.autoProvisionWireGuardPeer(id)
	actor, _, _ = s.currentAdmin(r)
	s.createEvent("plan", "info", fmt.Sprintf("Plan applied: %s", plan.Name), fmt.Sprintf("Admin %s applied plan %s to %s", actor, plan.Name, username), actor, username)
	writeJSON(w, map[string]any{"ok": true, "plan": plan, "wallet_deducted": plan.Price})
}

func (s *Server) plans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPlans(w, r)
	case http.MethodPost:
		s.createPlan(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) planByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/plans/")
	if !ok || action != "" {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getPlan(w, id)
	case http.MethodPatch:
		s.updatePlan(w, r, id)
	case http.MethodDelete:
		s.archivePlan(w, r, id)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listPlans(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(`SELECT id,name,data_gb,speed_mbps,duration_days,price,billing_type,price_per_gb,price_per_day,disconnect_on_zero,allow_passwordless,is_active,sort_order,created_at FROM plans ORDER BY is_active DESC, sort_order ASC, id DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	plans := []Plan{}
	for rows.Next() {
		plan, err := scanPlan(rows)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		plans = append(plans, plan)
	}
	writeJSON(w, map[string]any{"ok": true, "plans": plans})
}

func (s *Server) createPlan(w http.ResponseWriter, r *http.Request) {
	var in Plan
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.DataGB < 0 || in.SpeedMbps < 0 || in.DurationDays < 0 || in.Price < 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_plan"})
		return
	}
	if in.BillingType == "" {
		in.BillingType = "quota"
	}
	if in.BillingType != "quota" && in.BillingType != "payg" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_billing_type"})
		return
	}
	if in.PricePerGB < 0 || in.PricePerDay < 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_plan"})
		return
	}
	res, err := s.DB.Exec(`INSERT INTO plans(name,data_gb,speed_mbps,duration_days,price,billing_type,price_per_gb,price_per_day,disconnect_on_zero,allow_passwordless,is_active,sort_order) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.Name, in.DataGB, in.SpeedMbps, in.DurationDays, in.Price, in.BillingType, in.PricePerGB, in.PricePerDay, boolInt(in.DisconnectOnZero), boolInt(in.AllowPasswordless), boolInt(in.IsActive), in.SortOrder)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "plan.created", "plan", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name, "billing_type": in.BillingType}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) getPlan(w http.ResponseWriter, id int64) {
	row := s.DB.QueryRow(`SELECT id,name,data_gb,speed_mbps,duration_days,price,billing_type,price_per_gb,price_per_day,disconnect_on_zero,allow_passwordless,is_active,sort_order,created_at FROM plans WHERE id=? LIMIT 1`, id)
	plan, err := scanPlan(row)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "plan": plan})
}

func (s *Server) updatePlan(w http.ResponseWriter, r *http.Request, id int64) {
	var in Plan
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.DataGB < 0 || in.SpeedMbps < 0 || in.DurationDays < 0 || in.Price < 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_plan"})
		return
	}
	if in.BillingType == "" {
		in.BillingType = "quota"
	}
	if in.BillingType != "quota" && in.BillingType != "payg" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_billing_type"})
		return
	}
	if in.PricePerGB < 0 || in.PricePerDay < 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_plan"})
		return
	}
	if _, err := s.DB.Exec(`UPDATE plans SET name=?,data_gb=?,speed_mbps=?,duration_days=?,price=?,billing_type=?,price_per_gb=?,price_per_day=?,disconnect_on_zero=?,allow_passwordless=?,is_active=?,sort_order=? WHERE id=?`,
		in.Name, in.DataGB, in.SpeedMbps, in.DurationDays, in.Price, in.BillingType, in.PricePerGB, in.PricePerDay, boolInt(in.DisconnectOnZero), boolInt(in.AllowPasswordless), boolInt(in.IsActive), in.SortOrder, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "plan.updated", "plan", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name, "billing_type": in.BillingType}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) archivePlan(w http.ResponseWriter, r *http.Request, id int64) {
	if _, err := s.DB.Exec(`UPDATE plans SET is_active=0 WHERE id=?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "plan.deactivated", "plan", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) customerUsername(id int64) (string, error) {
	var username string
	err := s.DB.QueryRow(`SELECT username FROM customers WHERE id=? AND deleted_at IS NULL LIMIT 1`, id).Scan(&username)
	return username, err
}

func (s *Server) upsertRadCheck(username, attribute, value string) error {
	res, err := s.DB.Exec(`UPDATE radcheck SET value=? WHERE username=? AND attribute=?`, value, username, attribute)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected > 0 {
		return nil
	}
	_, err = s.DB.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES(?,?,':=',?)`, username, attribute, value)
	return err
}

func (s *Server) upsertRadReply(username, attribute, value string) error {
	res, err := s.DB.Exec(`UPDATE radreply SET value=? WHERE username=? AND attribute=?`, value, username, attribute)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected > 0 {
		return nil
	}
	_, err = s.DB.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES(?,?,':=',?)`, username, attribute, value)
	return err
}

func speedLimitValue(mbps float64) string {
	if math.Abs(mbps-math.Round(mbps)) < 0.001 {
		v := strconv.FormatInt(int64(math.Round(mbps)), 10) + "M"
		return v + "/" + v
	}
	v := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", mbps), "0"), ".") + "M"
	return v + "/" + v
}

func validCustomerStatus(status string) bool {
	switch status {
	case "active", "disabled", "expired", "limited":
		return true
	default:
		return false
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

type planScanner interface {
	Scan(dest ...any) error
}

func scanPlan(row planScanner) (Plan, error) {
	var p Plan
	var active int
	var disconnectOnZero int
	var allowPasswordless int
	var created sql.NullTime
	var billingType sql.NullString
	if err := row.Scan(&p.ID, &p.Name, &p.DataGB, &p.SpeedMbps, &p.DurationDays, &p.Price, &billingType, &p.PricePerGB, &p.PricePerDay, &disconnectOnZero, &allowPasswordless, &active, &p.SortOrder, &created); err != nil {
		return p, err
	}
	p.IsActive = active == 1
	p.DisconnectOnZero = disconnectOnZero == 1
	p.AllowPasswordless = allowPasswordless == 1
	if billingType.Valid {
		p.BillingType = billingType.String
	} else {
		p.BillingType = "quota"
	}
	if created.Valid {
		p.CreatedAt = created.Time.Format(time.RFC3339)
	}
	return p, nil
}

func pathID(urlPath, prefix string) (int64, string, bool) {
	rest := strings.Trim(strings.TrimPrefix(urlPath, prefix), "/")
	if rest == "" || strings.HasPrefix(rest, "../") {
		return 0, "", false
	}
	parts := strings.Split(rest, "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", false
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	return id, action, true
}

func (s *Server) nodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listNodes(w, r)
	case http.MethodPost:
		s.createNode(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) nodeByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/nodes/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if action == "" {
		switch r.Method {
		case http.MethodGet:
			s.getNode(w, id)
		case http.MethodPatch:
			s.updateNode(w, r, id)
		case http.MethodDelete:
			s.deleteNode(w, id)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "rotate-token":
		s.rotateNodeToken(w, r, id)
	case "enable":
		s.setNodeStatus(w, id, "offline")
	case "disable":
		s.setNodeStatus(w, id, "disabled")
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) {
	s.markStaleNodes()
	rows, err := s.DB.Query(`SELECT id,name,public_ip,COALESCE(domain,''),status,last_seen_at,created_at,proxy_config FROM nodes ORDER BY id DESC LIMIT 500`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	out := []Node{}
	for rows.Next() {
		node, err := s.scanNode(rows)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_ = s.fillNodeRuntime(&node)
		out = append(out, node)
	}
	writeJSON(w, map[string]any{"ok": true, "nodes": out})
}

func (s *Server) getNode(w http.ResponseWriter, id int64) {
	s.markStaleNodes()
	node, err := s.scanNode(s.DB.QueryRow(`SELECT id,name,public_ip,COALESCE(domain,''),status,last_seen_at,created_at,proxy_config FROM nodes WHERE id=? LIMIT 1`, id))
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = s.fillNodeRuntime(&node)
	writeJSON(w, map[string]any{"ok": true, "node": node})
}

func (s *Server) createNode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		PublicIP string `json:"public_ip"`
		Domain   string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.PublicIP = strings.TrimSpace(in.PublicIP)
	in.Domain = strings.TrimSpace(in.Domain)
	if in.Name == "" || in.PublicIP == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_public_ip_required"})
		return
	}
	token := "kn_" + auth.RandomToken(24)
	res, err := s.DB.Exec(`INSERT INTO nodes(name,public_ip,domain,api_token_hash,status) VALUES(?,?,?,?, 'offline')`, in.Name, in.PublicIP, nullString(in.Domain), hashToken(token))
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	// Create default VPN configs for all protocols on the new node
	defaultConfigs := []struct {
		protocol string
		port     int
		network  string
		extra    string
	}{
		{"openvpn", 1194, "10.8.0.0/20", `{"transport":"udp","cipher":"AES-256-GCM","tls_mode":"tls-crypt","dns1":"8.8.8.8","dns2":"8.8.4.4","comp_lzo":false,"topology":"subnet","verb":3,"keepalive":"10 120"}`},
		{"l2tp", 1701, "10.9.0.0/20", `{"ipsec_mode":"ipsec","psk":"","auth_method":"CHAP","dns1":"8.8.8.8","dns2":"8.8.4.4","lcp_echo_interval":30,"lcp_echo_failure":4}`},
		{"ikev2", 500, "10.10.0.0/20", `{"auth_type":"psk","psk":"","dns1":"8.8.8.8","dns2":"8.8.4.4","dpd_interval":30,"dpd_timeout":150,"rekey_time":"4h","ike_proposals":"aes256-sha256-modp2048","esp_proposals":"aes256-sha256"}`},
		{"ssh", 2222, "", `{"listen_address":"0.0.0.0","key_type":"ed25519","max_sessions":10,"idle_timeout":0,"shell_access":false,"accounting_enabled":true,"accounting_interval":300}`},
		{"wireguard", 51820, "10.66.0.0/20", `{"dns_1":"1.1.1.1","dns_2":"8.8.8.8","gaming_optimize":false}`},
	}
	for _, dc := range defaultConfigs {
		_, _ = s.DB.Exec(`INSERT INTO node_vpn_configs(node_id, protocol, enabled, port, network, extra_json) VALUES(?, ?, 0, ?, ?, ?)`,
			id, dc.protocol, dc.port, dc.network, dc.extra)
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "node.created", "node", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": id, "token": token})
}

func (s *Server) updateNode(w http.ResponseWriter, r *http.Request, id int64) {
	var in struct {
		Name          string  `json:"name"`
		PublicIP      string  `json:"public_ip"`
		Domain        string  `json:"domain"`
		ProxyEnabled  *bool   `json:"proxy_enabled,omitempty"`
		ProxyType     *string `json:"proxy_type,omitempty"`
		ProxyAddress  *string `json:"proxy_address,omitempty"`
		ProxyUsername *string `json:"proxy_username,omitempty"`
		ProxyPassword *string `json:"proxy_password,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.PublicIP = strings.TrimSpace(in.PublicIP)
	in.Domain = strings.TrimSpace(in.Domain)
	if in.Name == "" || in.PublicIP == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_public_ip_required"})
		return
	}

	// Build proxy_config JSON if any proxy fields are provided
	var proxyConfigJSON *string
	if in.ProxyEnabled != nil || in.ProxyType != nil || in.ProxyAddress != nil {
		pc := map[string]any{}
		if in.ProxyEnabled != nil {
			pc["enabled"] = *in.ProxyEnabled
		}
		if in.ProxyType != nil {
			pc["type"] = *in.ProxyType
		}
		if in.ProxyAddress != nil {
			pc["address"] = *in.ProxyAddress
		}
		if in.ProxyUsername != nil {
			pc["username"] = *in.ProxyUsername
		}
		if in.ProxyPassword != nil {
			pc["password"] = *in.ProxyPassword
		}
		b, _ := json.Marshal(pc)
		s := string(b)
		proxyConfigJSON = &s
	}

	if proxyConfigJSON != nil {
		if _, err := s.DB.Exec(`UPDATE nodes SET name=?,public_ip=?,domain=?,proxy_config=? WHERE id=?`, in.Name, in.PublicIP, nullString(in.Domain), *proxyConfigJSON, id); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	} else {
		if _, err := s.DB.Exec(`UPDATE nodes SET name=?,public_ip=?,domain=? WHERE id=?`, in.Name, in.PublicIP, nullString(in.Domain), id); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "node.updated", "node", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name}, clientIP(r))

	// Push config update to the node agent so NODE_NAME stays in sync
	configPayload := map[string]any{
		"NODE_NAME": in.Name,
	}
	payloadJSON, _ := json.Marshal(map[string]any{"config": configPayload})
	_, _ = s.DB.Exec(`INSERT INTO node_tasks(node_id, action, payload_json, status, created_by) VALUES(?, 'agent.update_config', ?, 'pending', ?)`,
		id, string(payloadJSON), actor)

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) rotateNodeToken(w http.ResponseWriter, r *http.Request, id int64) {
	token := "kn_" + auth.RandomToken(24)
	if _, err := s.DB.Exec(`UPDATE nodes SET api_token_hash=? WHERE id=?`, hashToken(token), id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Push new token to the node agent before returning
	// The agent will update its NODE_TOKEN env var and reload
	configPayload := map[string]any{
		"NODE_TOKEN": token,
	}
	payloadJSON, _ := json.Marshal(map[string]any{"config": configPayload})
	_, _ = s.DB.Exec(`INSERT INTO node_tasks(node_id, action, payload_json, status, created_by) VALUES(?, 'agent.update_config', ?, 'pending', ?)`,
		id, string(payloadJSON), "system")

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "node.token_rotated", "node", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "token": token})
}

func (s *Server) setNodeStatus(w http.ResponseWriter, id int64, status string) {
	if _, err := s.DB.Exec(`UPDATE nodes SET status=? WHERE id=?`, status, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// When disabling a node, disconnect all active sessions on it and revoke WireGuard peers
	if status == "disabled" {
		// Get node's NAS IP for RADIUS disconnect
		var nasIP string
		_ = s.DB.QueryRow(`SELECT public_ip FROM nodes WHERE id=?`, id).Scan(&nasIP)
		if nasIP == "" {
			nasIP = "127.0.0.1"
		}

		// Disconnect all active RADIUS sessions originating from this node
		rows, err := s.DB.Query(`SELECT radacctid, username, acctsessionid FROM radacct WHERE acctstoptime IS NULL AND nasipaddress=?`, nasIP)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var radID int64
				var username, sessionID string
				if rows.Scan(&radID, &username, &sessionID) == nil {
					// Close the session in radacct
					_, _ = s.DB.Exec(`UPDATE radacct SET acctstoptime=NOW(), acctterminatecause='Admin-Node-Disabled' WHERE radacctid=?`, radID)
					// Send CoA disconnect (best effort)
					go func(u, sid, ip string) {
						attrs := fmt.Sprintf("User-Name=%s,Acct-Session-Id=%s", u, sid)
						cmd := exec.Command("radclient", "-x", ip+":3799", "disconnect", "testing123")
						cmd.Stdin = strings.NewReader(attrs)
						_ = cmd.Run()
					}(username, sessionID, nasIP)
				}
			}
		}

		// Revoke WireGuard peers on this node
		_, _ = s.DB.Exec(`UPDATE wg_peers SET status='revoked' WHERE node_id=? AND status='active'`, id)

		log.Printf("[node] disabled node %d, disconnected sessions and revoked WG peers", id)
	}

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) deleteNode(w http.ResponseWriter, id int64) {
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	// Clean up all related tables within a transaction (explicit queries, no concatenation)
	if _, err := tx.Exec(`DELETE FROM node_vpn_configs WHERE node_id=?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Sprintf("failed to clean node_vpn_configs: %v", err)})
		return
	}
	if _, err := tx.Exec(`DELETE FROM node_tasks WHERE node_id=?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Sprintf("failed to clean node_tasks: %v", err)})
		return
	}
	if _, err := tx.Exec(`DELETE FROM node_status WHERE node_id=?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Sprintf("failed to clean node_status: %v", err)})
		return
	}
	if _, err := tx.Exec(`DELETE FROM node_services WHERE node_id=?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Sprintf("failed to clean node_services: %v", err)})
		return
	}
	if _, err := tx.Exec(`DELETE FROM node_usage_snapshots WHERE node_id=?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Sprintf("failed to clean node_usage_snapshots: %v", err)})
		return
	}
	if _, err := tx.Exec(`DELETE FROM node_diagnostics WHERE node_id=?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": fmt.Sprintf("failed to clean node_diagnostics: %v", err)})
		return
	}

	if _, err := tx.Exec(`DELETE FROM nodes WHERE id=?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

type nodeScanner interface{ Scan(dest ...any) error }

func (s *Server) scanNode(row nodeScanner) (Node, error) {
	var n Node
	var lastSeen, created sql.NullTime
	var proxyConfig []byte
	if err := row.Scan(&n.ID, &n.Name, &n.PublicIP, &n.Domain, &n.Status, &lastSeen, &created, &proxyConfig); err != nil {
		return n, err
	}
	if lastSeen.Valid {
		n.LastSeenAt = lastSeen.Time.Format(time.RFC3339)
	}
	if created.Valid {
		n.CreatedAt = created.Time.Format(time.RFC3339)
	}
	if len(proxyConfig) > 0 {
		n.ProxyConfig = json.RawMessage(proxyConfig)
	}
	return n, nil
}

func (s *Server) fillNodeRuntime(n *Node) error {
	var updated sql.NullTime
	_ = s.DB.QueryRow(`SELECT cpu_percent,ram_percent,disk_percent,rx_bps,tx_bps,openvpn_status,l2tp_status,ikev2_status,updated_at FROM node_status WHERE node_id=?`, n.ID).Scan(&n.StatusMetrics.CPUPercent, &n.StatusMetrics.RAMPercent, &n.StatusMetrics.DiskPercent, &n.StatusMetrics.RxBps, &n.StatusMetrics.TxBps, &n.StatusMetrics.OpenVPN, &n.StatusMetrics.L2TP, &n.StatusMetrics.IKEv2, &updated)
	if updated.Valid {
		n.StatusMetrics.UpdatedAt = updated.Time.Format(time.RFC3339)
	}
	rows, err := s.DB.Query(`SELECT service,status,updated_at FROM node_services WHERE node_id=? ORDER BY service`, n.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var svc Service
			var t sql.NullTime
			if err := rows.Scan(&svc.Service, &svc.Status, &t); err == nil {
				if t.Valid {
					svc.UpdatedAt = t.Time.Format(time.RFC3339)
				}
				n.Services = append(n.Services, svc)
				// Populate SSH status from services
				if svc.Service == "ssh" && svc.Status != "" {
					n.StatusMetrics.SSH = svc.Status
				}
			}
		}
	}

	hRows, err := s.DB.Query(`SELECT id, node_id, rx_bytes, tx_bytes, online_users, created_at FROM node_usage_snapshots WHERE node_id=? ORDER BY id DESC LIMIT 15`, n.ID)
	if err == nil {
		defer hRows.Close()
		for hRows.Next() {
			var snap NodeUsageSnapshot
			var t time.Time
			if err := hRows.Scan(&snap.ID, &snap.NodeID, &snap.RxBytes, &snap.TxBytes, &snap.OnlineUsers, &t); err == nil {
				snap.CreatedAt = t.Format(time.RFC3339)
				n.History = append(n.History, snap)
			}
		}
	}

	var diag DiagnosticsReport
	err = s.DB.QueryRow(`SELECT agent_version, uptime_seconds, go_version, goroutines, mem_alloc_bytes FROM node_diagnostics WHERE node_id=?`, n.ID).Scan(
		&diag.AgentVersion, &diag.UptimeSeconds, &diag.GoVersion, &diag.Goroutines, &diag.MemAllocBytes)
	if err == nil {
		n.Diagnostics = &diag
	}

	return nil
}

func (s *Server) markStaleNodes() {
	_, _ = s.DB.Exec(`UPDATE nodes SET status='stale' WHERE status='online' AND last_seen_at < (NOW() - INTERVAL 2 MINUTE)`)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func nullString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.TrimSpace(v)
}

func (s *Server) vpnSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.readVPNSettings(r)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "settings": settings})
	case http.MethodPatch:
		var in struct {
			VPNSettings
			Apply bool `json:"apply"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}
		in.OpenVPNProtocol = strings.ToLower(strings.TrimSpace(in.OpenVPNProtocol))
		if in.OpenVPNProtocol != "udp" && in.OpenVPNProtocol != "tcp" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_protocol"})
			return
		}
		if in.OpenVPNPort <= 0 || in.OpenVPNPort > 65535 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_port"})
			return
		}
		if in.OpenVPNNetwork == "" || in.L2TPNetwork == "" || in.IKEv2Network == "" || in.DNS1 == "" || in.DNS2 == "" {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing_required_settings"})
			return
		}
		// Validate network CIDRs are private RFC1918 ranges
		if err := templates.ValidatePrivateNetwork(in.OpenVPNNetwork, false); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid openvpn_network: " + err.Error()})
			return
		}
		if err := templates.ValidatePrivateNetwork(in.L2TPNetwork, false); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid l2tp_network: " + err.Error()})
			return
		}
		if err := templates.ValidatePrivateNetwork(in.IKEv2Network, false); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid ikev2_network: " + err.Error()})
			return
		}
		// Validate port, protocol, and DNS
		if err := templates.ValidatePort(in.OpenVPNPort); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := templates.ValidateProtocol(in.OpenVPNProtocol); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if err := templates.ValidateDNS(in.DNS1); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid dns_1: " + err.Error()})
			return
		}
		if err := templates.ValidateDNS(in.DNS2); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid dns_2: " + err.Error()})
			return
		}
		_, err := s.DB.Exec(`INSERT INTO vpn_core_settings(id,openvpn_port,openvpn_protocol,openvpn_network,l2tp_network,ikev2_network,ipsec_psk,dns_1,dns_2)
			VALUES(1,?,?,?,?,?,?,?,?)
			ON DUPLICATE KEY UPDATE openvpn_port=VALUES(openvpn_port),openvpn_protocol=VALUES(openvpn_protocol),openvpn_network=VALUES(openvpn_network),l2tp_network=VALUES(l2tp_network),ikev2_network=VALUES(ikev2_network),ipsec_psk=VALUES(ipsec_psk),dns_1=VALUES(dns_1),dns_2=VALUES(dns_2)`, in.OpenVPNPort, in.OpenVPNProtocol, in.OpenVPNNetwork, in.L2TPNetwork, in.IKEv2Network, nullString(in.IPSecPSK), in.DNS1, in.DNS2)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		applied := false
		applyError := ""
		if in.Apply {
			if err := applyOpenVPNServerConfig(in.VPNSettings); err != nil {
				applyError = err.Error()
			} else {
				applied = true
			}
		}
		settings, _ := s.readVPNSettings(r)
		actor, _, _ := s.currentAdmin(r)
		s.logAudit(actor, "vpn.settings_saved", "vpn_settings", "1", nil, map[string]any{"applied": applied}, clientIP(r))
		writeJSON(w, map[string]any{"ok": true, "settings": settings, "applied": applied, "apply_error": applyError})
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) readVPNSettings(r *http.Request) (VPNSettings, error) {
	var v VPNSettings
	var updated sql.NullTime
	err := s.DB.QueryRow(`SELECT id,openvpn_port,openvpn_protocol,openvpn_network,l2tp_network,ikev2_network,COALESCE(ipsec_psk,''),dns_1,dns_2,updated_at FROM vpn_core_settings WHERE id=1`).Scan(&v.ID, &v.OpenVPNPort, &v.OpenVPNProtocol, &v.OpenVPNNetwork, &v.L2TPNetwork, &v.IKEv2Network, &v.IPSecPSK, &v.DNS1, &v.DNS2, &updated)
	if err == sql.ErrNoRows {
		_, _ = s.DB.Exec(`INSERT IGNORE INTO vpn_core_settings(id) VALUES(1)`)
		err = s.DB.QueryRow(`SELECT id,openvpn_port,openvpn_protocol,openvpn_network,l2tp_network,ikev2_network,COALESCE(ipsec_psk,''),dns_1,dns_2,updated_at FROM vpn_core_settings WHERE id=1`).Scan(&v.ID, &v.OpenVPNPort, &v.OpenVPNProtocol, &v.OpenVPNNetwork, &v.L2TPNetwork, &v.IKEv2Network, &v.IPSecPSK, &v.DNS1, &v.DNS2, &updated)
	}
	if err != nil {
		return v, err
	}
	if updated.Valid {
		v.UpdatedAt = updated.Time.Format(time.RFC3339)
	}
	ca := getenvFirst("PANEL_OPENVPN_CA_FILE", "/etc/openvpn/server/ca.crt", "/etc/openvpn/easy-rsa/pki/ca.crt")
	tls := getenvFirst("PANEL_OPENVPN_TLS_CRYPT_FILE", "/etc/openvpn/server/tc.key", "/etc/openvpn/server/tls-crypt.key", "/etc/openvpn/server/ta.key")
	v.CAFile = ca
	v.TLSCryptFile = tls
	_, v.CAExists = fileExists(ca)
	_, v.TLSCryptExists = fileExists(tls)
	v.RemoteHost, _, _, v.ActiveNode = s.openVPNEndpoint(r)
	v.OpenVPNServiceStatus = "unknown"
	_ = s.DB.QueryRow(`SELECT openvpn_status FROM node_status ORDER BY updated_at DESC LIMIT 1`).Scan(&v.OpenVPNServiceStatus)
	return v, nil
}

func fileExists(path string) (os.FileInfo, bool) {
	if strings.TrimSpace(path) == "" {
		return nil, false
	}
	info, err := os.Stat(path)
	return info, err == nil
}

func applyOpenVPNServerConfig(v VPNSettings) error {
	// Validate inputs
	if err := templates.ValidatePort(v.OpenVPNPort); err != nil {
		return fmt.Errorf("port validation failed: %w", err)
	}
	if err := templates.ValidateProtocol(v.OpenVPNProtocol); err != nil {
		return fmt.Errorf("protocol validation failed: %w", err)
	}
	if v.OpenVPNNetwork == "" {
		return fmt.Errorf("network validation failed: OpenVPN network is required")
	}

	conf := strings.TrimSpace(os.Getenv("PANEL_OPENVPN_SERVER_CONF"))
	if conf == "" {
		conf = "/etc/openvpn/server/server.conf"
	}

	serverNet, serverMask := cidrToOpenVPNServer(v.OpenVPNNetwork)

	vars := templates.TemplateVars{
		Port:       v.OpenVPNPort,
		Protocol:   v.OpenVPNProtocol,
		Network:    v.OpenVPNNetwork,
		ServerNet:  serverNet,
		ServerMask: serverMask,
		DNS1:       v.DNS1,
		DNS2:       v.DNS2,
	}

	engine := templates.NewEngine(os.Getenv("PANEL_TEMPLATE_DIR"))
	if err := engine.Apply("openvpn", conf, vars); err != nil {
		return err
	}

	cmd := exec.Command("systemctl", "restart", "openvpn-server@server")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restart openvpn: %w: %s", err, string(out))
	}
	return nil
}

func cidrToOpenVPNServer(cidr string) (string, string) {
	ip, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil || ip.To4() == nil {
		return "", ""
	}
	mask := ipNet.Mask
	return ip.To4().String(), fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}

func (s *Server) nodeTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listNodeTasks(w, r)
	case http.MethodPost:
		s.createNodeTask(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) nodeTaskByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/node/tasks/")
	if !ok || action == "" || r.Method != http.MethodPost {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch action {
	case "cancel":
		if _, _, ok := s.currentAdmin(r); !ok {
			writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		s.cancelNodeTask(w, id)
	case "complete":
		s.completeNodeTask(w, r, id)
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) listNodeTasks(w http.ResponseWriter, r *http.Request) {
	where := "1=1"
	args := []any{}
	if nodeID := strings.TrimSpace(r.URL.Query().Get("node_id")); nodeID != "" {
		where += " AND t.node_id=?"
		args = append(args, nodeID)
	}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		where += " AND t.status=?"
		args = append(args, status)
	}
	rows, err := s.DB.Query(`SELECT t.id,t.node_id,n.name,t.action,COALESCE(t.payload_json,JSON_OBJECT()),t.status,COALESCE(t.result_json,JSON_OBJECT()),COALESCE(t.error,''),COALESCE(t.created_by,''),t.claimed_at,t.completed_at,t.created_at,t.updated_at FROM node_tasks t LEFT JOIN nodes n ON n.id=t.node_id WHERE `+where+` ORDER BY t.id DESC LIMIT 500`, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	tasks := []NodeTask{}
	for rows.Next() {
		task, err := scanNodeTask(rows)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		tasks = append(tasks, task)
	}
	writeJSON(w, map[string]any{"ok": true, "tasks": tasks})
}

func (s *Server) createNodeTask(w http.ResponseWriter, r *http.Request) {
	actor, _, _ := s.currentAdmin(r)
	var in struct {
		NodeID  int64           `json:"node_id"`
		Action  string          `json:"action"`
		Payload json.RawMessage `json:"payload_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Action = strings.TrimSpace(in.Action)
	if in.NodeID <= 0 || in.Action == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_action_required"})
		return
	}
	if !validNodeTaskAction(in.Action) {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_action"})
		return
	}
	payload := string(in.Payload)
	if strings.TrimSpace(payload) == "" || strings.TrimSpace(payload) == "null" {
		payload = `{}`
	}
	res, err := s.DB.Exec(`INSERT INTO node_tasks(node_id,action,payload_json,created_by) VALUES(?,?,?,?)`, in.NodeID, in.Action, payload, actor)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	s.logAudit(actor, "node_task.created", "node_task", strconv.FormatInt(id, 10), nil, map[string]any{"node_id": in.NodeID, "action": in.Action}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) cancelNodeTask(w http.ResponseWriter, id int64) {
	if _, err := s.DB.Exec(`UPDATE node_tasks SET status='cancelled',completed_at=NOW() WHERE id=? AND status IN('pending','running')`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) nodeTaskPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	nodeID, ok := s.authNode(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "bad_token"})
		return
	}
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT t.id,t.node_id,n.name,t.action,COALESCE(t.payload_json,JSON_OBJECT()),t.status,COALESCE(t.result_json,JSON_OBJECT()),COALESCE(t.error,''),COALESCE(t.created_by,''),t.claimed_at,t.completed_at,t.created_at,t.updated_at FROM node_tasks t LEFT JOIN nodes n ON n.id=t.node_id WHERE t.node_id=? AND t.status='pending' ORDER BY t.id ASC LIMIT 5 FOR UPDATE`, nodeID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	tasks := []NodeTask{}
	ids := []any{}
	for rows.Next() {
		task, err := scanNodeTask(rows)
		if err == nil {
			tasks = append(tasks, task)
			ids = append(ids, task.ID)
		}
	}
	rows.Close()
	for _, id := range ids {
		_, _ = tx.Exec(`UPDATE node_tasks SET status='running',claimed_at=NOW() WHERE id=? AND status='pending'`, id)
	}
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "tasks": tasks})
}

func (s *Server) completeNodeTask(w http.ResponseWriter, r *http.Request, id int64) {
	nodeID, ok := s.authNode(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "bad_token"})
		return
	}
	var in struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result_json"`
		Error  string          `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.Status != "succeeded" && in.Status != "failed" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_status"})
		return
	}
	result := string(in.Result)
	if strings.TrimSpace(result) == "" || strings.TrimSpace(result) == "null" {
		result = `{}`
	}
	res, err := s.DB.Exec(`UPDATE node_tasks SET status=?,result_json=?,error=?,completed_at=NOW() WHERE id=? AND node_id=? AND status IN('pending','running')`, in.Status, result, in.Error, id, nodeID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "task_not_found"})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func scanNodeTask(row interface{ Scan(dest ...any) error }) (NodeTask, error) {
	var t NodeTask
	var payload, result []byte
	var claimed, completed, created, updated sql.NullTime
	if err := row.Scan(&t.ID, &t.NodeID, &t.NodeName, &t.Action, &payload, &t.Status, &result, &t.Error, &t.CreatedBy, &claimed, &completed, &created, &updated); err != nil {
		return t, err
	}
	t.Payload = json.RawMessage(payload)
	t.Result = json.RawMessage(result)
	if claimed.Valid {
		t.ClaimedAt = claimed.Time.Format(time.RFC3339)
	}
	if completed.Valid {
		t.CompletedAt = completed.Time.Format(time.RFC3339)
	}
	if created.Valid {
		t.CreatedAt = created.Time.Format(time.RFC3339)
	}
	if updated.Valid {
		t.UpdatedAt = updated.Time.Format(time.RFC3339)
	}
	return t, nil
}

func validNodeTaskAction(action string) bool {
	switch action {
	case "service.restart", "service.status", "service.reload", "service.stop", "agent.status",
		"agent.update", "agent.reload_config",
		"vpn.disconnect-user", "vpn.apply_outbound",
		"wireguard.setup", "wireguard.add_peer", "wireguard.remove_peer",
		"wireguard.update_config", "wireguard.sync_config",
		"cert.distribute",
		"backup.collect_configs", "backup.restore_configs":
		return true
	default:
		return false
	}
}

// authNode authenticates a node request.
// Token is read ONLY from the X-Node-Token header. The request body is never consumed,
// allowing downstream handlers to read it for their own purposes.
func (s *Server) authNode(r *http.Request) (int64, bool) {
	token := r.Header.Get("X-Node-Token")
	if token == "" {
		return 0, false
	}
	var id int64
	var status string
	if err := s.DB.QueryRow(`SELECT id,status FROM nodes WHERE api_token_hash=? LIMIT 1`, hashToken(token)).Scan(&id, &status); err != nil || status == "disabled" {
		return 0, false
	}
	return id, true
}

func (s *Server) realtimeWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return s.checkWSOrigin(r)
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(65 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(65 * time.Second))
		return nil
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// Use a mutex to serialize all writes to the WebSocket connection.
	// gorilla/websocket supports only one concurrent writer.
	var wsMu sync.Mutex
	writeRealtime := func(kind string, data any) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		return conn.WriteJSON(map[string]any{"type": kind, "time": time.Now().UTC(), "data": data})
	}
	writePing := func() error {
		wsMu.Lock()
		defer wsMu.Unlock()
		return conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
	}

	_ = writeRealtime("stats", s.dashboardStatsPayload())
	_ = writeRealtime("sessions", s.liveSessionsPayload())
	notifCh := s.addWSSubscriber()
	defer s.removeWSSubscriber(notifCh)
	ticker := time.NewTicker(3 * time.Second)
	pingTicker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	defer pingTicker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := writeRealtime("stats", s.dashboardStatsPayload()); err != nil {
				return
			}
			if err := writeRealtime("sessions", s.liveSessionsPayload()); err != nil {
				return
			}
			if err := writeRealtime("bandwidth", s.bandwidthPayload()); err != nil {
				return
			}
		case <-pingTicker.C:
			if err := writePing(); err != nil {
				return
			}
		case notif := <-notifCh:
			if err := writeRealtime("notification", notif); err != nil {
				return
			}
		}
	}
}

func (s *Server) paymentMethods(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPaymentMethods(w, false)
	case http.MethodPost:
		s.createPaymentMethod(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) paymentMethodByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/payment-methods/")
	if !ok || action != "" {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		s.updatePaymentMethod(w, r, id)
	case http.MethodDelete:
		s.deactivatePaymentMethod(w, r, id)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) portalPaymentMethods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	s.listPaymentMethods(w, true)
}

func (s *Server) listPaymentMethods(w http.ResponseWriter, activeOnly bool) {
	var rows *sql.Rows
	var err error
	if activeOnly {
		rows, err = s.DB.Query(`SELECT id,name,type,COALESCE(JSON_UNQUOTE(JSON_EXTRACT(config_json,'$.instructions')),''),is_active,sort_order,created_at FROM payment_methods WHERE is_active=1 ORDER BY sort_order ASC, id DESC`)
	} else {
		rows, err = s.DB.Query(`SELECT id,name,type,COALESCE(JSON_UNQUOTE(JSON_EXTRACT(config_json,'$.instructions')),''),is_active,sort_order,created_at FROM payment_methods ORDER BY is_active DESC, sort_order ASC, id DESC`)
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	methods := []PaymentMethod{}
	for rows.Next() {
		method, err := scanPaymentMethod(rows)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		methods = append(methods, method)
	}
	writeJSON(w, map[string]any{"ok": true, "methods": methods})
}

func (s *Server) createPaymentMethod(w http.ResponseWriter, r *http.Request) {
	var in PaymentMethod
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.TrimSpace(in.Type)
	if in.Name == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_required"})
		return
	}
	if in.Type == "" {
		in.Type = "manual"
	}
	res, err := s.DB.Exec(`INSERT INTO payment_methods(name,type,config_json,is_active,sort_order) VALUES(?,?,JSON_OBJECT('instructions', ?),?,?)`, in.Name, in.Type, in.Instructions, boolInt(in.IsActive), in.SortOrder)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "payment_method.created", "payment_method", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) updatePaymentMethod(w http.ResponseWriter, r *http.Request, id int64) {
	var in PaymentMethod
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.TrimSpace(in.Type)
	if in.Name == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_required"})
		return
	}
	if in.Type == "" {
		in.Type = "manual"
	}
	if _, err := s.DB.Exec(`UPDATE payment_methods SET name=?,type=?,config_json=JSON_OBJECT('instructions', ?),is_active=?,sort_order=? WHERE id=?`, in.Name, in.Type, in.Instructions, boolInt(in.IsActive), in.SortOrder, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "payment_method.updated", "payment_method", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) deactivatePaymentMethod(w http.ResponseWriter, r *http.Request, id int64) {
	if _, err := s.DB.Exec(`UPDATE payment_methods SET is_active=0 WHERE id=?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "payment_method.deactivated", "payment_method", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

type paymentMethodScanner interface{ Scan(dest ...any) error }

func scanPaymentMethod(row paymentMethodScanner) (PaymentMethod, error) {
	var method PaymentMethod
	var active int
	var created sql.NullTime
	if err := row.Scan(&method.ID, &method.Name, &method.Type, &method.Instructions, &active, &method.SortOrder, &created); err != nil {
		return method, err
	}
	method.IsActive = active == 1
	if created.Valid {
		method.CreatedAt = created.Time.Format(time.RFC3339)
	}
	return method, nil
}

func (s *Server) tickets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listTickets(w, r, "")
	case http.MethodPost:
		s.createTicket(w, r, "admin", "")
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) ticketByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/tickets/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if action == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.getTicket(w, r, id, "")
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "reply":
		s.replyTicket(w, r, id, "admin", "")
	case "close":
		s.setTicketStatus(w, r, id, "closed")
	case "open":
		s.setTicketStatus(w, r, id, "open")
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) portalTickets(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listTickets(w, r, username)
	case http.MethodPost:
		s.createTicket(w, r, "customer", username)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) portalTicketByID(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	id, action, ok := pathID(r.URL.Path, "/api/portal/tickets/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if action == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		s.getTicket(w, r, id, username)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "reply":
		s.replyTicket(w, r, id, "customer", username)
	case "close":
		if !s.ticketBelongsTo(id, username) {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		s.setTicketStatus(w, r, id, "closed")
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) listTickets(w http.ResponseWriter, r *http.Request, username string) {
	where := "t.deleted_at IS NULL"
	args := []any{}
	if username != "" {
		where += " AND t.username=?"
		args = append(args, username)
	}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		where += " AND t.status=?"
		args = append(args, status)
	}
	rows, err := s.DB.Query(`SELECT t.id,t.customer_id,t.username,t.subject,t.status,t.priority,t.created_at,t.updated_at,t.closed_at FROM tickets t WHERE `+where+` ORDER BY t.updated_at DESC,t.id DESC LIMIT 500`, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	out := []Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		out = append(out, t)
	}
	writeJSON(w, map[string]any{"ok": true, "tickets": out})
}

func (s *Server) createTicket(w http.ResponseWriter, r *http.Request, senderType, forcedUsername string) {
	actor := forcedUsername
	if senderType == "admin" {
		actor, _, _ = s.currentAdmin(r)
	}
	var in struct {
		Username string `json:"username"`
		Subject  string `json:"subject"`
		Priority string `json:"priority"`
		Message  string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if forcedUsername != "" {
		in.Username = forcedUsername
	}
	in.Username = strings.TrimSpace(in.Username)
	in.Subject = strings.TrimSpace(in.Subject)
	in.Priority = strings.TrimSpace(in.Priority)
	in.Message = strings.TrimSpace(in.Message)
	if in.Priority == "" {
		in.Priority = "normal"
	}
	if !validTicketPriority(in.Priority) || in.Username == "" || in.Subject == "" || in.Message == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_ticket"})
		return
	}
	var customerID sql.NullInt64
	_ = s.DB.QueryRow(`SELECT id FROM customers WHERE username=? AND deleted_at IS NULL LIMIT 1`, in.Username).Scan(&customerID)
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO tickets(customer_id,username,subject,priority,status) VALUES(?,?,?,?, 'open')`, nullableInt(customerID), in.Username, in.Subject, in.Priority)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	if _, err := tx.Exec(`INSERT INTO ticket_messages(ticket_id,sender_type,sender_name,message) VALUES(?,?,?,?)`, id, senderType, actor, in.Message); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.logAudit(actor, "ticket.created", "ticket", strconv.FormatInt(id, 10), nil, map[string]any{"username": in.Username, "subject": in.Subject}, clientIP(r))
	severity := "info"
	if in.Priority == "high" {
		severity = "warning"
	}
	s.createEvent("ticket", severity, fmt.Sprintf("New ticket #%d: %s", id, in.Subject), fmt.Sprintf("Ticket #%d created by %s for %s", id, actor, in.Username), actor, in.Username)
	if senderType == "customer" {
		s.broadcastNotification(map[string]any{
			"id":        fmt.Sprintf("ticket-%d-%d", id, time.Now().UnixMilli()),
			"type":      "new_ticket",
			"message":   fmt.Sprintf("New support ticket from %s: %s", in.Username, in.Subject),
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"read":      false,
		})
		// Telegram notification to admin
		s.Notify.SendEvent("ticket", fmt.Sprintf("🎫 New Ticket #%d", id), fmt.Sprintf("From: %s\nSubject: %s\nPriority: %s", in.Username, in.Subject, in.Priority))
	}
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) getTicket(w http.ResponseWriter, r *http.Request, id int64, username string) {
	if username != "" && !s.ticketBelongsTo(id, username) {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	ticket, err := scanTicket(s.DB.QueryRow(`SELECT id,customer_id,username,subject,status,priority,created_at,updated_at,closed_at FROM tickets WHERE id=? AND deleted_at IS NULL LIMIT 1`, id))
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	messages, err := s.ticketMessages(id)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "ticket": TicketDetail{Ticket: ticket, Messages: messages}})
}

func (s *Server) replyTicket(w http.ResponseWriter, r *http.Request, id int64, senderType, username string) {
	if username != "" && !s.ticketBelongsTo(id, username) {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	sender := username
	if senderType == "admin" {
		sender, _, _ = s.currentAdmin(r)
	}
	var in struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	if in.Message == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "message_required"})
		return
	}
	if _, err := s.DB.Exec(`INSERT INTO ticket_messages(ticket_id,sender_type,sender_name,message) VALUES(?,?,?,?)`, id, senderType, sender, in.Message); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = s.DB.Exec(`UPDATE tickets SET status='open',updated_at=NOW() WHERE id=?`, id)
	s.logAudit(sender, "ticket.replied", "ticket", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
	ticketUser := username
	if ticketUser == "" {
		var tu string
		_ = s.DB.QueryRow(`SELECT username FROM tickets WHERE id=? LIMIT 1`, id).Scan(&tu)
		ticketUser = tu
	}
	s.createEvent("ticket", "info", fmt.Sprintf("Ticket #%d replied", id), fmt.Sprintf("%s replied to ticket #%d", sender, id), sender, ticketUser)
	// Telegram notification when customer replies to admin
	if senderType == "customer" {
		s.Notify.SendEvent("ticket", fmt.Sprintf("💬 Ticket #%d Reply", id), fmt.Sprintf("From: %s\nMessage: %s", sender, in.Message[:min(len(in.Message), 100)]))
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) setTicketStatus(w http.ResponseWriter, r *http.Request, id int64, status string) {
	closedExpr := "NULL"
	if status == "closed" {
		closedExpr = "NOW()"
	}
	if _, err := s.DB.Exec(`UPDATE tickets SET status=?,closed_at=`+closedExpr+`,updated_at=NOW() WHERE id=? AND deleted_at IS NULL`, status, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	if actor == "" {
		actor, _ = s.currentCustomer(r)
	}
	s.logAudit(actor, "ticket.status_changed", "ticket", strconv.FormatInt(id, 10), nil, map[string]any{"status": status}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) ticketBelongsTo(id int64, username string) bool {
	var count int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM tickets WHERE id=? AND username=? AND deleted_at IS NULL`, id, username).Scan(&count)
	return count > 0
}

func (s *Server) ticketMessages(id int64) ([]TicketMessage, error) {
	rows, err := s.DB.Query(`SELECT id,ticket_id,sender_type,sender_name,message,created_at FROM ticket_messages WHERE ticket_id=? ORDER BY id ASC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TicketMessage{}
	for rows.Next() {
		var m TicketMessage
		var created sql.NullTime
		if err := rows.Scan(&m.ID, &m.TicketID, &m.SenderType, &m.SenderName, &m.Message, &created); err != nil {
			return out, err
		}
		if created.Valid {
			m.CreatedAt = created.Time.Format(time.RFC3339)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

type ticketScanner interface{ Scan(dest ...any) error }

func scanTicket(row ticketScanner) (Ticket, error) {
	var t Ticket
	var customerID sql.NullInt64
	var created, updated, closed sql.NullTime
	if err := row.Scan(&t.ID, &customerID, &t.Username, &t.Subject, &t.Status, &t.Priority, &created, &updated, &closed); err != nil {
		return t, err
	}
	if customerID.Valid {
		t.CustomerID = &customerID.Int64
	}
	if created.Valid {
		t.CreatedAt = created.Time.Format(time.RFC3339)
	}
	if updated.Valid {
		t.UpdatedAt = updated.Time.Format(time.RFC3339)
	}
	if closed.Valid {
		t.ClosedAt = closed.Time.Format(time.RFC3339)
	}
	return t, nil
}

func validTicketPriority(priority string) bool {
	switch priority {
	case "low", "normal", "high":
		return true
	default:
		return false
	}
}

func (s *Server) payments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listPayments(w, r)
	case http.MethodPost:
		s.createManualPayment(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) paymentByID(w http.ResponseWriter, r *http.Request) {
	id, action, ok := pathID(r.URL.Path, "/api/payments/")
	if !ok || action == "" || r.Method != http.MethodPost {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	switch action {
	case "approve":
		s.setPaymentStatus(w, r, id, "approved")
	case "reject":
		s.setPaymentStatus(w, r, id, "rejected")
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) walletByUsername(w http.ResponseWriter, r *http.Request) {
	username, action, ok := pathUsername(r.URL.Path, "/api/wallets/")
	if !ok || r.Method != http.MethodPost {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	var in struct {
		Amount      float64 `json:"amount"`
		Balance     float64 `json:"balance"`
		Description string  `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	switch action {
	case "adjust":
		if in.Amount == 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "amount_required"})
			return
		}
		if err := s.applyWalletChange(username, in.Amount, "adjustment", in.Description, "admin"); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	case "set":
		if err := s.setWalletBalance(username, in.Balance, in.Description, "admin"); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "wallet."+action, "wallet", username, nil, map[string]any{"amount": in.Amount, "balance": in.Balance, "description": in.Description}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) listPayments(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(`SELECT p.id,p.username,p.amount,p.method,p.status,COALESCE(p.intent_type,'wallet_topup'),p.intent_id,COALESCE(pl.name,''),p.created_at,p.updated_at FROM payments p LEFT JOIN plans pl ON pl.id=p.intent_id AND p.intent_type='plan_renewal' ORDER BY p.id DESC LIMIT 500`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	payments := []Payment{}
	for rows.Next() {
		var p Payment
		var intentID sql.NullInt64
		var created, updated sql.NullTime
		if err := rows.Scan(&p.ID, &p.Username, &p.Amount, &p.Method, &p.Status, &p.IntentType, &intentID, &p.IntentLabel, &created, &updated); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if p.IntentType == "" {
			p.IntentType = "wallet_topup"
		}
		if intentID.Valid {
			p.IntentID = &intentID.Int64
		}
		if created.Valid {
			p.CreatedAt = created.Time.Format(time.RFC3339)
		}
		if updated.Valid {
			p.UpdatedAt = updated.Time.Format(time.RFC3339)
		}
		payments = append(payments, p)
	}
	writeJSON(w, map[string]any{"ok": true, "payments": payments})
}

func (s *Server) createManualPayment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username    string  `json:"username"`
		Amount      float64 `json:"amount"`
		Method      string  `json:"method"`
		Receipt     string  `json:"receipt"`
		Description string  `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" || in.Amount <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "username_amount_required"})
		return
	}
	if in.Method == "" {
		in.Method = "manual"
	}
	customerID := sql.NullInt64{}
	_ = s.DB.QueryRow(`SELECT id FROM customers WHERE username=? AND deleted_at IS NULL LIMIT 1`, in.Username).Scan(&customerID.Int64)
	if customerID.Int64 > 0 {
		customerID.Valid = true
	}
	res, err := s.DB.Exec(`INSERT INTO payments(customer_id,username,amount,method,receipt,status,intent_type,admin_note) VALUES(?,?,?,?,?,'approved','wallet_topup',?)`, nullableInt(customerID), in.Username, in.Amount, in.Method, in.Receipt, in.Description)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	paymentID, _ := res.LastInsertId()
	if err := s.applyWalletChangeRef(in.Username, in.Amount, "topup", fmt.Sprintf("payment #%d approved", paymentID), "admin", "payment", &paymentID); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "payment.created", "payment", strconv.FormatInt(paymentID, 10), nil, map[string]any{"username": in.Username, "amount": in.Amount}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": paymentID})
}

func (s *Server) setPaymentStatus(w http.ResponseWriter, r *http.Request, id int64, status string) {
	if status != "approved" && status != "rejected" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_status"})
		return
	}
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	var username, oldStatus, method, intentType string
	var amount float64
	var intentID sql.NullInt64
	if err := tx.QueryRow(`SELECT username,amount,status,method,COALESCE(intent_type,'wallet_topup'),intent_id FROM payments WHERE id=? LIMIT 1 FOR UPDATE`, id).Scan(&username, &amount, &oldStatus, &method, &intentType, &intentID); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if intentType == "" {
		intentType = "wallet_topup"
	}
	if oldStatus != status {
		if _, err := tx.Exec(`UPDATE payments SET status=? WHERE id=?`, status, id); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if method != "wallet" && intentType != "reseller_topup" {
		if err := s.syncPaymentWalletStateTx(tx, id, username, amount, status); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if status == "approved" && intentType == "reseller_topup" {
		_, err = tx.Exec(`UPDATE admins SET credit = credit + ? WHERE username=?`, amount, username)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		desc := fmt.Sprintf("Manual Top-up approved (Payment #%d): +%.2f IRT", id, amount)
		_, err = tx.Exec(`INSERT INTO reseller_transactions(reseller_username, amount, type, description, actor) VALUES(?,?, 'allocation', ?, ?)`, username, amount, desc, "admin")
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if status == "approved" && intentType == "plan_renewal" && intentID.Valid {
		if err := s.applyPlanIntentTx(tx, username, intentID.Int64, id, "admin"); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Auto-provision WireGuard peer when plan renewal payment is approved
	if status == "approved" && intentType == "plan_renewal" && intentID.Valid {
		var custID int64
		if s.DB.QueryRow(`SELECT id FROM customers WHERE username=? AND deleted_at IS NULL LIMIT 1`, username).Scan(&custID) == nil {
			s.autoProvisionWireGuardPeer(custID)
		}
	}
	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "payment.status_"+status, "payment", strconv.FormatInt(id, 10), map[string]any{"old_status": oldStatus}, map[string]any{"new_status": status}, clientIP(r))
	severity := "info"
	if status == "rejected" {
		severity = "warning"
	}
	s.createEvent("payment", severity, fmt.Sprintf("Payment %s #%d", status, id), fmt.Sprintf("Payment #%d for %s was %s", id, username, status), actor, username)
	writeJSON(w, map[string]any{"ok": true, "intent_type": intentType})
}

func (s *Server) applyPlanIntentTx(tx *sql.Tx, username string, planID, paymentID int64, actor string) error {
	var existing int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM wallet_transactions WHERE reference_type='payment' AND reference_id=? AND type='purchase'`, paymentID).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}
	var customerID int64
	if err := tx.QueryRow(`SELECT id FROM customers WHERE username=? AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID); err != nil {
		return err
	}
	var plan Plan
	var active int
	var created sql.NullTime
	if err := tx.QueryRow(`SELECT id,name,data_gb,speed_mbps,duration_days,price,is_active,sort_order,created_at FROM plans WHERE id=? LIMIT 1`, planID).Scan(&plan.ID, &plan.Name, &plan.DataGB, &plan.SpeedMbps, &plan.DurationDays, &plan.Price, &active, &plan.SortOrder, &created); err != nil {
		return err
	}
	if active != 1 {
		return fmt.Errorf("plan_inactive")
	}
	var walletCredit float64
	_ = tx.QueryRow(`SELECT COALESCE(credit,0) FROM wallets WHERE username=?`, username).Scan(&walletCredit)
	if plan.Price > 0 && walletCredit+0.0001 < plan.Price {
		return fmt.Errorf("insufficient_wallet")
	}
	if _, err := tx.Exec(`UPDATE customers SET plan_id=?,status='active' WHERE id=? AND deleted_at IS NULL`, plan.ID, customerID); err != nil {
		return err
	}
	_, _ = tx.Exec(`DELETE FROM radcheck WHERE username=? AND attribute='Max-Data'`, username)
	if plan.DataGB > 0 {
		bytes := int64(math.Round(plan.DataGB * 1024 * 1024 * 1024))
		if _, err := tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES(?,'Max-Data',':=',?)`, username, bytes); err != nil {
			return err
		}
	}
	_, _ = tx.Exec(`DELETE FROM radreply WHERE username=? AND attribute='Mikrotik-Rate-Limit'`, username)
	if plan.SpeedMbps > 0 {
		if _, err := tx.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES(?,'Mikrotik-Rate-Limit',':=',?)`, username, speedLimitValue(plan.SpeedMbps)); err != nil {
			return err
		}
	}
	var expires any
	if plan.DurationDays > 0 {
		expires = time.Now().AddDate(0, 0, plan.DurationDays)
	}
	if _, err := tx.Exec(`INSERT INTO subscriptions(customer_id,username,plan_id,expires_at,paid_amount) VALUES(?,?,?,?,?)`, customerID, username, plan.ID, expires, plan.Price); err != nil {
		return err
	}
	if plan.Price > 0 {
		desc := "plan activated: " + plan.Name
		if _, err := tx.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES(?,?,?) ON DUPLICATE KEY UPDATE credit=credit+VALUES(credit), customer_id=COALESCE(VALUES(customer_id),customer_id)`, customerID, username, -plan.Price); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES(?,?,?,?,?,?,?,?)`, customerID, username, -plan.Price, "purchase", desc, actor, "payment", paymentID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) syncPaymentWalletStateTx(tx *sql.Tx, paymentID int64, username string, amount float64, status string) error {
	desired := 0.0
	if status == "approved" {
		desired = amount
	}
	like := fmt.Sprintf("payment #%d %%", paymentID)
	var current float64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM wallet_transactions WHERE username=? AND type <> 'purchase' AND (reference_type='payment' AND reference_id=? OR description LIKE ?)`, username, paymentID, like).Scan(&current); err != nil {
		return err
	}
	delta := desired - current
	if math.Abs(delta) < 0.0001 {
		return nil
	}
	var customerID sql.NullInt64
	_ = tx.QueryRow(`SELECT id FROM customers WHERE username=? AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
	if _, err := tx.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES(?,?,?) ON DUPLICATE KEY UPDATE credit=credit+VALUES(credit), customer_id=COALESCE(VALUES(customer_id),customer_id)`, nullableInt(customerID), username, delta); err != nil {
		return err
	}
	kind := "adjustment"
	desc := fmt.Sprintf("payment #%d wallet reconciliation: %s", paymentID, status)
	if delta > 0 && status == "approved" {
		kind = "topup"
		desc = fmt.Sprintf("payment #%d approved", paymentID)
	} else if delta < 0 && status != "approved" {
		desc = fmt.Sprintf("payment #%d approval reversed: %s", paymentID, status)
	}
	_, err := tx.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES(?,?,?,?,?,?,?,?)`, nullableInt(customerID), username, delta, kind, desc, "admin", "payment", paymentID)
	return err
}

func (s *Server) applyWalletChange(username string, amount float64, kind, description, actor string) error {
	return s.applyWalletChangeRef(username, amount, kind, description, actor, "", nil)
}

func (s *Server) applyWalletChangeRef(username string, amount float64, kind, description, actor, referenceType string, referenceID *int64) error {
	var customerID sql.NullInt64
	_ = s.DB.QueryRow(`SELECT id FROM customers WHERE username=? AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
	_, err := s.DB.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES(?,?,?) ON DUPLICATE KEY UPDATE credit=credit+VALUES(credit), customer_id=COALESCE(VALUES(customer_id),customer_id)`, nullableInt(customerID), username, amount)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES(?,?,?,?,?,?,?,?)`, nullableInt(customerID), username, amount, kind, description, actor, referenceType, nullableInt64Ptr(referenceID))
	return err
}

func (s *Server) setWalletBalance(username string, balance float64, description, actor string) error {
	var customerID sql.NullInt64
	_ = s.DB.QueryRow(`SELECT id FROM customers WHERE username=? AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
	var current float64
	_ = s.DB.QueryRow(`SELECT COALESCE(credit,0) FROM wallets WHERE username=?`, username).Scan(&current)
	delta := balance - current
	_, err := s.DB.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES(?,?,?) ON DUPLICATE KEY UPDATE credit=VALUES(credit), customer_id=COALESCE(VALUES(customer_id),customer_id)`, nullableInt(customerID), username, balance)
	if err != nil {
		return err
	}
	if math.Abs(delta) < 0.0001 {
		return nil
	}
	if description == "" {
		description = fmt.Sprintf("set wallet balance to %.2f", balance)
	}
	_, err = s.DB.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES(?,?,?,?,?,?,?,NULL)`, nullableInt(customerID), username, delta, "adjustment", description, actor, "manual")
	return err
}

func nullableInt(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}

func nullableInt64Ptr(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func pathUsername(urlPath, prefix string) (string, string, bool) {
	rest := strings.Trim(strings.TrimPrefix(urlPath, prefix), "/")
	if rest == "" || strings.Contains(rest, "..") {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	username := strings.TrimSpace(parts[0])
	if username == "" {
		return "", "", false
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	return username, action, true
}

func (s *Server) getCustomerUsage(w http.ResponseWriter, id int64) {
	username, err := s.customerUsername(id)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	usage, err := s.usageForUsername(username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "usage": usage})
}

func (s *Server) portalUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	usage, err := s.usageForUsername(username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "usage": usage})
}

func (s *Server) usageForUsername(username string) (UsageSummary, error) {
	usage := UsageSummary{Sessions: []UsageSession{}}
	var lastConnected, lastDisconnected sql.NullTime
	if err := s.DB.QueryRow(`SELECT COALESCE(SUM(COALESCE(acctinputoctets,0)),0),COALESCE(SUM(COALESCE(acctoutputoctets,0)),0),COALESCE(SUM(CASE WHEN acctstoptime IS NULL THEN 1 ELSE 0 END),0),MAX(acctstarttime),MAX(acctstoptime) FROM radacct WHERE username=?`, username).Scan(&usage.TotalInputBytes, &usage.TotalOutputBytes, &usage.ActiveSessions, &lastConnected, &lastDisconnected); err != nil {
		return usage, err
	}
	usage.TotalUsageBytes = usage.TotalInputBytes + usage.TotalOutputBytes
	usage.Online = usage.ActiveSessions > 0
	if lastConnected.Valid {
		usage.LastConnectedAt = lastConnected.Time.Format(time.RFC3339)
	}
	if lastDisconnected.Valid {
		usage.LastDisconnectedAt = lastDisconnected.Time.Format(time.RFC3339)
	}
	var maxData string
	if err := s.DB.QueryRow(`SELECT value FROM radcheck WHERE username=? AND attribute='Max-Data' ORDER BY id DESC LIMIT 1`, username).Scan(&maxData); err == nil {
		usage.MaxDataBytes, _ = strconv.ParseInt(strings.TrimSpace(maxData), 10, 64)
	}
	if usage.MaxDataBytes > 0 {
		remaining := usage.MaxDataBytes - usage.TotalUsageBytes
		if remaining < 0 {
			remaining = 0
		}
		usage.RemainingBytes = &remaining
	}
	rows, err := s.DB.Query(`SELECT radacctid,username,acctstarttime,acctupdatetime,acctstoptime,COALESCE(acctsessiontime,TIMESTAMPDIFF(SECOND,acctstarttime,COALESCE(acctstoptime,NOW())),0),COALESCE(acctinputoctets,0),COALESCE(acctoutputoctets,0),framedipaddress,callingstationid,acctterminatecause FROM radacct WHERE username=? ORDER BY radacctid DESC LIMIT 50`, username)
	if err != nil {
		return usage, err
	}
	defer rows.Close()
	for rows.Next() {
		var session UsageSession
		var start, update, stop sql.NullTime
		var seconds sql.NullInt64
		if err := rows.Scan(&session.ID, &session.Username, &start, &update, &stop, &seconds, &session.InputBytes, &session.OutputBytes, &session.FramedIP, &session.CallingStationID, &session.TerminateCause); err != nil {
			return usage, err
		}
		if start.Valid {
			session.StartTime = start.Time.Format(time.RFC3339)
		}
		if update.Valid {
			session.UpdateTime = update.Time.Format(time.RFC3339)
		}
		if stop.Valid {
			session.StopTime = stop.Time.Format(time.RFC3339)
		}
		if seconds.Valid {
			session.SessionSeconds = seconds.Int64
		}
		session.TotalBytes = session.InputBytes + session.OutputBytes
		session.Online = !stop.Valid
		usage.Sessions = append(usage.Sessions, session)
	}
	return usage, rows.Err()
}

func (s *Server) portalNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	_, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	rows, err := s.DB.Query(`SELECT id,name,COALESCE(domain,''),public_ip,status FROM nodes WHERE status <> 'disabled' ORDER BY CASE status WHEN 'online' THEN 0 WHEN 'stale' THEN 1 ELSE 2 END, id ASC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	type NodeInfo struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		Domain   string `json:"domain"`
		PublicIP string `json:"public_ip"`
		Status   string `json:"status"`
	}
	out := []NodeInfo{}
	for rows.Next() {
		var n NodeInfo
		if err := rows.Scan(&n.ID, &n.Name, &n.Domain, &n.PublicIP, &n.Status); err == nil {
			out = append(out, n)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "nodes": out})
}

func (s *Server) openVPNEndpointNode(r *http.Request, nodeID int64) (host string, port int, proto string, nodeName string) {
	port = 1194
	proto = "udp"
	_ = s.DB.QueryRow(`SELECT openvpn_port,openvpn_protocol FROM vpn_core_settings WHERE id=1`).Scan(&port, &proto)

	// Priority 0: Global VPN domain (static config — same domain for all nodes, DNS-based failover)
	var globalVPNDomain string
	_ = s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key='vpn_domain'`).Scan(&globalVPNDomain)
	globalVPNDomain = strings.TrimSpace(globalVPNDomain)
	if globalVPNDomain != "" {
		host = globalVPNDomain
	}

	if nodeID > 0 {
		// Get node name regardless
		var domain, publicIP string
		_ = s.DB.QueryRow(`SELECT name,COALESCE(domain,''),public_ip FROM nodes WHERE id=? AND status <> 'disabled' LIMIT 1`, nodeID).Scan(&nodeName, &domain, &publicIP)

		if host == "" {
			// Priority 1: Check for active failover domain pointing to this node
			var failoverDomain string
			if err := s.DB.QueryRow(
				`SELECT domain FROM failover_domains WHERE current_node_id = ? AND is_active = 1 LIMIT 1`, nodeID,
			).Scan(&failoverDomain); err == nil && strings.TrimSpace(failoverDomain) != "" {
				host = strings.TrimSpace(failoverDomain)
			}
		}

		if host == "" {
			// Priority 2 & 3: Node's domain field, then public_ip
			var domain2, publicIP2 string
			_ = s.DB.QueryRow(`SELECT COALESCE(domain,''),public_ip FROM nodes WHERE id=? LIMIT 1`, nodeID).Scan(&domain2, &publicIP2)
			host = strings.TrimSpace(domain2)
			if host == "" {
				host = strings.TrimSpace(publicIP2)
			}
		}
	}
	if host == "" {
		var domain, publicIP string
		_ = s.DB.QueryRow(`SELECT name,COALESCE(domain,''),public_ip FROM nodes WHERE status <> 'disabled' ORDER BY CASE status WHEN 'online' THEN 0 WHEN 'stale' THEN 1 ELSE 2 END, id ASC LIMIT 1`).Scan(&nodeName, &domain, &publicIP)
		host = strings.TrimSpace(domain)
		if host == "" {
			host = strings.TrimSpace(publicIP)
		}
	}
	if host == "" {
		host = r.Host
		if strings.Contains(host, ":") {
			host = strings.Split(host, ":")[0]
		}
	}
	if proto == "" {
		proto = "udp"
	}
	if port <= 0 {
		port = 1194
	}
	return host, port, proto, nodeName
}

func (s *Server) portalProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	nodeID, _ := strconv.ParseInt(r.URL.Query().Get("node_id"), 10, 64)
	host, port, _, nodeName := s.openVPNEndpointNode(r, nodeID)
	var psk string
	_ = s.DB.QueryRow(`SELECT COALESCE(ipsec_psk,'') FROM vpn_core_settings WHERE id=1`).Scan(&psk)
	psk = strings.TrimSpace(psk)
	passwordlessAvailable := s.canUsePasswordless(username)

	// Build filenames:
	// - OpenVPN with auth: generic config, use node name only (e.g. "🇩🇪Germany.ovpn")
	// - Passwordless / mobileconfig: per-user, use "username-nodename" (e.g. "john-🇩🇪Germany.ovpn")
	nodeBase := safeFilename(nodeName)
	if nodeBase == "" {
		nodeBase = "vpn"
	}
	userBase := safeFilename(username)
	genericFilenameUDP := nodeBase + ".ovpn"
	genericFilenameTCP := nodeBase + "-TCP.ovpn"
	perUserOvpn := userBase + "-" + nodeBase + ".ovpn"
	perUserL2TP := userBase + "-" + nodeBase + ".mobileconfig"
	perUserIKEv2 := userBase + "-" + nodeBase + "-ikev2.mobileconfig"

	// Get user's preferred node
	var preferredNodeID int64
	_ = s.DB.QueryRow(`SELECT COALESCE(preferred_node_id, 0) FROM customers WHERE username=? AND deleted_at IS NULL`, username).Scan(&preferredNodeID)

	writeJSON(w, map[string]any{
		"ok":                     true,
		"passwordless_available": passwordlessAvailable,
		"preferred_node_id":      preferredNodeID,
		"profiles": []map[string]any{
			{
				"type":                  "openvpn-udp",
				"name":                  "OpenVPN UDP — " + nodeName,
				"filename":              genericFilenameUDP,
				"filename_passwordless": perUserOvpn,
				"available":             host != "",
				"remote":                host,
				"port":                  port,
				"protocol":              "udp",
				"node":                  nodeName,
				"download":              fmt.Sprintf("/api/portal/profiles/openvpn.ovpn?node_id=%d", nodeID),
				"description":           "Fast, best for gaming. Direct connection with failover.",
			},
			{
				"type":        "openvpn-tcp",
				"name":        "OpenVPN TCP — " + nodeName,
				"filename":    genericFilenameTCP,
				"available":   host != "",
				"remote":      host,
				"port":        443,
				"protocol":    "tcp",
				"node":        nodeName,
				"download":    fmt.Sprintf("/api/portal/profiles/openvpn-tcp.ovpn?node_id=%d", nodeID),
				"description": "Stable, supports node selection. Works behind firewalls.",
			},
			{
				"type":      "l2tp",
				"name":      "L2TP/IPSec — " + nodeName,
				"filename":  perUserL2TP,
				"available": host != "" && psk != "",
				"remote":    host,
				"port":      1701,
				"protocol":  "l2tp",
				"node":      nodeName,
				"download":  fmt.Sprintf("/api/portal/profiles/l2tp.mobileconfig?node_id=%d", nodeID),
			},
			{
				"type":      "ikev2",
				"name":      "IKEv2 — " + nodeName,
				"filename":  perUserIKEv2,
				"available": host != "",
				"remote":    host,
				"port":      500,
				"protocol":  "ikev2",
				"node":      nodeName,
				"download":  fmt.Sprintf("/api/portal/profiles/ikev2.mobileconfig?node_id=%d", nodeID),
			},
		},
	})
}

func (s *Server) portalProfileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/openvpn-tcp.ovpn"):
		nodeID, _ := strconv.ParseInt(r.URL.Query().Get("node_id"), 10, 64)
		profile := s.openVPNProfileTCP(username, r, nodeID)
		_, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
		nodeBase := safeFilename(nodeName)
		if nodeBase == "" {
			nodeBase = "vpn"
		}
		filename := nodeBase + "-TCP.ovpn"
		w.Header().Set("Content-Type", "application/x-openvpn-profile; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+url.PathEscape(filename))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(profile))
	case strings.HasSuffix(path, "/openvpn.ovpn"):
		nodeID, _ := strconv.ParseInt(r.URL.Query().Get("node_id"), 10, 64)
		passwordless := r.URL.Query().Get("passwordless") == "true"
		var profile string
		if passwordless && s.canUsePasswordless(username) {
			profile = s.openVPNProfilePasswordless(username, r, nodeID)
		} else {
			profile = s.openVPNProfile(username, r, nodeID)
		}
		_, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
		nodeBase := safeFilename(nodeName)
		if nodeBase == "" {
			nodeBase = "vpn"
		}
		// Passwordless configs are per-user; standard OpenVPN is generic (node name only)
		var filename string
		if passwordless {
			filename = safeFilename(username) + "-" + nodeBase + ".ovpn"
		} else {
			filename = nodeBase + ".ovpn"
		}
		w.Header().Set("Content-Type", "application/x-openvpn-profile; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+url.PathEscape(filename))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(profile))
	case strings.HasSuffix(path, "/l2tp.mobileconfig"):
		nodeID, _ := strconv.ParseInt(r.URL.Query().Get("node_id"), 10, 64)
		profile := s.l2tpMobileConfig(username, r, nodeID)
		_, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
		nodeBase := safeFilename(nodeName)
		if nodeBase == "" {
			nodeBase = "vpn"
		}
		// mobileconfig embeds username — always per-user
		filename := safeFilename(username) + "-" + nodeBase + ".mobileconfig"
		w.Header().Set("Content-Type", "application/x-apple-aspen-config; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+url.PathEscape(filename))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(profile))
	case strings.HasSuffix(path, "/ikev2.mobileconfig"):
		nodeID, _ := strconv.ParseInt(r.URL.Query().Get("node_id"), 10, 64)
		profile := s.ikev2MobileConfig(username, r, nodeID)
		_, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
		nodeBase := safeFilename(nodeName)
		if nodeBase == "" {
			nodeBase = "vpn"
		}
		// mobileconfig embeds username — always per-user
		filename := safeFilename(username) + "-" + nodeBase + "-ikev2.mobileconfig"
		w.Header().Set("Content-Type", "application/x-apple-aspen-config; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+url.PathEscape(filename))
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(profile))
	default:
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
	}
}

func (s *Server) openVPNEndpoint(r *http.Request) (host string, port int, proto string, nodeName string) {
	port = 1194
	proto = "udp"
	_ = s.DB.QueryRow(`SELECT openvpn_port,openvpn_protocol FROM vpn_core_settings WHERE id=1`).Scan(&port, &proto)
	var domain, publicIP string
	_ = s.DB.QueryRow(`SELECT name,COALESCE(domain,''),public_ip FROM nodes WHERE status <> 'disabled' ORDER BY CASE status WHEN 'online' THEN 0 WHEN 'stale' THEN 1 ELSE 2 END, id ASC LIMIT 1`).Scan(&nodeName, &domain, &publicIP)
	host = strings.TrimSpace(domain)
	if host == "" {
		host = strings.TrimSpace(publicIP)
	}
	if host == "" {
		host = r.Host
		if strings.Contains(host, ":") {
			host = strings.Split(host, ":")[0]
		}
	}
	if proto == "" {
		proto = "udp"
	}
	if port <= 0 {
		port = 1194
	}
	return host, port, proto, nodeName
}

func (s *Server) openVPNProfile(username string, r *http.Request, nodeID int64) string {
	return s.openVPNProfileWithAuth(username, r, nodeID, true)
}

func (s *Server) openVPNProfilePasswordless(username string, r *http.Request, nodeID int64) string {
	return s.openVPNProfileWithAuth(username, r, nodeID, false)
}

// openVPNProfileTCP generates a TCP-based OpenVPN config on port 443.
// Uses the user's preferred node as primary, with backup nodes as fallback.
func (s *Server) openVPNProfileTCP(username string, r *http.Request, nodeID int64) string {
	host, _, _, nodeName := s.openVPNEndpointNode(r, nodeID)
	if nodeName == "" {
		nodeName = host
	}
	caBlock := inlineOpenVPNBlock("ca", getenvFirst("PANEL_OPENVPN_CA_FILE", "/etc/openvpn/server/ca.crt", "/etc/openvpn/easy-rsa/pki/ca.crt"))
	tlsCryptBlock := inlineOpenVPNBlock("tls-crypt", getenvFirst("PANEL_OPENVPN_TLS_CRYPT_FILE", "/etc/openvpn/server/tc.key", "/etc/openvpn/server/tls-crypt.key", "/etc/openvpn/server/ta.key"))

	// Build remote lines for TCP: preferred node first, then backups
	remoteLines := fmt.Sprintf("remote %s 443 tcp", host)

	// Get user's preferred node — put it first if different from default
	var preferredNodeID int64
	_ = s.DB.QueryRow(`SELECT COALESCE(preferred_node_id, 0) FROM customers WHERE username=? AND deleted_at IS NULL`, username).Scan(&preferredNodeID)
	if preferredNodeID > 0 && preferredNodeID != nodeID {
		var prefDomain, prefIP string
		if s.DB.QueryRow(`SELECT COALESCE(domain,''), public_ip FROM nodes WHERE id=? AND status <> 'disabled'`, preferredNodeID).Scan(&prefDomain, &prefIP) == nil {
			prefHost := strings.TrimSpace(prefDomain)
			if prefHost == "" {
				prefHost = strings.TrimSpace(prefIP)
			}
			if prefHost != "" && prefHost != host {
				// Preferred node goes first
				remoteLines = fmt.Sprintf("remote %s 443 tcp\nremote %s 443 tcp", prefHost, host)
			}
		}
	}

	// Add other active nodes as backup
	rows, err := s.DB.Query(`
		SELECT COALESCE(n.domain,''), n.public_ip
		FROM nodes n
		JOIN node_vpn_configs c ON c.node_id = n.id AND c.protocol = 'openvpn' AND c.enabled = 1
		WHERE n.status <> 'disabled' AND n.id <> ? AND n.id <> ?
		ORDER BY n.id`, nodeID, preferredNodeID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var domain, ip string
			if rows.Scan(&domain, &ip) == nil {
				backupHost := strings.TrimSpace(domain)
				if backupHost == "" {
					backupHost = strings.TrimSpace(ip)
				}
				if backupHost != "" && backupHost != host {
					remoteLines += fmt.Sprintf("\nremote %s 443 tcp", backupHost)
				}
			}
		}
	}

	return fmt.Sprintf(`# KorisPanel OpenVPN TCP Profile
# User: %s
# Node: %s
# Generated: %s
# TCP mode — supports node selection via portal
client
dev tun
%s
resolv-retry infinite
nobind
persist-key
persist-tun
remote-cert-tls server
setenv CLIENT_CERT 0
auth-user-pass
auth-nocache
auth SHA256
data-ciphers AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305
data-ciphers-fallback AES-256-GCM
verb 3
pull
%s%s`, username, nodeName, time.Now().UTC().Format(time.RFC3339), remoteLines, caBlock, tlsCryptBlock)
}

// canUsePasswordless checks if a customer is allowed to generate passwordless configs.
// Requires: global setting enabled AND customer's plan allows passwordless.
func (s *Server) canUsePasswordless(username string) bool {
	// Check global setting
	var enabled string
	_ = s.DB.QueryRow(`SELECT setting_value FROM panel_settings WHERE setting_key='passwordless_configs_enabled'`).Scan(&enabled)
	if enabled != "true" {
		return false
	}
	// Check per-plan setting
	var allowPasswordless int
	err := s.DB.QueryRow(`SELECT COALESCE(p.allow_passwordless, 0) FROM customers c JOIN plans p ON p.id = c.plan_id WHERE c.username = ? AND c.deleted_at IS NULL LIMIT 1`, username).Scan(&allowPasswordless)
	if err != nil {
		return false
	}
	return allowPasswordless == 1
}

func (s *Server) openVPNProfileWithAuth(username string, r *http.Request, nodeID int64, withAuth bool) string {
	host, port, proto, nodeName := s.openVPNEndpointNode(r, nodeID)
	if nodeName == "" {
		nodeName = host
	}
	caBlock := inlineOpenVPNBlock("ca", getenvFirst("PANEL_OPENVPN_CA_FILE", "/etc/openvpn/server/ca.crt", "/etc/openvpn/easy-rsa/pki/ca.crt"))
	tlsCryptBlock := inlineOpenVPNBlock("tls-crypt", getenvFirst("PANEL_OPENVPN_TLS_CRYPT_FILE", "/etc/openvpn/server/tc.key", "/etc/openvpn/server/tls-crypt.key", "/etc/openvpn/server/ta.key"))

	authLine := "auth-user-pass\n"
	authComment := "# Login with your VPN username/password when OpenVPN asks for credentials."
	if !withAuth {
		authLine = ""
		authComment = "# Passwordless mode — no credentials required."
	}

	// Build remote lines: primary + backup nodes for failover
	remoteLines := fmt.Sprintf("remote %s %d %s", host, port, proto)

	// Add backup remotes: all other active nodes with OpenVPN enabled
	rows, err := s.DB.Query(`
		SELECT COALESCE(n.domain,''), n.public_ip
		FROM nodes n
		JOIN node_vpn_configs c ON c.node_id = n.id AND c.protocol = 'openvpn' AND c.enabled = 1
		WHERE n.status <> 'disabled' AND n.id <> ?
		ORDER BY n.id`, nodeID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var domain, ip string
			if rows.Scan(&domain, &ip) == nil {
				backupHost := strings.TrimSpace(domain)
				if backupHost == "" {
					backupHost = strings.TrimSpace(ip)
				}
				if backupHost != "" && backupHost != host {
					remoteLines += fmt.Sprintf("\nremote %s %d %s", backupHost, port, proto)
				}
			}
		}
	}

	// Add remote-random only if explicitly configured (disabled by default)
	// Load balancing is handled by smart proxy, not client-side randomization

	return fmt.Sprintf(`# KorisPanel generated OpenVPN profile
# User: %s
# Node: %s
# Generated: %s
%s
client
dev tun
%s
resolv-retry infinite
nobind
persist-key
persist-tun
remote-cert-tls server
setenv CLIENT_CERT 0
%sauth-nocache
auth SHA256
data-ciphers AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305
data-ciphers-fallback AES-256-GCM
explicit-exit-notify 1
verb 3
pull
%s%s`, username, nodeName, time.Now().UTC().Format(time.RFC3339), authComment, remoteLines, authLine, caBlock, tlsCryptBlock)
}

func getenvFirst(envName string, paths ...string) string {
	if v := strings.TrimSpace(os.Getenv(envName)); v != "" {
		return v
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func inlineOpenVPNBlock(name, filePath string) string {
	if filePath == "" {
		return ""
	}
	b, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(b))
	if content == "" {
		return ""
	}
	return fmt.Sprintf("\n<%s>\n%s\n</%s>\n", name, content, name)
}

func safeFilename(s string) string {
	return strings.NewReplacer("/", "_", "\\", "_", " ", "_", "\x00", "_").Replace(s)
}

func (s *Server) l2tpMobileConfig(username string, r *http.Request, nodeID int64) string {
	host, _, _, _ := s.openVPNEndpointNode(r, nodeID)
	if host == "" {
		host = r.Host
	}
	var psk string
	_ = s.DB.QueryRow(`SELECT COALESCE(ipsec_psk,'') FROM vpn_core_settings WHERE id=1`).Scan(&psk)
	psk = strings.TrimSpace(psk)
	uuidPayload := strings.ToLower(auth.RandomToken(8) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(12))
	uuidProfile := strings.ToLower(auth.RandomToken(8) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(12))
	pskData := base64.StdEncoding.EncodeToString([]byte(psk))
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadDescription</key>
			<string>Configures L2TP VPN</string>
			<key>PayloadDisplayName</key>
			<string>Koris L2TP</string>
			<key>PayloadIdentifier</key>
			<string>koris.vpn.l2tp.%s</string>
			<key>PayloadType</key>
			<string>com.apple.vpn.managed</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>UserDefinedName</key>
			<string>Koris L2TP</string>
			<key>VPNType</key>
			<string>L2TP</string>
			<key>IPv4</key>
			<dict>
				<key>OverridePrimary</key>
				<integer>1</integer>
			</dict>
			<key>PPP</key>
			<dict>
				<key>AuthName</key>
				<string>%s</string>
				<key>CommRemoteAddress</key>
				<string>%s</string>
				<key>OnDemandEnabled</key>
				<integer>0</integer>
			</dict>
			<key>IPSec</key>
			<dict>
				<key>AuthenticationMethod</key>
				<string>SharedSecret</string>
				<key>SharedSecret</key>
				<data>%s</data>
			</dict>
		</dict>
	</array>
	<key>PayloadDisplayName</key>
	<string>Koris L2TP</string>
	<key>PayloadIdentifier</key>
	<string>koris.vpn.l2tp.profile.%s</string>
	<key>PayloadRemovalDisallowed</key>
	<false/>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>%s</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>`, username, uuidPayload, username, host, pskData, username, uuidProfile)
}

func (s *Server) ikev2MobileConfig(username string, r *http.Request, nodeID int64) string {
	host, _, _, _ := s.openVPNEndpointNode(r, nodeID)
	if host == "" {
		host = r.Host
	}
	uuidPayload := strings.ToLower(auth.RandomToken(8) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(12))
	uuidProfile := strings.ToLower(auth.RandomToken(8) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(4) + "-" + auth.RandomToken(12))
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadDescription</key>
			<string>Configures IKEv2 VPN</string>
			<key>PayloadDisplayName</key>
			<string>Koris IKEv2</string>
			<key>PayloadIdentifier</key>
			<string>koris.vpn.ikev2.%s</string>
			<key>PayloadType</key>
			<string>com.apple.vpn.managed</string>
			<key>PayloadUUID</key>
			<string>%s</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
			<key>UserDefinedName</key>
			<string>Koris IKEv2</string>
			<key>VPNType</key>
			<string>IKEv2</string>
			<key>IPv4</key>
			<dict>
				<key>OverridePrimary</key>
				<integer>1</integer>
			</dict>
			<key>AuthenticationMethod</key>
			<string>UserName</string>
			<key>AuthName</key>
			<string>%s</string>
			<key>ExtendedAuthEnabled</key>
			<true/>
			<key>ServerAddress</key>
			<string>%s</string>
			<key>RemoteAddress</key>
			<string>%s</string>
			<key>IKEv2</key>
			<dict>
				<key>AuthenticationMethod</key>
				<string>UserName</string>
				<key>AuthName</key>
				<string>%s</string>
				<key>ExtendedAuthEnabled</key>
				<true/>
				<key>RemoteAddress</key>
				<string>%s</string>
				<key>ServerAddress</key>
				<string>%s</string>
				<key>DeadPeerDetectionRate</key>
				<string>Medium</string>
				<key>DisableMOBIKE</key>
				<integer>0</integer>
				<key>DisableRedirect</key>
				<integer>0</integer>
				<key>EnableCertificateRevocationCheck</key>
				<integer>0</integer>
				<key>EnablePFS</key>
				<integer>0</integer>
				<key>ChildSecurityAssociationParameters</key>
				<dict>
					<key>EncryptionAlgorithm</key>
					<string>AES-256-GCM</string>
					<key>IntegrityAlgorithm</key>
					<string>SHA2-384</string>
					<key>DiffieHellmanGroup</key>
					<integer>20</integer>
					<key>LifeTimeInMinutes</key>
					<integer>1440</integer>
				</dict>
				<key>IKESecurityAssociationParameters</key>
				<dict>
					<key>EncryptionAlgorithm</key>
					<string>AES-256-GCM</string>
					<key>IntegrityAlgorithm</key>
					<string>SHA2-384</string>
					<key>DiffieHellmanGroup</key>
					<integer>20</integer>
					<key>LifeTimeInMinutes</key>
					<integer>1440</integer>
				</dict>
			</dict>
		</dict>
	</array>
	<key>PayloadDisplayName</key>
	<string>Koris IKEv2</string>
	<key>PayloadIdentifier</key>
	<string>koris.vpn.ikev2.profile.%s</string>
	<key>PayloadRemovalDisallowed</key>
	<false/>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>%s</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>`, username, uuidPayload, username, host, host, username, host, host, username, uuidProfile)
}

func (s *Server) portalPlans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`SELECT id,name,data_gb,speed_mbps,duration_days,price,billing_type,price_per_gb,price_per_day,disconnect_on_zero,allow_passwordless,is_active,sort_order,created_at FROM plans WHERE is_active=1 ORDER BY sort_order ASC, id DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	plans := []Plan{}
	for rows.Next() {
		plan, err := scanPlan(rows)
		if err == nil {
			plans = append(plans, plan)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "plans": plans})
}

func (s *Server) portalRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	var in struct {
		PlanID int64 `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.PlanID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "plan_required"})
		return
	}

	var customerID int64
	if err := s.DB.QueryRow(`SELECT id FROM customers WHERE username=? AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "customer_not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	plan, err := scanPlan(s.DB.QueryRow(`SELECT id,name,data_gb,speed_mbps,duration_days,price,billing_type,price_per_gb,price_per_day,disconnect_on_zero,allow_passwordless,is_active,sort_order,created_at FROM plans WHERE id=? LIMIT 1`, in.PlanID))
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "plan_not_found"})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if !plan.IsActive {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "plan_inactive"})
		return
	}

	var walletCredit float64
	_ = s.DB.QueryRow(`SELECT COALESCE(credit,0) FROM wallets WHERE username=?`, username).Scan(&walletCredit)
	if plan.Price > 0 && walletCredit+0.0001 < plan.Price {
		required := plan.Price - walletCredit
		if required < plan.Price && required < 1 {
			required = plan.Price
		}
		res, err := s.DB.Exec(`INSERT INTO payments(customer_id,username,amount,method,receipt,status,intent_type,intent_id,metadata_json,admin_note) VALUES(?,?,?,'portal_topup','','pending','plan_renewal',?,JSON_OBJECT('plan_name',?,'plan_price',?,'wallet_at_request',?),?)`, customerID, username, required, plan.ID, plan.Name, plan.Price, walletCredit, "portal renewal request: "+plan.Name)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		paymentID, _ := res.LastInsertId()
		writeJSON(w, map[string]any{"ok": true, "renewed": false, "payment_required": true, "payment_id": paymentID, "required_amount": required, "wallet": walletCredit, "price": plan.Price})
		return
	}

	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE customers SET plan_id=?,status='active' WHERE id=? AND deleted_at IS NULL`, plan.ID, customerID); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = tx.Exec(`DELETE FROM radcheck WHERE username=? AND attribute='Max-Data'`, username)
	if plan.DataGB > 0 {
		bytes := int64(math.Round(plan.DataGB * 1024 * 1024 * 1024))
		if _, err := tx.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES(?,'Max-Data',':=',?)`, username, bytes); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	_, _ = tx.Exec(`DELETE FROM radreply WHERE username=? AND attribute='Mikrotik-Rate-Limit'`, username)
	if plan.SpeedMbps > 0 {
		if _, err := tx.Exec(`INSERT INTO radreply(username,attribute,op,value) VALUES(?,'Mikrotik-Rate-Limit',':=',?)`, username, speedLimitValue(plan.SpeedMbps)); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	var expires any
	if plan.DurationDays > 0 {
		expires = time.Now().AddDate(0, 0, plan.DurationDays)
	}
	if _, err := tx.Exec(`INSERT INTO subscriptions(customer_id,username,plan_id,expires_at,paid_amount) VALUES(?,?,?,?,?)`, customerID, username, plan.ID, expires, plan.Price); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if plan.Price > 0 {
		desc := "portal plan activated: " + plan.Name
		paymentRes, err := tx.Exec(`INSERT INTO payments(customer_id,username,amount,method,status,admin_note) VALUES(?,?,?,'wallet','approved',?)`, customerID, username, plan.Price, desc)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		paymentID, _ := paymentRes.LastInsertId()
		if _, err := tx.Exec(`INSERT INTO wallets(customer_id,username,credit) VALUES(?,?,?) ON DUPLICATE KEY UPDATE credit=credit+VALUES(credit), customer_id=COALESCE(VALUES(customer_id),customer_id)`, customerID, username, -plan.Price); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if _, err := tx.Exec(`INSERT INTO wallet_transactions(customer_id,username,amount,type,description,actor,reference_type,reference_id) VALUES(?,?,?,?,?,?,?,?)`, customerID, username, -plan.Price, "purchase", desc, "customer", "payment", paymentID); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Auto-provision WireGuard peer on portal subscription renewal
	s.autoProvisionWireGuardPeer(customerID)
	writeJSON(w, map[string]any{"ok": true, "renewed": true, "payment_required": false, "wallet_deducted": plan.Price, "plan": plan})
}

func (s *Server) portalPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	var in struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if len(in.NewPassword) < 4 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "password_too_short"})
		return
	}
	var currentPw string
	err := s.DB.QueryRow(`SELECT value FROM radcheck WHERE username=? AND attribute IN('Cleartext-Password','User-Password') ORDER BY id DESC LIMIT 1`, username).Scan(&currentPw)
	if err != nil {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid_old_password"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(currentPw), []byte(in.OldPassword)) != 1 {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid_old_password"})
		return
	}
	res, err := s.DB.Exec(`UPDATE radcheck SET value=? WHERE username=? AND attribute IN('Cleartext-Password','User-Password')`, in.NewPassword, username)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		_, err = s.DB.Exec(`INSERT INTO radcheck(username,attribute,op,value) VALUES(?,'Cleartext-Password',':=',?)`, username, in.NewPassword)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	s.createEvent("account", "info", "Password changed", "Customer changed their VPN password", username, username)
	writeJSON(w, map[string]any{"ok": true})
}

// portalPreferredNode allows customers to get/set their preferred VPN node.
// GET: returns current preferred node ID
// POST: sets preferred node (0 = random/auto)
func (s *Server) portalPreferredNode(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		var preferredNodeID int64
		_ = s.DB.QueryRow(`SELECT COALESCE(preferred_node_id, 0) FROM customers WHERE username=? AND deleted_at IS NULL`, username).Scan(&preferredNodeID)
		writeJSON(w, map[string]any{"ok": true, "preferred_node_id": preferredNodeID})
	case http.MethodPost:
		var in struct {
			NodeID int64 `json:"node_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}
		// Validate node exists and is active (0 = auto/random)
		if in.NodeID > 0 {
			var exists int
			if err := s.DB.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id=? AND status <> 'disabled'`, in.NodeID).Scan(&exists); err != nil || exists == 0 {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node"})
				return
			}
		}
		if in.NodeID == 0 {
			_, _ = s.DB.Exec(`UPDATE customers SET preferred_node_id=NULL WHERE username=? AND deleted_at IS NULL`, username)
		} else {
			_, _ = s.DB.Exec(`UPDATE customers SET preferred_node_id=? WHERE username=? AND deleted_at IS NULL`, in.NodeID, username)
		}
		writeJSON(w, map[string]any{"ok": true, "preferred_node_id": in.NodeID})
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) portalPayments(w http.ResponseWriter, r *http.Request) {
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := s.DB.Query(`SELECT p.id,p.username,p.amount,p.method,p.status,COALESCE(p.intent_type,'wallet_topup'),p.intent_id,COALESCE(pl.name,''),p.created_at,p.updated_at FROM payments p LEFT JOIN plans pl ON pl.id=p.intent_id AND p.intent_type='plan_renewal' WHERE p.username=? ORDER BY p.id DESC LIMIT 100`, username)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer rows.Close()
		payments := []Payment{}
		for rows.Next() {
			var p Payment
			var intentID sql.NullInt64
			var created, updated sql.NullTime
			if err := rows.Scan(&p.ID, &p.Username, &p.Amount, &p.Method, &p.Status, &p.IntentType, &intentID, &p.IntentLabel, &created, &updated); err == nil {
				if p.IntentType == "" {
					p.IntentType = "wallet_topup"
				}
				if intentID.Valid {
					p.IntentID = &intentID.Int64
				}
				if created.Valid {
					p.CreatedAt = created.Time.Format(time.RFC3339)
				}
				if updated.Valid {
					p.UpdatedAt = updated.Time.Format(time.RFC3339)
				}
				payments = append(payments, p)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "payments": payments})
	case http.MethodPost:
		var in struct {
			Amount  float64 `json:"amount"`
			Method  string  `json:"method"`
			Receipt string  `json:"receipt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}
		if in.Amount <= 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "amount_required"})
			return
		}
		if strings.TrimSpace(in.Method) == "" {
			in.Method = "manual"
		}
		var customerID sql.NullInt64
		_ = s.DB.QueryRow(`SELECT id FROM customers WHERE username=? AND deleted_at IS NULL LIMIT 1`, username).Scan(&customerID)
		res, err := s.DB.Exec(`INSERT INTO payments(customer_id,username,amount,method,receipt,status,intent_type,admin_note) VALUES(?,?,?,?,?,'pending','wallet_topup','portal request')`, nullableInt(customerID), username, in.Amount, strings.TrimSpace(in.Method), strings.TrimSpace(in.Receipt))
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		id, _ := res.LastInsertId()
		writeJSON(w, map[string]any{"ok": true, "id": id})
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) portalMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	var id int64
	var displayName, plan string
	var status string
	var credit float64
	var created sql.NullTime
	var subToken string
	err := s.DB.QueryRow(`SELECT c.id,COALESCE(c.display_name,''),c.status,COALESCE(p.name,''),COALESCE(w.credit,0),c.created_at,COALESCE(c.sub_token,'')
		FROM customers c
		LEFT JOIN plans p ON p.id=c.plan_id
		LEFT JOIN wallets w ON w.username=c.username
		WHERE c.username=? AND c.deleted_at IS NULL LIMIT 1`, username).Scan(&id, &displayName, &status, &plan, &credit, &created, &subToken)
	if err == sql.ErrNoRows {
		writeJSON(w, map[string]any{"ok": true, "customer": map[string]any{"username": username, "status": "active"}})
		return
	}
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	customer := map[string]any{
		"id":           id,
		"username":     username,
		"display_name": displayName,
		"status":       status,
		"plan":         plan,
		"credit":       credit,
		"sub_token":    subToken,
	}
	if created.Valid {
		customer["created_at"] = created.Time.Format(time.RFC3339)
	}

	var subPlan, subStatus string
	var expires sql.NullTime
	if err := s.DB.QueryRow(`SELECT COALESCE(p.name,''),s.status,s.expires_at
		FROM subscriptions s
		LEFT JOIN plans p ON p.id=s.plan_id
		WHERE s.username=? ORDER BY s.id DESC LIMIT 1`, username).Scan(&subPlan, &subStatus, &expires); err == nil {
		sub := map[string]any{"plan": subPlan, "status": subStatus}
		if expires.Valid {
			sub["expires_at"] = expires.Time.Format(time.RFC3339)
		}
		customer["subscription"] = sub
	}

	var maxData string
	if err := s.DB.QueryRow(`SELECT value FROM radcheck WHERE username=? AND attribute='Max-Data' ORDER BY id DESC LIMIT 1`, username).Scan(&maxData); err == nil {
		customer["max_data_bytes"] = maxData
	}
	writeJSON(w, map[string]any{"ok": true, "customer": customer})
}

func (s *Server) nodePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	limitBody(w, r, 5<<20) // 5MB for node push (includes per-user bandwidth data)
	var in struct {
		Token            string             `json:"token"`
		Hostname         string             `json:"hostname"`
		PublicIP         string             `json:"public_ip"`
		PublicIPv6       string             `json:"public_ipv6"`
		OS               string             `json:"os"`
		CPUPercent       float64            `json:"cpu_percent"`
		RAMPercent       float64            `json:"ram_percent"`
		DiskPercent      float64            `json:"disk_percent"`
		RxBps            int64              `json:"rx_bps"`
		TxBps            int64              `json:"tx_bps"`
		RxBytes          int64              `json:"rx_bytes"`
		TxBytes          int64              `json:"tx_bytes"`
		OnlineUsers      int                `json:"online_users"`
		OpenVPNStatus    string             `json:"openvpn_status"`
		L2TPStatus       string             `json:"l2tp_status"`
		IKEv2Status      string             `json:"ikev2_status"`
		Services         map[string]string  `json:"services"`
		Diagnostics      *DiagnosticsReport `json:"diagnostics,omitempty"`
		PerUserBandwidth []struct {
			IP      string `json:"ip"`
			ClassID string `json:"class_id"`
			RxBps   int64  `json:"rx_bps"`
			TxBps   int64  `json:"tx_bps"`
		} `json:"per_user_bandwidth,omitempty"`
		WireguardPeers []struct {
			PublicKey     string `json:"public_key"`
			LastHandshake int64  `json:"last_handshake"`
			RxBytes       int64  `json:"rx_bytes"`
			TxBytes       int64  `json:"tx_bytes"`
		} `json:"wireguard_peers,omitempty"`
		WireguardActivePeers int `json:"wireguard_active_peers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	if in.Token == "" {
		in.Token = r.Header.Get("X-Node-Token")
	}
	if in.Token == "" {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "token_required"})
		return
	}
	var nodeID int64
	var status string
	if err := s.DB.QueryRow(`SELECT id,status FROM nodes WHERE api_token_hash=? LIMIT 1`, hashToken(in.Token)).Scan(&nodeID, &status); err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "bad_token"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if status == "disabled" {
		writeJSONCode(w, http.StatusForbidden, map[string]any{"ok": false, "error": "node_disabled"})
		return
	}
	if in.OpenVPNStatus == "" {
		in.OpenVPNStatus = in.Services["openvpn"]
	}
	if in.L2TPStatus == "" {
		in.L2TPStatus = in.Services["l2tp"]
	}
	if in.IKEv2Status == "" {
		in.IKEv2Status = in.Services["ikev2"]
	}
	if in.OpenVPNStatus == "" {
		in.OpenVPNStatus = "unknown"
	}
	if in.L2TPStatus == "" {
		in.L2TPStatus = "unknown"
	}
	if in.IKEv2Status == "" {
		in.IKEv2Status = "unknown"
	}
	payload, _ := json.Marshal(in)
	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()
	if in.PublicIP != "" {
		_, _ = tx.Exec(`UPDATE nodes SET status='online',last_seen_at=NOW(),public_ip=? WHERE id=?`, in.PublicIP, nodeID)
	} else {
		_, _ = tx.Exec(`UPDATE nodes SET status='online',last_seen_at=NOW() WHERE id=?`, nodeID)
	}
	_, err = tx.Exec(`INSERT INTO node_status(node_id,cpu_percent,ram_percent,disk_percent,rx_bps,tx_bps,openvpn_status,l2tp_status,ikev2_status,payload_json)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE cpu_percent=VALUES(cpu_percent),ram_percent=VALUES(ram_percent),disk_percent=VALUES(disk_percent),rx_bps=VALUES(rx_bps),tx_bps=VALUES(tx_bps),openvpn_status=VALUES(openvpn_status),l2tp_status=VALUES(l2tp_status),ikev2_status=VALUES(ikev2_status),payload_json=VALUES(payload_json)`, nodeID, in.CPUPercent, in.RAMPercent, in.DiskPercent, in.RxBps, in.TxBps, in.OpenVPNStatus, in.L2TPStatus, in.IKEv2Status, string(payload))
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	for service, serviceStatus := range in.Services {
		service = strings.TrimSpace(strings.ToLower(service))
		serviceStatus = strings.TrimSpace(strings.ToLower(serviceStatus))
		if service == "" || serviceStatus == "" {
			continue
		}
		_, _ = tx.Exec(`INSERT INTO node_services(node_id,service,status) VALUES(?,?,?) ON DUPLICATE KEY UPDATE status=VALUES(status)`, nodeID, service, serviceStatus)
	}
	_, _ = tx.Exec(`INSERT INTO node_usage_snapshots(node_id,rx_bytes,tx_bytes,online_users) VALUES(?,?,?,?)`, nodeID, in.RxBytes, in.TxBytes, in.OnlineUsers)
	if in.Diagnostics != nil {
		_, _ = tx.Exec(`INSERT INTO node_diagnostics(node_id, agent_version, uptime_seconds, go_version, goroutines, mem_alloc_bytes) VALUES(?,?,?,?,?,?) ON DUPLICATE KEY UPDATE agent_version=VALUES(agent_version), uptime_seconds=VALUES(uptime_seconds), go_version=VALUES(go_version), goroutines=VALUES(goroutines), mem_alloc_bytes=VALUES(mem_alloc_bytes)`,
			nodeID, in.Diagnostics.AgentVersion, in.Diagnostics.UptimeSeconds, in.Diagnostics.GoVersion, in.Diagnostics.Goroutines, in.Diagnostics.MemAllocBytes)
	}
	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Store per-user bandwidth snapshots if present
	if len(in.PerUserBandwidth) > 0 {
		// Lookup IP-to-username mapping from radacct active sessions
		ipToUser := make(map[string]string)
		rows, err := s.DB.Query(`SELECT username, framedipaddress FROM radacct WHERE acctstoptime IS NULL`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var uname, fip string
				if err := rows.Scan(&uname, &fip); err == nil && fip != "" {
					// Extract last octet from IP for matching with class ID
					parts := strings.Split(fip, ".")
					if len(parts) == 4 {
						ipToUser[parts[3]] = uname
					}
				}
			}
			rows.Close()
		}

		for _, bw := range in.PerUserBandwidth {
			username := ipToUser[bw.IP]
			if username == "" {
				username = "unknown"
			}
			_, _ = s.DB.Exec(`INSERT INTO user_bandwidth_snapshots(node_id, username, ip, rx_bps, tx_bps) VALUES(?,?,?,?,?)`,
				nodeID, username, bw.IP, bw.RxBps, bw.TxBps)
		}
	}

	// Update WireGuard peer stats from node push
	if len(in.WireguardPeers) > 0 {
		for _, wp := range in.WireguardPeers {
			if wp.PublicKey == "" {
				continue
			}
			var handshakeAt *time.Time
			if wp.LastHandshake > 0 {
				t := time.Unix(wp.LastHandshake, 0)
				handshakeAt = &t
			}
			_, _ = s.DB.Exec(`UPDATE wg_peers SET last_handshake_at=?, rx_bytes=?, tx_bytes=? WHERE public_key=? AND node_id=?`,
				handshakeAt, wp.RxBytes, wp.TxBytes, wp.PublicKey, nodeID)
		}
	}

	writeJSON(w, map[string]any{"ok": true, "node_id": nodeID})
}

func (s *Server) radiusRows(table, username string) ([]RadiusCheck, error) {
	if table != "radcheck" && table != "radreply" {
		return nil, fmt.Errorf("invalid_radius_table")
	}
	rows, err := s.DB.Query(`SELECT id,username,attribute,op,value FROM `+table+` WHERE username=? ORDER BY id ASC`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RadiusCheck{}
	for rows.Next() {
		var row RadiusCheck
		if err := rows.Scan(&row.ID, &row.Username, &row.Attribute, &row.Op, &row.Value); err != nil {
			return out, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := s.currentAdmin(r); !ok {
			writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// RequireAdmin is the exported version of requireAdmin for use by the main package.
func (s *Server) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAdmin(next)
}

func (s *Server) requireCustomer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.currentCustomer(r); !ok {
			writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) currentAdmin(r *http.Request) (string, string, bool) {
	username, ok := auth.ReadSession(r, auth.AdminCookieName, s.Config.SessionSecret)
	if !ok {
		return "", "", false
	}
	var role string
	var active int
	err := s.DB.QueryRow(`SELECT role,is_active FROM admins WHERE username=? LIMIT 1`, username).Scan(&role, &active)
	if err != nil || active != 1 {
		return "", "", false
	}
	return username, role, true
}

func (s *Server) currentCustomer(r *http.Request) (string, bool) {
	username, ok := auth.ReadSession(r, auth.CustomerCookieName, s.Config.SessionSecret)
	if !ok {
		return "", false
	}
	var status string
	err := s.DB.QueryRow(`SELECT status FROM customers WHERE username=? AND deleted_at IS NULL LIMIT 1`, username).Scan(&status)
	if err == nil {
		return username, status != "disabled" && status != "deleted"
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM radcheck WHERE username=?`, username).Scan(&count); err != nil {
		return "", false
	}
	return username, count > 0
}

func (s *Server) count(query string, args ...any) int64 {
	var v int64
	_ = s.DB.QueryRow(query, args...).Scan(&v)
	return v
}

func (s *Server) sum(query string, args ...any) float64 {
	var v float64
	_ = s.DB.QueryRow(query, args...).Scan(&v)
	return v
}

func redirectTo(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusFound)
	}
}

func spaHandler(dir, prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		indexPath := filepath.Join(dir, "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`<html><body style="font-family:system-ui;background:#080a10;color:#f8fafc;padding:40px"><h1>Koris UI is not built yet</h1><p>Build the Vue app and copy it to the configured web directory.</p></body></html>`))
			return
		}

		rel := strings.TrimPrefix(r.URL.Path, prefix)
		clean := path.Clean("/" + rel)
		if clean != "/" {
			assetPath := strings.TrimPrefix(clean, "/")
			fullPath := filepath.Join(dir, filepath.FromSlash(assetPath))
			if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
				if strings.HasPrefix(assetPath, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFile(w, r, fullPath)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, indexPath)
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) logAudit(actor, action, entityType, entityID string, before, after map[string]any, ip string) {
	bj, _ := json.Marshal(before)
	aj, _ := json.Marshal(after)
	_, _ = s.DB.Exec(`INSERT INTO audit_logs(actor,action,entity_type,entity_id,before_json,after_json,ip) VALUES(?,?,?,?,?,?,?)`, actor, action, entityType, entityID, string(bj), string(aj), ip)
}

func (s *Server) auditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	offset := 0
	if v, _ := strconv.Atoi(r.URL.Query().Get("limit")); v > 0 && v <= 500 {
		limit = v
	}
	if v, _ := strconv.Atoi(r.URL.Query().Get("offset")); v > 0 {
		offset = v
	}
	rows, err := s.DB.Query(`SELECT id,actor,action,entity_type,entity_id,COALESCE(before_json,''),COALESCE(after_json,''),ip,created_at FROM audit_logs ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	type AuditLog struct {
		ID         int64  `json:"id"`
		Actor      string `json:"actor"`
		Action     string `json:"action"`
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		BeforeJSON string `json:"before_json"`
		AfterJSON  string `json:"after_json"`
		IP         string `json:"ip"`
		CreatedAt  string `json:"created_at"`
	}
	out := []AuditLog{}
	for rows.Next() {
		var a AuditLog
		var before, after []byte
		var created sql.NullTime
		if err := rows.Scan(&a.ID, &a.Actor, &a.Action, &a.EntityType, &a.EntityID, &before, &after, &a.IP, &created); err != nil {
			continue
		}
		a.BeforeJSON = string(before)
		a.AfterJSON = string(after)
		if created.Valid {
			a.CreatedAt = created.Time.Format(time.RFC3339)
		}
		out = append(out, a)
	}
	writeJSON(w, map[string]any{"ok": true, "logs": out, "limit": limit, "offset": offset})
}

func (s *Server) createEvent(eventType, severity, title, message, actor, related string) {
	_, _ = s.DB.Exec(`INSERT INTO events(type,severity,title,message,actor,related) VALUES(?,?,?,?,?,?)`, eventType, severity, title, message, actor, related)
	// Send Telegram notification for warning/error events and key info events
	if severity == "warning" || severity == "error" {
		s.Notify.SendEvent(eventType, title, message)
	} else if eventType == "customer" || eventType == "payment" || eventType == "node" {
		s.Notify.SendEvent(eventType, title, message)
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	offset := 0
	if v, _ := strconv.Atoi(r.URL.Query().Get("limit")); v > 0 && v <= 500 {
		limit = v
	}
	if v, _ := strconv.Atoi(r.URL.Query().Get("offset")); v > 0 {
		offset = v
	}
	where := "1=1"
	args := []any{}
	if seen := r.URL.Query().Get("seen"); seen != "" {
		where += " AND seen=?"
		args = append(args, seen)
	}
	if eventType := r.URL.Query().Get("type"); eventType != "" {
		where += " AND type=?"
		args = append(args, eventType)
	}
	query := fmt.Sprintf(`SELECT id,type,severity,title,COALESCE(message,''),COALESCE(actor,''),COALESCE(related,''),seen,notified,created_at FROM events WHERE %s ORDER BY id DESC LIMIT ? OFFSET ?`, where)
	args = append(args, limit, offset)
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	type Event struct {
		ID        int64  `json:"id"`
		Type      string `json:"type"`
		Severity  string `json:"severity"`
		Title     string `json:"title"`
		Message   string `json:"message"`
		Actor     string `json:"actor"`
		Related   string `json:"related"`
		Seen      bool   `json:"seen"`
		Notified  bool   `json:"notified"`
		CreatedAt string `json:"created_at"`
	}
	out := []Event{}
	for rows.Next() {
		var e Event
		var created sql.NullTime
		var seen, notified int
		if err := rows.Scan(&e.ID, &e.Type, &e.Severity, &e.Title, &e.Message, &e.Actor, &e.Related, &seen, &notified, &created); err != nil {
			continue
		}
		e.Seen = seen == 1
		e.Notified = notified == 1
		if created.Valid {
			e.CreatedAt = created.Time.Format(time.RFC3339)
		}
		out = append(out, e)
	}
	var unseenCount int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE seen=0`).Scan(&unseenCount)
	writeJSON(w, map[string]any{"ok": true, "events": out, "unseen_count": unseenCount, "limit": limit, "offset": offset})
}

func (s *Server) eventByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	id, action, ok := pathID(r.URL.Path, "/api/events/")
	if !ok || action != "seen" {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if _, err := s.DB.Exec(`UPDATE events SET seen=1,notified=1 WHERE id=?`, id); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) portalEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	limit := 100
	if v, _ := strconv.Atoi(r.URL.Query().Get("limit")); v > 0 && v <= 500 {
		limit = v
	}
	rows, err := s.DB.Query(`SELECT id,type,severity,title,COALESCE(message,''),COALESCE(actor,''),COALESCE(related,''),seen,notified,created_at FROM events WHERE related=? ORDER BY id DESC LIMIT ?`, username, limit)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	type Event struct {
		ID        int64  `json:"id"`
		Type      string `json:"type"`
		Severity  string `json:"severity"`
		Title     string `json:"title"`
		Message   string `json:"message"`
		Actor     string `json:"actor"`
		Related   string `json:"related"`
		Seen      bool   `json:"seen"`
		Notified  bool   `json:"notified"`
		CreatedAt string `json:"created_at"`
	}
	out := []Event{}
	for rows.Next() {
		var e Event
		var created sql.NullTime
		var seen, notified int
		if err := rows.Scan(&e.ID, &e.Type, &e.Severity, &e.Title, &e.Message, &e.Actor, &e.Related, &seen, &notified, &created); err != nil {
			continue
		}
		e.Seen = seen == 1
		e.Notified = notified == 1
		if created.Valid {
			e.CreatedAt = created.Time.Format(time.RFC3339)
		}
		out = append(out, e)
	}
	var unseenCount int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM events WHERE related=? AND seen=0`, username).Scan(&unseenCount)
	writeJSON(w, map[string]any{"ok": true, "events": out, "unseen_count": unseenCount})
}

func (s *Server) portalEventByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.currentCustomer(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	id, action, ok := pathID(r.URL.Path, "/api/portal/events/")
	if !ok || action != "seen" {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}
	if _, err := s.DB.Exec(`UPDATE events SET seen=1,notified=1 WHERE id=? AND related=?`, id, username); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func csvResponse(w http.ResponseWriter, filename string, headers []string, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	cw := csv.NewWriter(w)
	_ = cw.Write(headers)
	for _, row := range rows {
		_ = cw.Write(row)
	}
	cw.Flush()
}

func (s *Server) exportCustomersCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`SELECT c.id,c.username,COALESCE(c.display_name,''),c.status,COALESCE(p.name,''),COALESCE(w.credit,0),c.created_at FROM customers c LEFT JOIN plans p ON p.id=c.plan_id LEFT JOIN wallets w ON w.username=c.username WHERE c.deleted_at IS NULL ORDER BY c.id DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		var id int64
		var username, displayName, status, plan string
		var credit float64
		var created sql.NullTime
		if err := rows.Scan(&id, &username, &displayName, &status, &plan, &credit, &created); err != nil {
			continue
		}
		createdStr := ""
		if created.Valid {
			createdStr = created.Time.Format(time.RFC3339)
		}
		out = append(out, []string{strconv.FormatInt(id, 10), username, displayName, status, plan, fmt.Sprintf("%.2f", credit), createdStr})
	}
	csvResponse(w, "customers.csv", []string{"id", "username", "display_name", "status", "plan", "credit", "created_at"}, out)
}

func (s *Server) exportPaymentsCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`SELECT id,username,amount,method,status,COALESCE(intent_type,'wallet_topup'),intent_id,created_at FROM payments ORDER BY id DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		var id int64
		var username, method, status, intentType string
		var amount float64
		var intentID sql.NullInt64
		var created sql.NullTime
		if err := rows.Scan(&id, &username, &amount, &method, &status, &intentType, &intentID, &created); err != nil {
			continue
		}
		intentIDStr := ""
		if intentID.Valid {
			intentIDStr = strconv.FormatInt(intentID.Int64, 10)
		}
		createdStr := ""
		if created.Valid {
			createdStr = created.Time.Format(time.RFC3339)
		}
		out = append(out, []string{strconv.FormatInt(id, 10), username, fmt.Sprintf("%.2f", amount), method, status, intentType, intentIDStr, createdStr})
	}
	csvResponse(w, "payments.csv", []string{"id", "username", "amount", "method", "status", "intent_type", "intent_id", "created_at"}, out)
}

func (s *Server) exportRadacctCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`SELECT radacctid,username,acctstarttime,acctstoptime,COALESCE(acctsessiontime,0),COALESCE(acctinputoctets,0),COALESCE(acctoutputoctets,0),framedipaddress,acctterminatecause FROM radacct ORDER BY radacctid DESC LIMIT 10000`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		var id, sessionTime, inputBytes, outputBytes int64
		var username, framedIP, terminateCause string
		var start, stop sql.NullTime
		if err := rows.Scan(&id, &username, &start, &stop, &sessionTime, &inputBytes, &outputBytes, &framedIP, &terminateCause); err != nil {
			continue
		}
		startStr, stopStr := "", ""
		if start.Valid {
			startStr = start.Time.Format(time.RFC3339)
		}
		if stop.Valid {
			stopStr = stop.Time.Format(time.RFC3339)
		}
		out = append(out, []string{strconv.FormatInt(id, 10), username, startStr, stopStr, strconv.FormatInt(sessionTime, 10), strconv.FormatInt(inputBytes, 10), strconv.FormatInt(outputBytes, 10), framedIP, terminateCause})
	}
	csvResponse(w, "radacct.csv", []string{"id", "username", "start_time", "stop_time", "session_seconds", "input_bytes", "output_bytes", "framed_ip", "terminate_cause"}, out)
}

func (s *Server) exportWalletTransactionsCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query(`SELECT id,username,amount,type,description,actor,COALESCE(reference_type,''),reference_id,created_at FROM wallet_transactions ORDER BY id DESC LIMIT 10000`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		var id int64
		var amount float64
		var username, ttype, description, actor, refType string
		var refID sql.NullInt64
		var created sql.NullTime
		if err := rows.Scan(&id, &username, &amount, &ttype, &description, &actor, &refType, &refID, &created); err != nil {
			continue
		}
		refIDStr := ""
		if refID.Valid {
			refIDStr = strconv.FormatInt(refID.Int64, 10)
		}
		createdStr := ""
		if created.Valid {
			createdStr = created.Time.Format(time.RFC3339)
		}
		out = append(out, []string{strconv.FormatInt(id, 10), username, fmt.Sprintf("%.2f", amount), ttype, description, actor, refType, refIDStr, createdStr})
	}
	csvResponse(w, "wallet-transactions.csv", []string{"id", "username", "amount", "type", "description", "actor", "reference_type", "reference_id", "created_at"}, out)
}

// ─── Database Backup Export/Import ───────────────────────────────────────────

func (s *Server) backupExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	backup := map[string]any{
		"version":     1,
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"tables":      map[string]any{},
	}
	tables := backup["tables"].(map[string]any)

	// Customers
	customers := []map[string]any{}
	rows, err := s.DB.Query(`SELECT id, username, COALESCE(display_name,''), status, plan_id, COALESCE(notes,''), COALESCE(sub_token,''), created_at FROM customers WHERE deleted_at IS NULL ORDER BY id`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int64
			var username, displayName, status, notes, subToken string
			var planID sql.NullInt64
			var created sql.NullTime
			if err := rows.Scan(&id, &username, &displayName, &status, &planID, &notes, &subToken, &created); err != nil {
				continue
			}
			c := map[string]any{"id": id, "username": username, "display_name": displayName, "status": status, "notes": notes, "sub_token": subToken}
			if planID.Valid {
				c["plan_id"] = planID.Int64
			}
			if created.Valid {
				c["created_at"] = created.Time.Format(time.RFC3339)
			}
			customers = append(customers, c)
		}
	}
	tables["customers"] = customers

	// Payments
	payments := []map[string]any{}
	pRows, err := s.DB.Query(`SELECT id, username, amount, method, status, COALESCE(intent_type,''), intent_id, created_at FROM payments ORDER BY id`)
	if err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var id int64
			var username, method, status, intentType string
			var amount float64
			var intentID sql.NullInt64
			var created sql.NullTime
			if err := pRows.Scan(&id, &username, &amount, &method, &status, &intentType, &intentID, &created); err != nil {
				continue
			}
			p := map[string]any{"id": id, "username": username, "amount": amount, "method": method, "status": status, "intent_type": intentType}
			if intentID.Valid {
				p["intent_id"] = intentID.Int64
			}
			if created.Valid {
				p["created_at"] = created.Time.Format(time.RFC3339)
			}
			payments = append(payments, p)
		}
	}
	tables["payments"] = payments

	// Plans
	plans := []map[string]any{}
	plRows, err := s.DB.Query(`SELECT id, name, COALESCE(data_gb,0), COALESCE(speed_mbps,0), COALESCE(duration_days,0), COALESCE(price,0), COALESCE(billing_type,'fixed'), COALESCE(price_per_gb,0), COALESCE(price_per_day,0), disconnect_on_zero, is_active, COALESCE(sort_order,0), created_at FROM plans ORDER BY id`)
	if err == nil {
		defer plRows.Close()
		for plRows.Next() {
			var id int64
			var name, billingType string
			var dataGB, speedMbps, price, pricePerGB, pricePerDay float64
			var durationDays, sortOrder int
			var disconnectOnZero, isActive bool
			var created sql.NullTime
			if err := plRows.Scan(&id, &name, &dataGB, &speedMbps, &durationDays, &price, &billingType, &pricePerGB, &pricePerDay, &disconnectOnZero, &isActive, &sortOrder, &created); err != nil {
				continue
			}
			pl := map[string]any{"id": id, "name": name, "data_gb": dataGB, "speed_mbps": speedMbps, "duration_days": durationDays, "price": price, "billing_type": billingType, "price_per_gb": pricePerGB, "price_per_day": pricePerDay, "disconnect_on_zero": disconnectOnZero, "is_active": isActive, "sort_order": sortOrder}
			if created.Valid {
				pl["created_at"] = created.Time.Format(time.RFC3339)
			}
			plans = append(plans, pl)
		}
	}
	tables["plans"] = plans

	// Wallets
	wallets := []map[string]any{}
	wRows, err := s.DB.Query(`SELECT username, credit FROM wallets ORDER BY username`)
	if err == nil {
		defer wRows.Close()
		for wRows.Next() {
			var username string
			var credit float64
			if err := wRows.Scan(&username, &credit); err != nil {
				continue
			}
			wallets = append(wallets, map[string]any{"username": username, "credit": credit})
		}
	}
	tables["wallets"] = wallets

	// Nodes
	nodes := []map[string]any{}
	nRows, err := s.DB.Query(`SELECT id, name, public_ip, COALESCE(domain,''), status, created_at FROM nodes ORDER BY id`)
	if err == nil {
		defer nRows.Close()
		for nRows.Next() {
			var id int64
			var name, publicIP, domain, status string
			var created sql.NullTime
			if err := nRows.Scan(&id, &name, &publicIP, &domain, &status, &created); err != nil {
				continue
			}
			n := map[string]any{"id": id, "name": name, "public_ip": publicIP, "domain": domain, "status": status}
			if created.Valid {
				n["created_at"] = created.Time.Format(time.RFC3339)
			}
			nodes = append(nodes, n)
		}
	}
	tables["nodes"] = nodes

	// VPN Configs
	vpnConfigs := []map[string]any{}
	vcRows, err := s.DB.Query(`SELECT id, node_id, protocol, port, COALESCE(network,''), enabled, COALESCE(mtu,1500), COALESCE(max_clients,0), COALESCE(enable_logs,1), COALESCE(conn_limit,0), extra_json FROM vpn_configs ORDER BY id`)
	if err == nil {
		defer vcRows.Close()
		for vcRows.Next() {
			var id, nodeID int64
			var protocol, network string
			var port, mtu, maxClients, connLimit int
			var enabled, enableLogs bool
			var extraJSON sql.NullString
			if err := vcRows.Scan(&id, &nodeID, &protocol, &port, &network, &enabled, &mtu, &maxClients, &enableLogs, &connLimit, &extraJSON); err != nil {
				continue
			}
			vc := map[string]any{"id": id, "node_id": nodeID, "protocol": protocol, "port": port, "network": network, "enabled": enabled, "mtu": mtu, "max_clients": maxClients, "enable_logs": enableLogs, "conn_limit": connLimit}
			if extraJSON.Valid && extraJSON.String != "" {
				var extra map[string]any
				if json.Unmarshal([]byte(extraJSON.String), &extra) == nil {
					vc["extra_json"] = extra
				}
			}
			vpnConfigs = append(vpnConfigs, vc)
		}
	}
	tables["vpn_configs"] = vpnConfigs

	// Radcheck
	radcheck := []map[string]any{}
	rcRows, err := s.DB.Query(`SELECT id, username, attribute, op, value FROM radcheck ORDER BY id`)
	if err == nil {
		defer rcRows.Close()
		for rcRows.Next() {
			var id int64
			var username, attribute, op, value string
			if err := rcRows.Scan(&id, &username, &attribute, &op, &value); err != nil {
				continue
			}
			radcheck = append(radcheck, map[string]any{"id": id, "username": username, "attribute": attribute, "op": op, "value": value})
		}
	}
	tables["radcheck"] = radcheck

	// Subscriptions
	subscriptions := []map[string]any{}
	subRows, err := s.DB.Query(`SELECT id, username, COALESCE(plan,''), status, started_at, expires_at, COALESCE(paid_amount,0), COALESCE(discount_code,'') FROM subscriptions ORDER BY id`)
	if err == nil {
		defer subRows.Close()
		for subRows.Next() {
			var id int64
			var username, plan, status, discountCode string
			var paidAmount float64
			var startedAt, expiresAt sql.NullTime
			if err := subRows.Scan(&id, &username, &plan, &status, &startedAt, &expiresAt, &paidAmount, &discountCode); err != nil {
				continue
			}
			sub := map[string]any{"id": id, "username": username, "plan": plan, "status": status, "paid_amount": paidAmount, "discount_code": discountCode}
			if startedAt.Valid {
				sub["started_at"] = startedAt.Time.Format(time.RFC3339)
			}
			if expiresAt.Valid {
				sub["expires_at"] = expiresAt.Time.Format(time.RFC3339)
			}
			subscriptions = append(subscriptions, sub)
		}
	}
	tables["subscriptions"] = subscriptions

	// Tickets
	tickets := []map[string]any{}
	tRows, err := s.DB.Query(`SELECT id, customer_id, username, subject, status, priority, created_at FROM tickets WHERE deleted_at IS NULL ORDER BY id`)
	if err == nil {
		defer tRows.Close()
		for tRows.Next() {
			var id int64
			var customerID sql.NullInt64
			var username, subject, status, priority string
			var created sql.NullTime
			if err := tRows.Scan(&id, &customerID, &username, &subject, &status, &priority, &created); err != nil {
				continue
			}
			tk := map[string]any{"id": id, "username": username, "subject": subject, "status": status, "priority": priority}
			if customerID.Valid {
				tk["customer_id"] = customerID.Int64
			}
			if created.Valid {
				tk["created_at"] = created.Time.Format(time.RFC3339)
			}
			tickets = append(tickets, tk)
		}
	}
	tables["tickets"] = tickets

	// Wallet Transactions
	walletTx := []map[string]any{}
	wtRows, err := s.DB.Query(`SELECT id, username, amount, type, description, actor, COALESCE(reference_type,''), reference_id, created_at FROM wallet_transactions ORDER BY id`)
	if err == nil {
		defer wtRows.Close()
		for wtRows.Next() {
			var id int64
			var amount float64
			var username, ttype, description, actor, refType string
			var refID sql.NullInt64
			var created sql.NullTime
			if err := wtRows.Scan(&id, &username, &amount, &ttype, &description, &actor, &refType, &refID, &created); err != nil {
				continue
			}
			wt := map[string]any{"id": id, "username": username, "amount": amount, "type": ttype, "description": description, "actor": actor, "reference_type": refType}
			if refID.Valid {
				wt["reference_id"] = refID.Int64
			}
			if created.Valid {
				wt["created_at"] = created.Time.Format(time.RFC3339)
			}
			walletTx = append(walletTx, wt)
		}
	}
	tables["wallet_transactions"] = walletTx

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="panel-backup.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(backup)
}

func (s *Server) backupImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_form"})
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "file_required"})
		return
	}
	defer file.Close()

	var backup struct {
		Version    int                         `json:"version"`
		ExportedAt string                      `json:"exported_at"`
		Tables     map[string][]map[string]any `json:"tables"`
	}
	if err := json.NewDecoder(file).Decode(&backup); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_json"})
		return
	}
	if backup.Version == 0 || backup.Tables == nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_backup_format"})
		return
	}

	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	// Track import statistics per table
	imported := map[string]int{}
	failed := map[string]int{}

	// Import nodes first (referenced by vpn_configs)
	if nodes, ok := backup.Tables["nodes"]; ok {
		for _, n := range nodes {
			_, err := tx.Exec(`INSERT IGNORE INTO nodes(id, name, public_ip, domain, status) VALUES(?,?,?,?,?)`,
				toInt64(n["id"]), toString(n["name"]), toString(n["public_ip"]),
				toString(n["domain"]), toString(n["status"]))
			if err != nil {
				failed["nodes"]++
			} else {
				imported["nodes"]++
			}
		}
	}

	// Import plans (referenced by customers)
	if plans, ok := backup.Tables["plans"]; ok {
		for _, p := range plans {
			_, err := tx.Exec(`INSERT IGNORE INTO plans(id, name, data_gb, speed_mbps, duration_days, price, billing_type, price_per_gb, price_per_day, disconnect_on_zero, is_active, sort_order) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
				toInt64(p["id"]), toString(p["name"]), toFloat64(p["data_gb"]), toFloat64(p["speed_mbps"]),
				toInt(p["duration_days"]), toFloat64(p["price"]), toString(p["billing_type"]),
				toFloat64(p["price_per_gb"]), toFloat64(p["price_per_day"]), toBool(p["disconnect_on_zero"]),
				toBool(p["is_active"]), toInt(p["sort_order"]))
			if err != nil {
				failed["plans"]++
			} else {
				imported["plans"]++
			}
		}
	}

	// Import customers
	if customers, ok := backup.Tables["customers"]; ok {
		for _, c := range customers {
			planID := sql.NullInt64{}
			if v, exists := c["plan_id"]; exists && v != nil {
				planID = sql.NullInt64{Int64: toInt64(v), Valid: true}
			}
			_, err := tx.Exec(`INSERT IGNORE INTO customers(id, username, display_name, status, plan_id, notes, sub_token) VALUES(?,?,?,?,?,?,?)`,
				toInt64(c["id"]), toString(c["username"]), toString(c["display_name"]),
				toString(c["status"]), planID, toString(c["notes"]), toString(c["sub_token"]))
			if err != nil {
				failed["customers"]++
			} else {
				imported["customers"]++
			}
		}
	}

	// Import wallets
	if wallets, ok := backup.Tables["wallets"]; ok {
		for _, wal := range wallets {
			_, err := tx.Exec(`INSERT INTO wallets(username, credit) VALUES(?,?) ON DUPLICATE KEY UPDATE credit=VALUES(credit)`,
				toString(wal["username"]), toFloat64(wal["credit"]))
			if err != nil {
				failed["wallets"]++
			} else {
				imported["wallets"]++
			}
		}
	}

	// Import radcheck
	if radcheck, ok := backup.Tables["radcheck"]; ok {
		for _, rc := range radcheck {
			_, err := tx.Exec(`INSERT IGNORE INTO radcheck(id, username, attribute, op, value) VALUES(?,?,?,?,?)`,
				toInt64(rc["id"]), toString(rc["username"]), toString(rc["attribute"]), toString(rc["op"]), toString(rc["value"]))
			if err != nil {
				failed["radcheck"]++
			} else {
				imported["radcheck"]++
			}
		}
	}

	// Import payments
	if payments, ok := backup.Tables["payments"]; ok {
		for _, p := range payments {
			intentID := sql.NullInt64{}
			if v, exists := p["intent_id"]; exists && v != nil {
				intentID = sql.NullInt64{Int64: toInt64(v), Valid: true}
			}
			_, err := tx.Exec(`INSERT IGNORE INTO payments(id, username, amount, method, status, intent_type, intent_id) VALUES(?,?,?,?,?,?,?)`,
				toInt64(p["id"]), toString(p["username"]), toFloat64(p["amount"]),
				toString(p["method"]), toString(p["status"]), toString(p["intent_type"]), intentID)
			if err != nil {
				failed["payments"]++
			} else {
				imported["payments"]++
			}
		}
	}

	// Import subscriptions
	if subs, ok := backup.Tables["subscriptions"]; ok {
		for _, sub := range subs {
			_, err := tx.Exec(`INSERT IGNORE INTO subscriptions(id, username, plan, status, paid_amount, discount_code) VALUES(?,?,?,?,?,?)`,
				toInt64(sub["id"]), toString(sub["username"]), toString(sub["plan"]),
				toString(sub["status"]), toFloat64(sub["paid_amount"]), toString(sub["discount_code"]))
			if err != nil {
				failed["subscriptions"]++
			} else {
				imported["subscriptions"]++
			}
		}
	}

	// Import tickets
	if tickets, ok := backup.Tables["tickets"]; ok {
		for _, tk := range tickets {
			customerID := sql.NullInt64{}
			if v, exists := tk["customer_id"]; exists && v != nil {
				customerID = sql.NullInt64{Int64: toInt64(v), Valid: true}
			}
			_, err := tx.Exec(`INSERT IGNORE INTO tickets(id, customer_id, username, subject, status, priority) VALUES(?,?,?,?,?,?)`,
				toInt64(tk["id"]), customerID, toString(tk["username"]),
				toString(tk["subject"]), toString(tk["status"]), toString(tk["priority"]))
			if err != nil {
				failed["tickets"]++
			} else {
				imported["tickets"]++
			}
		}
	}

	// Import wallet transactions
	if wtxs, ok := backup.Tables["wallet_transactions"]; ok {
		for _, wt := range wtxs {
			refID := sql.NullInt64{}
			if v, exists := wt["reference_id"]; exists && v != nil {
				refID = sql.NullInt64{Int64: toInt64(v), Valid: true}
			}
			_, err := tx.Exec(`INSERT IGNORE INTO wallet_transactions(id, username, amount, type, description, actor, reference_type, reference_id) VALUES(?,?,?,?,?,?,?,?)`,
				toInt64(wt["id"]), toString(wt["username"]), toFloat64(wt["amount"]),
				toString(wt["type"]), toString(wt["description"]), toString(wt["actor"]),
				toString(wt["reference_type"]), refID)
			if err != nil {
				failed["wallet_transactions"]++
			} else {
				imported["wallet_transactions"]++
			}
		}
	}

	// Import vpn_configs (depends on nodes)
	if vpnConfigs, ok := backup.Tables["vpn_configs"]; ok {
		for _, vc := range vpnConfigs {
			var extraJSONStr sql.NullString
			if extra, exists := vc["extra_json"]; exists && extra != nil {
				if extraBytes, err := json.Marshal(extra); err == nil {
					extraJSONStr = sql.NullString{String: string(extraBytes), Valid: true}
				}
			}
			_, err := tx.Exec(`INSERT IGNORE INTO vpn_configs(id, node_id, protocol, port, network, enabled, mtu, max_clients, enable_logs, conn_limit, extra_json) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
				toInt64(vc["id"]), toInt64(vc["node_id"]), toString(vc["protocol"]),
				toInt(vc["port"]), toString(vc["network"]), toBool(vc["enabled"]),
				toInt(vc["mtu"]), toInt(vc["max_clients"]), toBool(vc["enable_logs"]),
				toInt(vc["conn_limit"]), extraJSONStr)
			if err != nil {
				failed["vpn_configs"]++
			} else {
				imported["vpn_configs"]++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, map[string]any{"ok": true, "imported": imported, "failed": failed})
}

// Backup helper functions for type conversion
func toInt64(v any) int64 {
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int64:
		return val
	case int:
		return int64(val)
	case json.Number:
		n, _ := val.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(val, 10, 64)
		return n
	}
	return 0
}

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	case json.Number:
		n, _ := val.Float64()
		return n
	case string:
		n, _ := strconv.ParseFloat(val, 64)
		return n
	}
	return 0
}

func toInt(v any) int {
	return int(toInt64(v))
}

func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case int:
		return val != 0
	case string:
		return val == "true" || val == "1"
	}
	return false
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(val, 10)
	case bool:
		if val {
			return "true"
		}
		return "false"
	}
	return fmt.Sprintf("%v", v)
}

func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	isActive := func(service string) bool {
		cmd := exec.Command("systemctl", "is-active", service)
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.TrimSpace(string(out)) == "active"
	}

	runCmd := func(name string, args ...string) string {
		cmd := exec.Command(name, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	var checks []map[string]any

	checks = append(checks, map[string]any{
		"name":   "Nginx service",
		"ok":     isActive("nginx"),
		"detail": "systemctl is-active nginx",
	})
	checks = append(checks, map[string]any{
		"name":   "MariaDB service",
		"ok":     isActive("mariadb"),
		"detail": "systemctl is-active mariadb",
	})
	checks = append(checks, map[string]any{
		"name":   "Auth service",
		"ok":     isActive("freeradius"),
		"detail": "systemctl is-active freeradius",
	})
	checks = append(checks, map[string]any{
		"name":   "Panel service",
		"ok":     isActive("panel"),
		"detail": "systemctl is-active panel",
	})
	checks = append(checks, map[string]any{
		"name":   "OpenVPN service",
		"ok":     isActive("openvpn-server@server") || isActive("openvpn"),
		"detail": "systemctl is-active openvpn-server@server",
	})
	checks = append(checks, map[string]any{
		"name":   "Node agent",
		"ok":     isActive("node-agent"),
		"detail": "systemctl is-active node-agent",
	})
	checks = append(checks, map[string]any{
		"name":   "L2TP service",
		"ok":     isActive("xl2tpd"),
		"detail": "systemctl is-active xl2tpd",
	})
	checks = append(checks, map[string]any{
		"name":   "IKEv2 service",
		"ok":     isActive("strongswan") || isActive("strongswan-starter") || isActive("swanctl"),
		"detail": "strongswan service check",
	})

	disk := runCmd("sh", "-c", "df -h / | tail -1 | awk '{print $3 \" / \" $2 \" (\" $5 \")\"}'")
	if disk == "" {
		disk = "N/A"
	}

	mem := runCmd("sh", "-c", "free -h | awk '/Mem:/ {print $3 \" / \" $2}'")
	if mem == "" {
		mem = "N/A"
	}

	ports := runCmd("sh", "-c", "ss -ltnp | grep -E ':(80|443|8088|1194|1812|1813)'")

	writeJSON(w, map[string]any{
		"ok":     true,
		"disk":   disk,
		"mem":    mem,
		"checks": checks,
		"ports":  ports,
	})
}

func (s *Server) resellers(w http.ResponseWriter, r *http.Request) {
	actor, role, ok := s.currentAdmin(r)
	if !ok || (role != "owner" && role != "admin") {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	if r.Method == http.MethodGet {
		rows, err := s.DB.Query(`SELECT id, username, role, is_active, credit, created_at FROM admins WHERE role='reseller' ORDER BY id DESC`)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer rows.Close()

		type Reseller struct {
			ID        int64   `json:"id"`
			Username  string  `json:"username"`
			Role      string  `json:"role"`
			IsActive  bool    `json:"is_active"`
			Credit    float64 `json:"credit"`
			CreatedAt string  `json:"created_at"`
		}

		list := []Reseller{}
		for rows.Next() {
			var res Reseller
			var active int
			var created time.Time
			if err := rows.Scan(&res.ID, &res.Username, &res.Role, &active, &res.Credit, &created); err == nil {
				res.IsActive = active == 1
				res.CreatedAt = created.Format(time.RFC3339)
				list = append(list, res)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "resellers": list})
		return
	}

	if r.Method == http.MethodPost {
		var in struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}
		in.Username = strings.TrimSpace(in.Username)
		if len(in.Username) < 3 || len(in.Password) < 4 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_username_or_password"})
			return
		}

		ph, err := auth.HashPassword(in.Password)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}

		_, err = s.DB.Exec(`INSERT INTO admins(username, password_hash, role, is_active) VALUES(?,?, 'reseller', 1)`, in.Username, ph)
		if err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "username_taken"})
			return
		}

		s.logAudit(actor, "reseller.created", "reseller", in.Username, nil, map[string]any{"username": in.Username}, clientIP(r))
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	http.Error(w, "method", http.StatusMethodNotAllowed)
}

func (s *Server) resellerByID(w http.ResponseWriter, r *http.Request) {
	actor, role, ok := s.currentAdmin(r)
	if !ok || (role != "owner" && role != "admin") {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/resellers/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "id_required"})
		return
	}
	id, _ := strconv.ParseInt(parts[0], 10, 64)

	var resellerUsername string
	err := s.DB.QueryRow(`SELECT username FROM admins WHERE id=? AND role='reseller' LIMIT 1`, id).Scan(&resellerUsername)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "reseller_not_found"})
		return
	}

	if r.Method == http.MethodDelete {
		_, err = s.DB.Exec(`DELETE FROM admins WHERE id=?`, id)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		s.logAudit(actor, "reseller.deleted", "reseller", strconv.FormatInt(id, 10), nil, map[string]any{"username": resellerUsername}, clientIP(r))
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	if r.Method == http.MethodPost && len(parts) > 1 && parts[1] == "credit" {
		var in struct {
			Amount float64 `json:"amount"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}

		_, err = s.DB.Exec(`UPDATE admins SET credit = credit + ? WHERE id=?`, in.Amount, id)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}

		desc := fmt.Sprintf("Admin %s adjusted reseller credit by %.2f", actor, in.Amount)
		ttype := "allocation"
		if in.Amount < 0 {
			ttype = "deduction"
		}
		_, _ = s.DB.Exec(`INSERT INTO reseller_transactions(reseller_username, amount, type, description, actor) VALUES(?,?,?,?,?)`, resellerUsername, in.Amount, ttype, desc, actor)

		s.logAudit(actor, "reseller.credit_adjusted", "reseller", strconv.FormatInt(id, 10), nil, map[string]any{"username": resellerUsername, "amount": in.Amount}, clientIP(r))
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	http.Error(w, "method", http.StatusMethodNotAllowed)
}

func (s *Server) subscriptionLink(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = strings.TrimPrefix(r.URL.Path, "/portal/sub/")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
		return
	}

	var username string
	var status string
	err := s.DB.QueryRow(`SELECT username, status FROM customers WHERE sub_token=? LIMIT 1`, token).Scan(&username, &status)
	if err == sql.ErrNoRows {
		http.Error(w, "Subscription not found", http.StatusNotFound)
		return
	}

	ua := strings.ToLower(r.Header.Get("User-Agent"))

	if strings.Contains(ua, "clash") {
		host, port, _, _ := s.openVPNEndpoint(r)

		yaml := fmt.Sprintf(`port: 7890
socks-port: 7891
allow-lan: true
mode: Rule
log-level: info
proxies:
  - name: "Koris-OpenVPN-%s"
    type: socks5
    server: "%s"
    port: %d
    # Subscription URL for direct profile: http://%s/api/portal/profiles/openvpn.ovpn?token=%s
  - name: "Koris-L2TP-%s"
    type: socks5
    server: "%s"
    port: 1701
    # L2TP/IPSec connection available. Configure in device settings.
proxy-groups:
  - name: PROXY
    type: select
    proxies:
      - "Koris-OpenVPN-%s"
      - "Koris-L2TP-%s"
rules:
  - DOMAIN-SUFFIX,ir,DIRECT
  - DOMAIN-SUFFIX,telewebion.com,DIRECT
  - DOMAIN-SUFFIX,snapp.ir,DIRECT
  - DOMAIN-KEYWORD,adservice,REJECT
  - DOMAIN-KEYWORD,analytics,REJECT
  - MATCH,PROXY
`, username, host, port, r.Host, token, username, host, username, username)

		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(yaml))
		return
	}

	isClientApp := strings.Contains(ua, "shadowrocket") || strings.Contains(ua, "sing-box") || strings.Contains(ua, "v2ray") || strings.Contains(ua, "trojan")

	if isClientApp {
		host, port, proto, _ := s.openVPNEndpoint(r)
		var psk string
		_ = s.DB.QueryRow(`SELECT COALESCE(ipsec_psk,'') FROM vpn_core_settings WHERE id=1`).Scan(&psk)

		var builder strings.Builder
		builder.WriteString("# Koris Unified Subscription\n")
		builder.WriteString(fmt.Sprintf("# User: %s (Status: %s)\n\n", username, status))

		ovpnURL := fmt.Sprintf("http://%s/api/portal/profiles/openvpn.ovpn?token=%s", r.Host, token)
		builder.WriteString(fmt.Sprintf("REMARKS=OpenVPN Node, URL=%s, PORT=%d, PROTOCOL=%s\n", ovpnURL, port, proto))
		builder.WriteString(fmt.Sprintf("REMARKS=L2TP Node, HOST=%s, PSK=%s, USERNAME=%s\n", host, psk, username))
		builder.WriteString(fmt.Sprintf("REMARKS=IKEv2 Node, HOST=%s, USERNAME=%s\n", host, username))

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Subscription-Userinfo", fmt.Sprintf("upload=0; download=0; total=100000000000; expire=0"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(builder.String()))))
		return
	}

	var maxData int64
	_ = s.DB.QueryRow(`SELECT COALESCE(value,0) FROM radcheck WHERE username=? AND attribute='Max-Data'`, username).Scan(&maxData)

	var used int64
	_ = s.DB.QueryRow(`SELECT COALESCE(SUM(acctinputoctets+acctoutputoctets),0) FROM radacct WHERE username=?`, username).Scan(&used)

	var online int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM radacct WHERE username=? AND acctstoptime IS NULL`, username).Scan(&online)

	isOnline := online > 0
	pct := 0.0
	if maxData > 0 {
		pct = math.Min(100.0, float64(used)/float64(maxData)*100.0)
	}

	lang := strings.ToLower(r.URL.Query().Get("lang"))
	if lang == "" {
		lang = "en"
	}

	translations := map[string]map[string]string{
		"en": {
			"title":       "Unified Secure Access Portal",
			"status":      "Status",
			"usage":       "Usage Summary",
			"download":    "Download OpenVPN Profile",
			"server":      "Server",
			"username":    "Username",
			"l2tp_psk":    "L2TP PSK",
			"unlimited":   "Unlimited",
			"online":      "Online",
			"offline":     "Offline",
			"langs":       `LANGS: <a href="?token=%s&lang=en">EN</a> · <a href="?token=%s&lang=fa">FA</a> · <a href="?token=%s&lang=ru">RU</a> · <a href="?token=%s&lang=zh">ZH</a>`,
			"guide_title": "Manual Setup Connection Guides",
			"guide_desc":  "For Windows, iOS & macOS native setups: Add an L2TP/IPSec VPN connection. Use the Server Address above, select Username/Password authentication, and enter the pre-shared secret (PSK) provided by your administrator.",
		},
		"fa": {
			"title":       "پورتال دسترسی امن یکپارچه",
			"status":      "وضعیت",
			"usage":       "خلاصه مصرف",
			"download":    "دانلود فایل تنظیمات OpenVPN",
			"server":      "سرور",
			"username":    "نام کاربری",
			"l2tp_psk":    "کلید L2TP PSK",
			"unlimited":   "نامحدود",
			"online":      "متصل",
			"offline":     "قطع",
			"langs":       `زبان‌ها: <a href="?token=%s&lang=en">EN</a> · <a href="?token=%s&lang=fa">FA</a> · <a href="?token=%s&lang=ru">RU</a> · <a href="?token=%s&lang=zh">ZH</a>`,
			"guide_title": "راهنمای اتصال دستی",
			"guide_desc":  "برای تنظیمات بومی ویندوز، iOS و macOS: یک اتصال VPN از نوع L2TP/IPSec اضافه کنید. از آدرس سرور بالا استفاده کنید، نوع تایید هویت را روی Username/Password بگذارید، و کلید مشترک (PSK) را وارد کنید.",
		},
		"ru": {
			"title":       "Единый портал безопасного доступа",
			"status":      "Статус",
			"usage":       "Сводка использования",
			"download":    "Скачать профиль OpenVPN",
			"server":      "Сервер",
			"username":    "Имя пользователя",
			"l2tp_psk":    "L2TP PSK",
			"unlimited":   "Безлимитный",
			"online":      "Онлайн",
			"offline":     "Оффлайн",
			"langs":       `Языки: <a href="?token=%s&lang=en">EN</a> · <a href="?token=%s&lang=fa">FA</a> · <a href="?token=%s&lang=ru">RU</a> · <a href="?token=%s&lang=zh">ZH</a>`,
			"guide_title": "Инструкции по ручной настройке",
			"guide_desc":  "Для стандартных подключений Windows, iOS и macOS: Добавьте VPN-подключение L2TP/IPSec. Используйте адрес сервера выше, выберите аутентификацию по имени пользователя/паролю и введите общий ключ (PSK).",
		},
		"zh": {
			"title":       "统一安全访问门户",
			"status":      "状态",
			"usage":       "用量摘要",
			"download":    "下载 OpenVPN 配置文件",
			"server":      "服务器",
			"username":    "用户名",
			"l2tp_psk":    "L2TP 预共享密钥",
			"unlimited":   "无限制",
			"online":      "在线",
			"offline":     "离线",
			"langs":       `语言: <a href="?token=%s&lang=en">EN</a> · <a href="?token=%s&lang=fa">FA</a> · <a href="?token=%s&lang=ru">RU</a> · <a href="?token=%s&lang=zh">ZH</a>`,
			"guide_title": "手动连接配置指南",
			"guide_desc":  "对于 Windows、iOS 和 macOS 原生设置：添加 L2TP/IPSec VPN 连接。使用上方的服务器地址，选择用户名/密码身份验证，然后输入预共享密钥 (PSK)。",
		},
	}

	t := translations[lang]
	if t == nil {
		t = translations["en"]
	}

	dir := "ltr"
	if lang == "fa" {
		dir = "rtl"
	}

	usedGB := float64(used) / (1024 * 1024 * 1024)
	totalGBStr := t["unlimited"]
	if maxData > 0 {
		totalGBStr = fmt.Sprintf("%.2f GB", float64(maxData)/(1024*1024*1024))
	}

	langsBar := fmt.Sprintf(t["langs"], token, token, token, token)

	html := fmt.Sprintf(`<!doctype html>
<html lang="%s" dir="%s">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<meta name="referrer" content="no-referrer">
	<title>%s</title>
	<style>
		:root {
			--bg: #030712;
			--panel: rgba(17, 24, 39, 0.7);
			--line: rgba(75, 85, 99, 0.25);
			--cyan: #22d3ee;
			--blue: #3b82f6;
			--green: #10b981;
			--red: #ef4444;
			--text: #f3f4f6;
		}
		* { box-sizing: border-box; }
		body {
			background: radial-gradient(1000px 600px at 70%% -10%%, rgba(59, 130, 246, 0.2), transparent 60%%), linear-gradient(135deg, #030712 0%%, #0f172a 100%%);
			color: var(--text);
			font-family: 'Inter', system-ui, -apple-system, sans-serif;
			margin: 0;
			min-height: 100vh;
			display: grid;
			place-items: center;
			padding: 24px;
		}
		.card {
			background: var(--panel);
			border: 1px solid var(--line);
			border-radius: 24px;
			width: 100%%;
			max-width: 580px;
			padding: 28px;
			box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
			backdrop-filter: blur(16px);
		}
		.brand {
			display: flex;
			align-items: center;
			gap: 12px;
			margin-bottom: 24px;
		}
		.logo {
			background: linear-gradient(135deg, var(--blue), var(--cyan));
			border-radius: 12px;
			width: 38px;
			height: 38px;
			display: grid;
			place-items: center;
			font-weight: 900;
			color: #fff;
		}
		h2 { margin: 0; font-size: 22px; font-weight: 800; letter-spacing: -0.03em; }
		.status-row {
			display: flex;
			justify-content: space-between;
			align-items: center;
			margin-bottom: 18px;
		}
		.pill {
			border-radius: 99px;
			padding: 6px 12px;
			font-size: 11px;
			font-weight: 900;
			text-transform: uppercase;
		}
		.pill.online { background: rgba(16, 185, 129, 0.15); color: var(--green); }
		.pill.offline { background: rgba(75, 85, 99, 0.15); color: #9ca3af; }
		.bar-wrap { margin-bottom: 24px; }
		.bar {
			background: rgba(0, 0, 0, 0.4);
			border-radius: 99px;
			height: 8px;
			overflow: hidden;
			margin-top: 6px;
		}
		.bar i { display: block; height: 100%%; background: linear-gradient(90deg, var(--blue), var(--cyan)); width: %.1f%%; }
		.row {
			display: flex;
			justify-content: space-between;
			align-items: center;
			border-bottom: 1px solid var(--line);
			padding: 12px 0;
		}
		.row b { color: #9ca3af; font-size: 13px; }
		.row span { font-size: 14px; font-weight: 700; word-break: break-all; text-align: right; }
		.btn {
			background: linear-gradient(135deg, var(--blue), #1d4ed8);
			border: 0;
			border-radius: 12px;
			color: #fff;
			display: block;
			width: 100%%;
			padding: 14px;
			font-weight: 900;
			text-align: center;
			text-decoration: none;
			margin-top: 18px;
			cursor: pointer;
		}
		.langs-bar {
			text-align: center;
			margin-bottom: 15px;
			font-size: 12px;
			color: var(--cyan);
		}
		.langs-bar a {
			color: #9ca3af;
			text-decoration: none;
			margin: 0 4px;
			font-weight: bold;
		}
		.langs-bar a:hover {
			color: var(--cyan);
		}
	</style>
</head>
<body>
	<div class="card">
		<div class="langs-bar">
			%s
		</div>
		<div class="brand">
			<div class="logo">K</div>
			<div>
				<h2>%s</h2>
			</div>
		</div>
		<div class="status-row">
			<div>
				<b>%s:</b> <span>%s</span>
			</div>
			<span class="pill %s">%s</span>
		</div>
		<div class="bar-wrap">
			<div class="status-row" style="margin-bottom: 6px;">
				<b>%s</b>
				<span>%.2f GB / %s</span>
			</div>
			<div class="bar"><i></i></div>
		</div>
		<div class="row">
			<b>%s</b>
			<span>%s</span>
		</div>
		<div class="row">
			<b>%s</b>
			<span>Default</span>
		</div>
		<p><a class="btn" href="/api/portal/profiles/openvpn.ovpn?token=%s">%s</a></p>
		<div class="row" style="margin-top: 18px; border-top: 1px solid var(--line); padding-top: 15px;">
			<b style="color: var(--cyan); font-size: 14px; font-weight: 800;">%s</b>
		</div>
		<p class="mu" style="font-size: 13px; line-height: 1.5; margin: 5px 0 0 0; color: var(--text); opacity: 0.85;">%s</p>
	</div>
</body>
</html>`, lang, dir, t["title"], pct, langsBar, t["title"], t["username"], username, map[bool]string{true: "online", false: "offline"}[isOnline], map[bool]string{true: t["online"], false: t["offline"]}[isOnline], t["usage"], usedGB, totalGBStr, t["status"], status, t["l2tp_psk"], token, t["download"], t["guide_title"], t["guide_desc"])

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

func (s *Server) liveSessionsPayload() []map[string]any {
	rows, err := s.DB.Query(`SELECT radacctid, username, COALESCE(framedipaddress,''), acctinputoctets, acctoutputoctets, acctsessiontime FROM radacct WHERE acctstoptime IS NULL`)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()

	now := time.Now()
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	activeIDs := make(map[int64]bool)
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var username, ip string
		var rx, tx, duration int64
		if err := rows.Scan(&id, &username, &ip, &rx, &tx, &duration); err == nil {
			activeIDs[id] = true
			prev, exists := s.prevSessionBytes[id]
			rxSpeed := 0.0
			txSpeed := 0.0
			if exists {
				dt := now.Sub(prev.Timestamp).Seconds()
				if dt > 0.1 {
					rxSpeed = float64(rx-prev.InputBytes) / 1024.0 / dt
					txSpeed = float64(tx-prev.OutputBytes) / 1024.0 / dt
					if rxSpeed < 0 {
						rxSpeed = 0
					}
					if txSpeed < 0 {
						txSpeed = 0
					}
				}
			}
			s.prevSessionBytes[id] = SessionBytes{
				InputBytes:  rx,
				OutputBytes: tx,
				Timestamp:   now,
			}

			out = append(out, map[string]any{
				"id":            id,
				"username":      username,
				"ip":            ip,
				"duration":      duration,
				"rx_bytes":      rx,
				"tx_bytes":      tx,
				"rx_speed_kbps": rxSpeed,
				"tx_speed_kbps": txSpeed,
			})
		}
	}

	// Cleanup stale entries: remove sessions no longer active to prevent memory leak
	for id := range s.prevSessionBytes {
		if !activeIDs[id] {
			delete(s.prevSessionBytes, id)
		}
	}

	return out
}

func (s *Server) bandwidthPayload() []map[string]any {
	rows, err := s.DB.Query(`SELECT username, ip, rx_bps, tx_bps FROM user_bandwidth_snapshots WHERE created_at >= NOW() - INTERVAL 30 SECOND ORDER BY created_at DESC`)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()

	// Group by username, take the most recent entry per user
	seen := make(map[string]bool)
	out := []map[string]any{}
	for rows.Next() {
		var username, ip string
		var rxBps, txBps int64
		if err := rows.Scan(&username, &ip, &rxBps, &txBps); err == nil {
			if !seen[username] {
				seen[username] = true
				out = append(out, map[string]any{
					"username": username,
					"ip":       ip,
					"rx_bps":   rxBps,
					"tx_bps":   txBps,
				})
			}
		}
	}
	return out
}

func (s *Server) killSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	var in struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}

	var sessionID, username, nasIP string
	err := s.DB.QueryRow(`SELECT acctsessionid, username, COALESCE(nasipaddress,'127.0.0.1') FROM radacct WHERE radacctid=? LIMIT 1`, in.ID).Scan(&sessionID, &username, &nasIP)
	if err == sql.ErrNoRows {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "session_not_found"})
		return
	} else if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	// Validate nasIP to prevent command injection (must be a valid IP address)
	if net.ParseIP(nasIP) == nil {
		nasIP = "127.0.0.1"
	}

	go func() {
		attrs := fmt.Sprintf("User-Name=%s,Acct-Session-Id=%s", username, sessionID)
		cmd := exec.Command("radclient", "-x", nasIP+":3799", "disconnect", "testing123")
		cmd.Stdin = strings.NewReader(attrs)
		_ = cmd.Run()
	}()

	_, err = s.DB.Exec(`UPDATE radacct SET acctstoptime=NOW() WHERE radacctid=?`, in.ID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "session.killed", "session", strconv.FormatInt(in.ID, 10), nil, map[string]any{"username": username, "session_id": sessionID}, clientIP(r))
	s.createEvent("session", "warning", fmt.Sprintf("Session terminated: %s", username), fmt.Sprintf("Admin %s terminated VPN session #%d for %s", actor, in.ID, username), actor, username)

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) resellerCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	actor, role, ok := s.currentAdmin(r)
	if !ok || role != "reseller" {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "reseller_only"})
		return
	}

	var in struct {
		Amount float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Amount <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_amount"})
		return
	}

	tx, err := s.DB.Begin()
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(`UPDATE admins SET credit = credit + ? WHERE username=?`, in.Amount, actor)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	desc := fmt.Sprintf("Automatic Gateway Top-up (Cryptomus/Zarinpal): +%.2f IRT", in.Amount)
	_, err = tx.Exec(`INSERT INTO reseller_transactions(reseller_username, amount, type, description, actor) VALUES(?,?, 'allocation', ?, ?)`, actor, in.Amount, desc, actor)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	s.logAudit(actor, "reseller.checkout_completed", "reseller", actor, nil, map[string]any{"username": actor, "amount": in.Amount}, clientIP(r))
	s.createEvent("reseller", "success", fmt.Sprintf("Reseller top-up: %s", actor), fmt.Sprintf("Reseller %s automatically topped up +%.2f IRT via gateway", actor, in.Amount), actor, actor)

	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) resellerTransactions(w http.ResponseWriter, r *http.Request) {
	actor, role, ok := s.currentAdmin(r)
	if !ok {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	where := "1=1"
	args := []any{}
	if role == "reseller" {
		where = "reseller_username = ?"
		args = append(args, actor)
	} else {
		reseller := r.URL.Query().Get("reseller")
		if reseller != "" {
			where = "reseller_username = ?"
			args = append(args, reseller)
		}
	}

	rows, err := s.DB.Query(`SELECT id, reseller_username, amount, type, description, actor, created_at FROM reseller_transactions WHERE `+where+` ORDER BY id DESC LIMIT 500`, args...)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()

	type Tx struct {
		ID          int64   `json:"id"`
		Reseller    string  `json:"reseller_username"`
		Amount      float64 `json:"amount"`
		Type        string  `json:"type"`
		Description string  `json:"description"`
		Actor       string  `json:"actor"`
		CreatedAt   string  `json:"created_at"`
	}

	list := []Tx{}
	for rows.Next() {
		var t Tx
		var created time.Time
		if err := rows.Scan(&t.ID, &t.Reseller, &t.Amount, &t.Type, &t.Description, &t.Actor, &created); err == nil {
			t.CreatedAt = created.Format(time.RFC3339)
			list = append(list, t)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "transactions": list})
}

func (s *Server) disconnectCustomerSessions(username string) {
	rows, err := s.DB.Query(`SELECT radacctid, acctsessionid, COALESCE(nasipaddress,'127.0.0.1') FROM radacct WHERE username=? AND acctstoptime IS NULL`, username)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var sessionID, nasIP string
		if err := rows.Scan(&id, &sessionID, &nasIP); err == nil {
			go func(u, sID, ip string) {
				// Validate nasIP to prevent command injection
				if net.ParseIP(ip) == nil {
					ip = "127.0.0.1"
				}
				attrs := fmt.Sprintf("User-Name=%s,Acct-Session-Id=%s", u, sID)
				cmd := exec.Command("radclient", "-x", ip+":3799", "disconnect", "testing123")
				cmd.Stdin = strings.NewReader(attrs)
				_ = cmd.Run()
			}(username, sessionID, nasIP)

			_, _ = s.DB.Exec(`UPDATE radacct SET acctstoptime=NOW() WHERE radacctid=?`, id)
		}
	}
}

func (s *Server) resellerPayments(w http.ResponseWriter, r *http.Request) {
	actor, role, ok := s.currentAdmin(r)
	if !ok || role != "reseller" {
		writeJSONCode(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "reseller_only"})
		return
	}

	if r.Method == http.MethodPost {
		var in struct {
			Amount      float64 `json:"amount"`
			Description string  `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Amount <= 0 {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_amount"})
			return
		}

		_, err := s.DB.Exec(`INSERT INTO payments(username, amount, method, status, intent_type, admin_note) VALUES(?, ?, 'manual', 'pending', 'reseller_topup', ?)`, actor, in.Amount, in.Description)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}

		s.createEvent("reseller", "info", fmt.Sprintf("Reseller top-up request: %s", actor), fmt.Sprintf("Reseller %s requested +%.2f IRT credit top-up", actor, in.Amount), actor, actor)
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	if r.Method == http.MethodGet {
		rows, err := s.DB.Query(`SELECT id, amount, method, status, COALESCE(admin_note,''), created_at FROM payments WHERE username=? AND intent_type='reseller_topup' ORDER BY id DESC LIMIT 100`, actor)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer rows.Close()

		type Pay struct {
			ID        int64   `json:"id"`
			Amount    float64 `json:"amount"`
			Method    string  `json:"method"`
			Status    string  `json:"status"`
			Note      string  `json:"note"`
			CreatedAt string  `json:"created_at"`
		}
		list := []Pay{}
		for rows.Next() {
			var p Pay
			var created time.Time
			if err := rows.Scan(&p.ID, &p.Amount, &p.Method, &p.Status, &p.Note, &created); err == nil {
				p.CreatedAt = created.Format(time.RFC3339)
				list = append(list, p)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "payments": list})
		return
	}

	http.Error(w, "method", http.StatusMethodNotAllowed)
}

// NoCacheMiddleware sets Cache-Control: no-store on all /api/ responses to prevent
// browser-level caching of dynamic data.
func NoCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	writeJSONCode(w, http.StatusOK, v)
}

func writeJSONCode(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// limitBody wraps the request body with a max size reader (1MB default for JSON endpoints).
// Returns false and writes a 413 error if the limit would be exceeded.
func limitBody(w http.ResponseWriter, r *http.Request, maxBytes int64) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
}

// maxJSONBody is the default max size for JSON request bodies (1MB).
const maxJSONBody int64 = 1 << 20

// ErrorResponse represents a structured error returned by the API.
type ErrorResponse struct {
	Error  string `json:"error"`
	Code   string `json:"code"`
	Status int    `json:"status"`
}

// writeError writes a standardized JSON error response with proper headers.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:  message,
		Code:   code,
		Status: status,
	})
}

// ========== Null-Safety Scanning Helpers ==========

// nullStringPtr converts a sql.NullString to a *string.
// Returns nil if the value is not valid, preserving JSON null serialization.
func nullStringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

// nullInt64Ptr converts a sql.NullInt64 to a *int64.
// Returns nil if the value is not valid, preserving JSON null serialization.
func nullInt64Ptr(ni sql.NullInt64) *int64 {
	if !ni.Valid {
		return nil
	}
	return &ni.Int64
}

// nullTimePtr converts a sql.NullTime to a *string formatted as RFC3339.
// Returns nil if the value is not valid, preserving JSON null serialization.
func nullTimePtr(nt sql.NullTime) *string {
	if !nt.Valid {
		return nil
	}
	s := nt.Time.Format(time.RFC3339)
	return &s
}

// ========== Per-Node VPN Config ==========

type NodeVPNConfig struct {
	ID       int64           `json:"id"`
	NodeID   int64           `json:"node_id"`
	Protocol string          `json:"protocol"`
	Enabled  bool            `json:"enabled"`
	Port     int             `json:"port"`
	Network  string          `json:"network"`
	Extra    json.RawMessage `json:"extra_json,omitempty"`
}

func (s *Server) nodeVPNConfig(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/nodes/vpn-config/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "node_id_required"})
		return
	}
	nodeID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || nodeID <= 0 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_node_id"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getNodeVPNConfigs(w, nodeID)
	case http.MethodPost, http.MethodPatch:
		s.upsertNodeVPNConfig(w, r, nodeID)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) getNodeVPNConfigs(w http.ResponseWriter, nodeID int64) {
	rows, err := s.DB.Query(`SELECT id, node_id, protocol, enabled, port, COALESCE(network,''), extra_json FROM node_vpn_configs WHERE node_id=? ORDER BY FIELD(protocol,'openvpn','l2tp','ikev2','ssh')`, nodeID)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	configs := []NodeVPNConfig{}
	for rows.Next() {
		var c NodeVPNConfig
		var enabled int
		var extra []byte
		if err := rows.Scan(&c.ID, &c.NodeID, &c.Protocol, &enabled, &c.Port, &c.Network, &extra); err == nil {
			c.Enabled = enabled == 1
			c.Extra = extra
			configs = append(configs, c)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "configs": configs})
}

func (s *Server) upsertNodeVPNConfig(w http.ResponseWriter, r *http.Request, nodeID int64) {
	var in struct {
		Protocol string          `json:"protocol"`
		Enabled  bool            `json:"enabled"`
		Port     int             `json:"port"`
		Network  string          `json:"network"`
		Extra    json.RawMessage `json:"extra_json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	if in.Protocol != "openvpn" && in.Protocol != "l2tp" && in.Protocol != "ikev2" && in.Protocol != "ssh" && in.Protocol != "wireguard" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_protocol"})
		return
	}

	// WireGuard-specific validation: stricter port range and network CIDR
	if in.Protocol == "wireguard" {
		if err := wireguard.ValidatePort(in.Port); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_port"})
			return
		}
		if strings.TrimSpace(in.Network) != "" {
			if err := wireguard.ValidateNetworkCIDR(strings.TrimSpace(in.Network)); err != nil {
				writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_network_cidr"})
				return
			}
		}
	}

	if in.Port <= 0 || in.Port > 65535 {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_port"})
		return
	}

	enabledInt := 0
	if in.Enabled {
		enabledInt = 1
	}
	extraStr := ""
	if len(in.Extra) > 0 {
		extraStr = string(in.Extra)
		// Validate outbound config if present
		var extraMap map[string]any
		if err := json.Unmarshal(in.Extra, &extraMap); err == nil {
			if outbound, ok := extraMap["outbound"].(map[string]any); ok {
				if enabled, _ := outbound["enabled"].(bool); enabled {
					oType, _ := outbound["type"].(string)
					validTypes := map[string]bool{"vless": true, "vmess": true, "trojan": true, "shadowsocks": true, "socks5": true}
					if oType != "" && !validTypes[oType] {
						writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_outbound_type"})
						return
					}
				}
			}
		}
	}

	_, err := s.DB.Exec(`INSERT INTO node_vpn_configs(node_id, protocol, enabled, port, network, extra_json)
		VALUES(?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE enabled=VALUES(enabled), port=VALUES(port), network=VALUES(network), extra_json=VALUES(extra_json)`,
		nodeID, in.Protocol, enabledInt, in.Port, strings.TrimSpace(in.Network), nullString(extraStr))
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "node.vpn_config_updated", "node", strconv.FormatInt(nodeID, 10), nil, map[string]any{"protocol": in.Protocol, "port": in.Port, "enabled": in.Enabled}, clientIP(r))

	// Auto-start/stop service on the node when toggled
	serviceMap := map[string]string{
		"openvpn":   "openvpn-server@server",
		"l2tp":      "xl2tpd",
		"ikev2":     "strongswan-starter",
		"ssh":       "ssh",
		"wireguard": "wg-quick@wg0",
	}
	if svcName, ok := serviceMap[in.Protocol]; ok {
		if in.Enabled {
			if in.Protocol == "wireguard" {
				// WireGuard needs setup first (generate keys, create config) before starting
				setupPayload, _ := json.Marshal(map[string]any{
					"port":    in.Port,
					"network": strings.TrimSpace(in.Network),
				})
				_, _ = s.DB.Exec(`INSERT INTO node_tasks(node_id, action, payload_json, status, created_by) VALUES(?, 'wireguard.setup', ?, 'pending', ?)`,
					nodeID, string(setupPayload), actor)
			} else {
				payload, _ := json.Marshal(map[string]any{"service": svcName})
				_, _ = s.DB.Exec(`INSERT INTO node_tasks(node_id, action, payload_json, status, created_by) VALUES(?, 'service.restart', ?, 'pending', ?)`,
					nodeID, string(payload), actor)
			}
		} else {
			payload, _ := json.Marshal(map[string]any{"service": svcName})
			_, _ = s.DB.Exec(`INSERT INTO node_tasks(node_id, action, payload_json, status, created_by) VALUES(?, 'service.stop', ?, 'pending', ?)`,
				nodeID, string(payload), actor)
		}
	}

	writeJSON(w, map[string]any{"ok": true})
}

// ========== Certificates ==========

type VPNCertificate struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	NodeID    *int64 `json:"node_id,omitempty"`
	Content   string `json:"content"`
	IsDefault bool   `json:"is_default"`
	CreatedAt string `json:"created_at"`
}

func (s *Server) certificates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listCertificates(w)
	case http.MethodPost:
		s.uploadCertificate(w, r)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listCertificates(w http.ResponseWriter) {
	rows, err := s.DB.Query(`SELECT id, name, type, node_id, SUBSTRING(content, 1, 80), is_default, created_at FROM vpn_certificates ORDER BY is_default DESC, id DESC`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	certs := []VPNCertificate{}
	for rows.Next() {
		var c VPNCertificate
		var nodeID sql.NullInt64
		var isDefault int
		var created sql.NullTime
		var preview string
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &nodeID, &preview, &isDefault, &created); err == nil {
			if nodeID.Valid {
				c.NodeID = &nodeID.Int64
			}
			c.Content = preview + "..."
			c.IsDefault = isDefault == 1
			if created.Valid {
				c.CreatedAt = created.Time.Format(time.RFC3339)
			}
			certs = append(certs, c)
		}
	}
	writeJSON(w, map[string]any{"ok": true, "certificates": certs})
}

func (s *Server) uploadCertificate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		NodeID    *int64 `json:"node_id"`
		Content   string `json:"content"`
		IsDefault bool   `json:"is_default"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.ToLower(strings.TrimSpace(in.Type))
	in.Content = strings.TrimSpace(in.Content)

	if in.Name == "" || in.Content == "" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "name_content_required"})
		return
	}
	if in.Type != "ca" && in.Type != "tls_crypt" && in.Type != "client_cert" && in.Type != "client_key" {
		writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid_type"})
		return
	}

	defaultInt := 0
	if in.IsDefault {
		defaultInt = 1
		// Unset other defaults of same type
		_, _ = s.DB.Exec(`UPDATE vpn_certificates SET is_default=0 WHERE type=?`, in.Type)
	}

	res, err := s.DB.Exec(`INSERT INTO vpn_certificates(name, type, node_id, content, is_default) VALUES(?,?,?,?,?)`,
		in.Name, in.Type, in.NodeID, in.Content, defaultInt)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()

	actor, _, _ := s.currentAdmin(r)
	s.logAudit(actor, "certificate.uploaded", "certificate", strconv.FormatInt(id, 10), nil, map[string]any{"name": in.Name, "type": in.Type}, clientIP(r))
	writeJSON(w, map[string]any{"ok": true, "id": id})
}

func (s *Server) certificateByID(w http.ResponseWriter, r *http.Request) {
	id, _, ok := pathID(r.URL.Path, "/api/certificates/")
	if !ok {
		writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		var c VPNCertificate
		var nodeID sql.NullInt64
		var isDefault int
		var created sql.NullTime
		err := s.DB.QueryRow(`SELECT id, name, type, node_id, content, is_default, created_at FROM vpn_certificates WHERE id=?`, id).Scan(&c.ID, &c.Name, &c.Type, &nodeID, &c.Content, &isDefault, &created)
		if err == sql.ErrNoRows {
			writeJSONCode(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not_found"})
			return
		}
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if nodeID.Valid {
			c.NodeID = &nodeID.Int64
		}
		c.IsDefault = isDefault == 1
		if created.Valid {
			c.CreatedAt = created.Time.Format(time.RFC3339)
		}
		writeJSON(w, map[string]any{"ok": true, "certificate": c})

	case http.MethodDelete:
		if _, err := s.DB.Exec(`DELETE FROM vpn_certificates WHERE id=?`, id); err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		actor, _, _ := s.currentAdmin(r)
		s.logAudit(actor, "certificate.deleted", "certificate", strconv.FormatInt(id, 10), nil, nil, clientIP(r))
		writeJSON(w, map[string]any{"ok": true})

	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// ========== Panel Settings ==========

func (s *Server) panelSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := s.DB.Query(`SELECT setting_key, setting_value FROM panel_settings ORDER BY setting_key`)
		if err != nil {
			writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		defer rows.Close()
		settings := map[string]string{}
		for rows.Next() {
			var k, v string
			if rows.Scan(&k, &v) == nil {
				settings[k] = v
			}
		}
		writeJSON(w, map[string]any{"ok": true, "settings": settings})

	case http.MethodPatch:
		var in map[string]string
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSONCode(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
			return
		}
		for k, v := range in {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			_, _ = s.DB.Exec(`INSERT INTO panel_settings(setting_key, setting_value) VALUES(?,?) ON DUPLICATE KEY UPDATE setting_value=VALUES(setting_value)`, k, v)
		}
		actor, _, _ := s.currentAdmin(r)
		s.logAudit(actor, "settings.updated", "panel_settings", "", nil, map[string]any{"keys": len(in)}, clientIP(r))
		writeJSON(w, map[string]any{"ok": true})

	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

// publicSettings returns non-sensitive panel settings (theme, mode, panel name, language)
// without requiring authentication. This allows the portal to fetch admin-chosen theme settings.
func (s *Server) publicSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	allowedKeys := map[string]bool{
		"ui_theme":   true,
		"ui_mode":    true,
		"panel_name": true,
		"language":   true,
	}
	rows, err := s.DB.Query(`SELECT setting_key, setting_value FROM panel_settings ORDER BY setting_key`)
	if err != nil {
		writeJSONCode(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer rows.Close()
	settings := map[string]string{}
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			if allowedKeys[k] {
				settings[k] = v
			}
		}
	}
	writeJSON(w, map[string]any{"ok": true, "settings": settings})
}

// checkWSOrigin validates the WebSocket Origin header against allowed origins.
// Empty Origin is allowed (same-origin requests from some browsers).
// The configured PublicBase and AllowedOrigins are checked.
func (s *Server) checkWSOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Same-origin requests may not send Origin
	}

	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}
	originHost := originURL.Host

	// Check against PublicBase
	if s.Config.PublicBase != "" {
		if pubURL, err := url.Parse(s.Config.PublicBase); err == nil {
			if pubURL.Host != "" && strings.EqualFold(pubURL.Host, originHost) {
				return true
			}
		}
	}

	// Check against AllowedOrigins list
	for _, allowed := range s.Config.AllowedOrigins {
		if allowedURL, err := url.Parse(allowed); err == nil {
			if strings.EqualFold(allowedURL.Host, originHost) {
				return true
			}
		}
		// Also allow direct host comparison
		if strings.EqualFold(allowed, originHost) || strings.EqualFold(allowed, origin) {
			return true
		}
	}

	// Check if origin matches the request's Host header (same-origin)
	if strings.EqualFold(originHost, r.Host) {
		return true
	}

	return false
}
