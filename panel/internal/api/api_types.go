package api

import (
	"encoding/json"
	"time"
)

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
	CreatedBy   string  `json:"created_by"`
	Avatar      string  `json:"avatar"`
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
	ID                 int64               `json:"id"`
	Name               string              `json:"name"`
	PublicIP           string              `json:"public_ip"`
	Domain             string              `json:"domain"`
	Status             string              `json:"status"`
	HealthScore        float64             `json:"health_score"`
	LastSeenAt         string              `json:"last_seen_at"`
	CreatedAt          string              `json:"created_at"`
	ProxyConfig        json.RawMessage     `json:"proxy_config,omitempty"`
	BandwidthQuotaGB   *int64              `json:"bandwidth_quota_gb"`
	BandwidthUsedBytes int64               `json:"bandwidth_used_bytes"`
	BandwidthResetAt   string              `json:"bandwidth_reset_at,omitempty"`
	StatusMetrics      NodeStatus          `json:"status_metrics"`
	Services           []Service           `json:"services"`
	History            []NodeUsageSnapshot `json:"history,omitempty"`
	Diagnostics        *DiagnosticsReport  `json:"diagnostics,omitempty"`
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
