-- KorisPanel consolidated PostgreSQL schema
-- Converted from MySQL migrations 000-071

---------------------------------------------------------------------
-- FreeRADIUS tables
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS radcheck (
    id SERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL DEFAULT '',
    attribute VARCHAR(64) NOT NULL DEFAULT '',
    op CHAR(2) NOT NULL DEFAULT ':=',
    value VARCHAR(253) NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_radcheck_username ON radcheck(username);

CREATE TABLE IF NOT EXISTS radreply (
    id SERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL DEFAULT '',
    attribute VARCHAR(64) NOT NULL DEFAULT '',
    op CHAR(2) NOT NULL DEFAULT ':=',
    value VARCHAR(253) NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_radreply_username ON radreply(username);

CREATE TABLE IF NOT EXISTS radacct (
    radacctid BIGSERIAL PRIMARY KEY,
    acctsessionid VARCHAR(64) NOT NULL DEFAULT '',
    acctuniqueid VARCHAR(32) NOT NULL DEFAULT '',
    username VARCHAR(64) NOT NULL DEFAULT '',
    realm VARCHAR(64) DEFAULT '',
    nasipaddress VARCHAR(15) NOT NULL DEFAULT '',
    nasportid VARCHAR(32) DEFAULT NULL,
    nasporttype VARCHAR(32) DEFAULT NULL,
    acctstarttime TIMESTAMPTZ DEFAULT NULL,
    acctupdatetime TIMESTAMPTZ DEFAULT NULL,
    acctstoptime TIMESTAMPTZ DEFAULT NULL,
    acctinterval INT DEFAULT NULL,
    acctsessiontime INT DEFAULT NULL,
    acctauthentic VARCHAR(32) DEFAULT NULL,
    connectinfo_start VARCHAR(128) DEFAULT NULL,
    connectinfo_stop VARCHAR(128) DEFAULT NULL,
    acctinputoctets BIGINT DEFAULT 0,
    acctoutputoctets BIGINT DEFAULT 0,
    calledstationid VARCHAR(64) NOT NULL DEFAULT '',
    callingstationid VARCHAR(64) NOT NULL DEFAULT '',
    acctterminatecause VARCHAR(32) NOT NULL DEFAULT '',
    servicetype VARCHAR(32) DEFAULT NULL,
    framedprotocol VARCHAR(32) DEFAULT NULL,
    framedipaddress VARCHAR(15) NOT NULL DEFAULT '',
    framedipv6address VARCHAR(45) NOT NULL DEFAULT '',
    framedipv6prefix VARCHAR(45) NOT NULL DEFAULT '',
    framedinterfaceid VARCHAR(44) NOT NULL DEFAULT '',
    delegatedipv6prefix VARCHAR(45) NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_radacct_username ON radacct(username);
CREATE INDEX IF NOT EXISTS idx_radacct_acctstarttime ON radacct(acctstarttime);
CREATE INDEX IF NOT EXISTS idx_radacct_acctstoptime ON radacct(acctstoptime);
CREATE INDEX IF NOT EXISTS idx_radacct_nasipaddress ON radacct(nasipaddress);
CREATE INDEX IF NOT EXISTS idx_radacct_acctuniqueid ON radacct(acctuniqueid);
CREATE INDEX IF NOT EXISTS idx_radacct_active ON radacct(username, acctstoptime);
CREATE INDEX IF NOT EXISTS idx_radacct_active_nas ON radacct(nasipaddress, acctstoptime);
CREATE INDEX IF NOT EXISTS idx_radacct_starttime ON radacct(acctstarttime, acctinputoctets, acctoutputoctets);

CREATE TABLE IF NOT EXISTS radpostauth (
    id SERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL DEFAULT '',
    pass VARCHAR(64) NOT NULL DEFAULT '',
    reply VARCHAR(32) NOT NULL DEFAULT '',
    authdate TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_radpostauth_username ON radpostauth(username);

CREATE TABLE IF NOT EXISTS nas (
    id SERIAL PRIMARY KEY,
    nasname VARCHAR(128) NOT NULL,
    shortname VARCHAR(32),
    type VARCHAR(30) DEFAULT 'other',
    ports INT,
    secret VARCHAR(60) NOT NULL DEFAULT 'secret',
    server VARCHAR(64),
    community VARCHAR(50),
    description VARCHAR(200) DEFAULT 'RADIUS Client'
);
CREATE INDEX IF NOT EXISTS idx_nas_nasname ON nas(nasname);


---------------------------------------------------------------------
-- Radacct archive (for sessions older than 90 days)
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS radacct_archive (
    radacctid BIGINT PRIMARY KEY,
    acctsessionid VARCHAR(64) NOT NULL DEFAULT '',
    acctuniqueid VARCHAR(32) NOT NULL DEFAULT '',
    username VARCHAR(64) NOT NULL DEFAULT '',
    realm VARCHAR(64) DEFAULT '',
    nasipaddress VARCHAR(15) NOT NULL DEFAULT '',
    nasportid VARCHAR(32) DEFAULT NULL,
    nasporttype VARCHAR(32) DEFAULT NULL,
    acctstarttime TIMESTAMPTZ DEFAULT NULL,
    acctupdatetime TIMESTAMPTZ DEFAULT NULL,
    acctstoptime TIMESTAMPTZ DEFAULT NULL,
    acctinterval INT DEFAULT NULL,
    acctsessiontime INT DEFAULT NULL,
    acctauthentic VARCHAR(32) DEFAULT NULL,
    connectinfo_start VARCHAR(128) DEFAULT NULL,
    connectinfo_stop VARCHAR(128) DEFAULT NULL,
    acctinputoctets BIGINT DEFAULT 0,
    acctoutputoctets BIGINT DEFAULT 0,
    calledstationid VARCHAR(64) NOT NULL DEFAULT '',
    callingstationid VARCHAR(64) NOT NULL DEFAULT '',
    acctterminatecause VARCHAR(32) NOT NULL DEFAULT '',
    servicetype VARCHAR(32) DEFAULT NULL,
    framedprotocol VARCHAR(32) DEFAULT NULL,
    framedipaddress VARCHAR(15) NOT NULL DEFAULT '',
    framedipv6address VARCHAR(45) NOT NULL DEFAULT '',
    framedipv6prefix VARCHAR(45) NOT NULL DEFAULT '',
    framedinterfaceid VARCHAR(44) NOT NULL DEFAULT '',
    delegatedipv6prefix VARCHAR(45) NOT NULL DEFAULT '',
    archived_at TIMESTAMPTZ DEFAULT NOW()
);

---------------------------------------------------------------------
-- Admin & Auth
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS admins (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    totp_secret VARCHAR(64) DEFAULT '',
    totp_enabled BOOLEAN DEFAULT FALSE,
    role VARCHAR(40) NOT NULL DEFAULT 'admin',
    permissions TEXT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    credit DECIMAL(12,2) NOT NULL DEFAULT 0.00,
    avatar VARCHAR(32) NULL,
    billing_mode VARCHAR(20) NOT NULL DEFAULT 'manual',
    payout_balance DECIMAL(12,2) NOT NULL DEFAULT 0,
    commission_percent DECIMAL(5,2) NOT NULL DEFAULT 0,
    min_payout_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS admin_login_attempts (
    id BIGSERIAL PRIMARY KEY,
    ip VARCHAR(64) NOT NULL,
    username VARCHAR(64) NULL,
    success BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_admin_login_ip ON admin_login_attempts(ip);
CREATE INDEX IF NOT EXISTS idx_admin_login_user ON admin_login_attempts(username);

CREATE TABLE IF NOT EXISTS admin_sessions (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL,
    session_hash VARCHAR(128) NOT NULL,
    ip_address VARCHAR(45) NOT NULL,
    user_agent TEXT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_active_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_admin_session_user ON admin_sessions(username);
CREATE INDEX IF NOT EXISTS idx_admin_session_hash ON admin_sessions(session_hash);
CREATE INDEX IF NOT EXISTS idx_admin_session_expires ON admin_sessions(expires_at);

---------------------------------------------------------------------
-- Customers
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS customers (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    xray_uuid VARCHAR(36),
    display_name VARCHAR(128) NULL,
    email VARCHAR(255) DEFAULT '',
    created_by VARCHAR(64) NULL,
    plan_id BIGINT NULL,
    preferred_node_id INT NULL,
    trial_used BOOLEAN NOT NULL DEFAULT FALSE,
    referral_code VARCHAR(20) NULL,
    referred_by VARCHAR(64) NULL,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    conn_limit INT DEFAULT 0,
    conn_limit_override INT NULL,
    extra_connections INT NOT NULL DEFAULT 0,
    auto_renew BOOLEAN NOT NULL DEFAULT FALSE,
    timezone VARCHAR(50) NULL,
    avatar VARCHAR(32) NULL,
    billing_mode VARCHAR(20) NULL,
    sub_token VARCHAR(96) NULL UNIQUE,
    notes TEXT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_customers_created_by ON customers(created_by);
CREATE INDEX IF NOT EXISTS idx_customers_plan ON customers(plan_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_customers_status ON customers(status, deleted_at);

---------------------------------------------------------------------
-- Plans
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS plans (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    data_gb DECIMAL(12,2) NOT NULL DEFAULT 0,
    speed_mbps DECIMAL(12,2) NOT NULL DEFAULT 0,
    duration_days INT NOT NULL DEFAULT 30,
    duration_hours INT NOT NULL DEFAULT 0,
    price DECIMAL(12,2) NOT NULL DEFAULT 0,
    billing_type VARCHAR(40) NOT NULL DEFAULT 'quota',
    price_per_gb DECIMAL(10,2) NOT NULL DEFAULT 0,
    price_per_day DECIMAL(10,2) NOT NULL DEFAULT 0,
    disconnect_on_zero BOOLEAN NOT NULL DEFAULT TRUE,
    allow_passwordless BOOLEAN NOT NULL DEFAULT FALSE,
    grace_days INT NOT NULL DEFAULT 0,
    currency VARCHAR(10) NOT NULL DEFAULT 'USD',
    default_connections INT NOT NULL DEFAULT 1,
    max_connections INT NOT NULL DEFAULT 3,
    price_per_extra_conn DECIMAL(12,2) NOT NULL DEFAULT 0,
    features TEXT DEFAULT NULL,
    plan_protocols JSONB DEFAULT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

---------------------------------------------------------------------
-- Subscriptions
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS subscriptions (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NULL,
    username VARCHAR(64) NOT NULL,
    plan_id BIGINT NULL,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    started_at TIMESTAMPTZ DEFAULT NOW(),
    first_connect_at TIMESTAMPTZ NULL,
    activate_on_connect BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ NULL,
    paid_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    discount_code VARCHAR(64) NULL
);
CREATE INDEX IF NOT EXISTS idx_subscriptions_customer ON subscriptions(customer_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_username ON subscriptions(username);
CREATE INDEX IF NOT EXISTS idx_subscriptions_plan ON subscriptions(plan_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_subs_expires ON subscriptions(username, expires_at);

---------------------------------------------------------------------
-- Discount Codes
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS discount_codes (
    code VARCHAR(64) PRIMARY KEY,
    percent INT NOT NULL DEFAULT 0,
    amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    max_uses INT NOT NULL DEFAULT 0,
    used INT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

---------------------------------------------------------------------
-- Wallets & Transactions
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS wallets (
    customer_id BIGINT NULL,
    username VARCHAR(64) PRIMARY KEY,
    credit DECIMAL(12,2) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_wallets_customer ON wallets(customer_id);

CREATE TABLE IF NOT EXISTS wallet_transactions (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NULL,
    invoice_id BIGINT NULL,
    username VARCHAR(64) NOT NULL,
    amount DECIMAL(12,2) NOT NULL,
    type VARCHAR(40) NOT NULL DEFAULT 'adjustment',
    description VARCHAR(255) DEFAULT '',
    actor VARCHAR(64) DEFAULT '',
    reference_type VARCHAR(40) NOT NULL DEFAULT '',
    reference_id BIGINT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_wallet_tx_customer ON wallet_transactions(customer_id);
CREATE INDEX IF NOT EXISTS idx_wallet_tx_username ON wallet_transactions(username);
CREATE INDEX IF NOT EXISTS idx_wallet_tx_type ON wallet_transactions(type);
CREATE INDEX IF NOT EXISTS idx_wallet_tx_reference ON wallet_transactions(reference_type, reference_id);
CREATE INDEX IF NOT EXISTS idx_wallet_tx_user ON wallet_transactions(username, id DESC);
CREATE INDEX IF NOT EXISTS idx_wallet_tx_invoice ON wallet_transactions(invoice_id);


---------------------------------------------------------------------
-- Payments & Payment Methods
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS payment_methods (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(80) NOT NULL,
    type VARCHAR(40) NOT NULL DEFAULT 'manual',
    config_json JSONB NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS payments (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NULL,
    username VARCHAR(64) NOT NULL,
    amount DECIMAL(12,2) NOT NULL,
    method VARCHAR(64) DEFAULT 'manual',
    receipt TEXT NULL,
    receipt_file VARCHAR(255) NULL,
    status VARCHAR(40) NOT NULL DEFAULT 'pending',
    gateway_authority VARCHAR(128) DEFAULT '',
    gateway_ref_id VARCHAR(128) DEFAULT '',
    gateway_name VARCHAR(32) DEFAULT '',
    intent_type VARCHAR(40) NOT NULL DEFAULT 'wallet_topup',
    intent_id BIGINT NULL,
    metadata_json JSONB NULL,
    admin_note TEXT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_payments_customer ON payments(customer_id);
CREATE INDEX IF NOT EXISTS idx_payments_username ON payments(username);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_payments_intent ON payments(intent_type, intent_id);

CREATE TABLE IF NOT EXISTS payment_gateways (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    display_name VARCHAR(100) NOT NULL,
    type VARCHAR(40) NOT NULL DEFAULT 'manual',
    config_json JSONB NULL,
    is_active BOOLEAN DEFAULT TRUE,
    sort_order INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS payment_transactions (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL,
    gateway_name VARCHAR(50) NOT NULL,
    reference_id VARCHAR(255) NOT NULL,
    amount DECIMAL(12,2) NOT NULL,
    currency VARCHAR(10) NOT NULL DEFAULT 'IRR',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    callback_data TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pt_customer ON payment_transactions(customer_id);
CREATE INDEX IF NOT EXISTS idx_pt_reference ON payment_transactions(gateway_name, reference_id);

CREATE TABLE IF NOT EXISTS invoices (
    id BIGSERIAL PRIMARY KEY,
    invoice_number VARCHAR(20) NOT NULL UNIQUE,
    customer_id BIGINT NOT NULL,
    transaction_id BIGINT DEFAULT NULL,
    amount DECIMAL(12,2) NOT NULL,
    tax DECIMAL(12,2) NOT NULL DEFAULT 0,
    total DECIMAL(12,2) NOT NULL DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'IRR',
    status VARCHAR(40) DEFAULT 'draft',
    type VARCHAR(40) NOT NULL DEFAULT 'subscription',
    description TEXT NULL,
    plan_id BIGINT NULL,
    plan_name VARCHAR(100) DEFAULT NULL,
    gateway_id BIGINT NULL,
    payment_method VARCHAR(50) DEFAULT NULL,
    payment_ref VARCHAR(100) NULL,
    pdf_path VARCHAR(255) NULL,
    refunded_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    paid_at TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_invoices_customer ON invoices(customer_id);
CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status);

---------------------------------------------------------------------
-- PAYG Billing
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS payg_deductions (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL,
    username VARCHAR(64) NOT NULL,
    plan_id BIGINT NOT NULL,
    deduction_type VARCHAR(20) NOT NULL,
    amount DECIMAL(10,4) NOT NULL,
    usage_value DECIMAL(14,4) NOT NULL,
    balance_before DECIMAL(10,2) NOT NULL,
    balance_after DECIMAL(10,2) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_payg_customer ON payg_deductions(customer_id);
CREATE INDEX IF NOT EXISTS idx_payg_username ON payg_deductions(username);
CREATE INDEX IF NOT EXISTS idx_payg_created ON payg_deductions(created_at);

---------------------------------------------------------------------
-- Data Packs
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS data_packs (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    data_gb DECIMAL(10,2) NOT NULL,
    price DECIMAL(12,2) NOT NULL,
    currency VARCHAR(10) NOT NULL DEFAULT 'USD',
    is_active BOOLEAN DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS customer_data_packs (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL,
    username VARCHAR(64) NOT NULL,
    data_pack_id BIGINT NOT NULL,
    data_bytes BIGINT NOT NULL,
    used_bytes BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    purchased_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_cdp_customer ON customer_data_packs(customer_id, status);
CREATE INDEX IF NOT EXISTS idx_cdp_username ON customer_data_packs(username, status);

---------------------------------------------------------------------
-- Promo Codes & Referrals
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS promo_codes (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(50) NOT NULL UNIQUE,
    type VARCHAR(20) NOT NULL DEFAULT 'percent',
    value DECIMAL(12,2) NOT NULL DEFAULT 0,
    max_uses INT NOT NULL DEFAULT 0,
    used_count INT NOT NULL DEFAULT 0,
    min_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    applicable_plans TEXT NULL,
    starts_at TIMESTAMPTZ NULL,
    expires_at TIMESTAMPTZ NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by VARCHAR(64) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_promo_code ON promo_codes(code);
CREATE INDEX IF NOT EXISTS idx_promo_active ON promo_codes(is_active, expires_at);

CREATE TABLE IF NOT EXISTS promo_usage (
    id BIGSERIAL PRIMARY KEY,
    promo_id BIGINT NOT NULL,
    customer_id BIGINT NOT NULL,
    username VARCHAR(64) NOT NULL,
    discount_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    used_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_promo_usage_promo ON promo_usage(promo_id);
CREATE INDEX IF NOT EXISTS idx_promo_usage_customer ON promo_usage(customer_id);

CREATE TABLE IF NOT EXISTS referrals (
    id BIGSERIAL PRIMARY KEY,
    referrer_username VARCHAR(64) NOT NULL,
    referred_username VARCHAR(64) NOT NULL,
    credit_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    status VARCHAR(40) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    credited_at TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_referral_referrer ON referrals(referrer_username);
CREATE INDEX IF NOT EXISTS idx_referral_referred ON referrals(referred_username);

---------------------------------------------------------------------
-- Plan Changes
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS plan_changes (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL,
    username VARCHAR(64) NOT NULL,
    old_plan_id BIGINT NULL,
    new_plan_id BIGINT NULL,
    change_type VARCHAR(40) NOT NULL,
    prorated_credit DECIMAL(12,2) NOT NULL DEFAULT 0,
    actor VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plan_change_customer ON plan_changes(customer_id);
CREATE INDEX IF NOT EXISTS idx_plan_change_date ON plan_changes(created_at DESC);


---------------------------------------------------------------------
-- Support Tickets
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS tickets (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NULL,
    username VARCHAR(64) NOT NULL DEFAULT '',
    subject VARCHAR(255) NOT NULL,
    category VARCHAR(40) DEFAULT 'general',
    priority VARCHAR(20) NOT NULL DEFAULT 'normal',
    status VARCHAR(40) NOT NULL DEFAULT 'open',
    assigned_to VARCHAR(64) NULL,
    satisfaction_rating SMALLINT NULL,
    sla_alerted_at TIMESTAMPTZ NULL,
    sla_breached BOOLEAN NOT NULL DEFAULT FALSE,
    sla_deadline_at TIMESTAMPTZ DEFAULT NULL,
    auto_close_days INT NOT NULL DEFAULT 7,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    resolved_at TIMESTAMPTZ NULL,
    closed_at TIMESTAMPTZ NULL,
    deleted_at TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_tickets_customer ON tickets(customer_id);
CREATE INDEX IF NOT EXISTS idx_tickets_username ON tickets(username);
CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status);

CREATE TABLE IF NOT EXISTS ticket_messages (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL,
    sender_type VARCHAR(20) NOT NULL,
    sender_name VARCHAR(64) NOT NULL DEFAULT '',
    sender_id VARCHAR(64) NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    is_internal BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ticket_messages_ticket ON ticket_messages(ticket_id);
CREATE INDEX IF NOT EXISTS idx_ticket_messages_sender ON ticket_messages(sender_type);

CREATE TABLE IF NOT EXISTS ticket_attachments (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL,
    filename VARCHAR(255) NOT NULL,
    filepath VARCHAR(512) NOT NULL,
    filesize INT NOT NULL,
    mime_type VARCHAR(100),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS canned_responses (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    body TEXT NOT NULL,
    category VARCHAR(100) NULL,
    usage_count INT NOT NULL DEFAULT 0,
    created_by VARCHAR(64) NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sla_config (
    id BIGSERIAL PRIMARY KEY,
    priority VARCHAR(20) NOT NULL UNIQUE,
    response_minutes INT NOT NULL
);

CREATE TABLE IF NOT EXISTS kb_articles (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    body TEXT NOT NULL,
    category VARCHAR(100) NOT NULL DEFAULT 'general',
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    locale VARCHAR(10) NOT NULL DEFAULT 'en',
    parent_id BIGINT DEFAULT NULL,
    view_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_kb_category ON kb_articles(category);
CREATE INDEX IF NOT EXISTS idx_kb_status ON kb_articles(status);

---------------------------------------------------------------------
-- Reseller System
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS reseller_transactions (
    id BIGSERIAL PRIMARY KEY,
    reseller_username VARCHAR(64) NOT NULL,
    amount DECIMAL(12,2) NOT NULL,
    type VARCHAR(32) NOT NULL,
    description VARCHAR(255) NOT NULL,
    actor VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_reseller_tx ON reseller_transactions(reseller_username);

CREATE TABLE IF NOT EXISTS reseller_plan_prices (
    id BIGSERIAL PRIMARY KEY,
    reseller_id BIGINT NOT NULL,
    plan_id BIGINT NOT NULL,
    sell_price DECIMAL(12,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(reseller_id, plan_id)
);
CREATE INDEX IF NOT EXISTS idx_reseller_prices_reseller ON reseller_plan_prices(reseller_id);
CREATE INDEX IF NOT EXISTS idx_reseller_prices_plan ON reseller_plan_prices(plan_id);

CREATE TABLE IF NOT EXISTS reseller_tickets (
    id BIGSERIAL PRIMARY KEY,
    reseller_username VARCHAR(64) NOT NULL,
    subject VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'open',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_reseller_tickets_user ON reseller_tickets(reseller_username);
CREATE INDEX IF NOT EXISTS idx_reseller_tickets_status ON reseller_tickets(status);

CREATE TABLE IF NOT EXISTS reseller_ticket_messages (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL,
    sender VARCHAR(64) NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_reseller_ticket_msg ON reseller_ticket_messages(ticket_id);

CREATE TABLE IF NOT EXISTS reseller_allowed_plans (
    reseller_id BIGINT NOT NULL,
    plan_id BIGINT NOT NULL,
    PRIMARY KEY (reseller_id, plan_id)
);
CREATE INDEX IF NOT EXISTS idx_reseller_allowed ON reseller_allowed_plans(reseller_id);

CREATE TABLE IF NOT EXISTS reseller_payouts (
    id BIGSERIAL PRIMARY KEY,
    reseller_username VARCHAR(64) NOT NULL,
    amount DECIMAL(12,2) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    payment_details TEXT,
    admin_note TEXT,
    requested_at TIMESTAMPTZ DEFAULT NOW(),
    processed_at TIMESTAMPTZ DEFAULT NULL,
    processed_by VARCHAR(64) DEFAULT NULL
);

---------------------------------------------------------------------
-- Nodes
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS node_groups (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    region VARCHAR(100) NOT NULL DEFAULT '',
    description TEXT,
    load_balancing_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    max_load_percent INT NOT NULL DEFAULT 85,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS nodes (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(64) NOT NULL UNIQUE,
    public_ip VARCHAR(64) NOT NULL,
    domain VARCHAR(255) NULL,
    failover_domain_id BIGINT NULL,
    api_token_hash VARCHAR(128) NOT NULL,
    status VARCHAR(40) NOT NULL DEFAULT 'offline',
    proxy_config TEXT DEFAULT NULL,
    group_id BIGINT NULL REFERENCES node_groups(id) ON DELETE SET NULL,
    max_capacity INT NOT NULL DEFAULT 100,
    bandwidth_quota_gb INT DEFAULT NULL,
    bandwidth_used_bytes BIGINT NOT NULL DEFAULT 0,
    bandwidth_reset_at TIMESTAMPTZ DEFAULT NULL,
    agent_version VARCHAR(20) DEFAULT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    latitude DECIMAL(10,7) NULL,
    longitude DECIMAL(10,7) NULL,
    maintenance_mode BOOLEAN DEFAULT FALSE,
    alert_cpu_threshold INT DEFAULT 80,
    alert_ram_threshold INT DEFAULT 90,
    alert_disk_threshold INT DEFAULT 85,
    alert_conn_threshold INT DEFAULT 0,
    last_seen_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_nodes_failover_domain ON nodes(failover_domain_id);

CREATE TABLE IF NOT EXISTS node_status (
    node_id BIGINT PRIMARY KEY,
    cpu_percent DECIMAL(6,2) DEFAULT 0,
    ram_percent DECIMAL(6,2) DEFAULT 0,
    disk_percent DECIMAL(6,2) DEFAULT 0,
    rx_bps BIGINT DEFAULT 0,
    tx_bps BIGINT DEFAULT 0,
    openvpn_status VARCHAR(24) DEFAULT 'unknown',
    l2tp_status VARCHAR(24) DEFAULT 'unknown',
    ikev2_status VARCHAR(24) DEFAULT 'unknown',
    grpc_connected BOOLEAN DEFAULT FALSE,
    knode_version VARCHAR(50),
    last_metrics_at TIMESTAMPTZ,
    metrics_state VARCHAR(20) DEFAULT 'unknown',
    payload_json JSONB NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS node_services (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL,
    service VARCHAR(40) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'unknown',
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(node_id, service)
);

CREATE TABLE IF NOT EXISTS node_usage_snapshots (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL,
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0,
    online_users INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_node_usage_node ON node_usage_snapshots(node_id);
CREATE INDEX IF NOT EXISTS idx_node_usage_created ON node_usage_snapshots(created_at);

CREATE TABLE IF NOT EXISTS node_tags (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    tag VARCHAR(50) NOT NULL,
    UNIQUE(node_id, tag)
);
CREATE INDEX IF NOT EXISTS idx_node_tags_tag ON node_tags(tag);

CREATE TABLE IF NOT EXISTS node_downtimes (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NULL,
    duration_seconds INT DEFAULT 0,
    reason VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_node_downtimes_node ON node_downtimes(node_id, started_at);

CREATE TABLE IF NOT EXISTS node_bandwidth_quotas (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL UNIQUE REFERENCES nodes(id) ON DELETE CASCADE,
    monthly_limit_gb INT NOT NULL DEFAULT 0,
    current_usage_gb DECIMAL(12,2) DEFAULT 0,
    alert_threshold_pct INT DEFAULT 80,
    reset_day INT DEFAULT 1,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS node_diagnostics (
    node_id BIGINT PRIMARY KEY,
    agent_version VARCHAR(32) NOT NULL DEFAULT '',
    uptime_seconds BIGINT NOT NULL DEFAULT 0,
    go_version VARCHAR(32) NOT NULL DEFAULT '',
    goroutines INT NOT NULL DEFAULT 0,
    mem_alloc_bytes BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);


---------------------------------------------------------------------
-- VPN Configuration
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS vpn_core_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1,
    openvpn_port INT NOT NULL DEFAULT 1194,
    openvpn_protocol VARCHAR(10) NOT NULL DEFAULT 'udp',
    openvpn_network VARCHAR(32) NOT NULL DEFAULT '10.8.0.0/24',
    l2tp_network VARCHAR(32) NOT NULL DEFAULT '10.9.0.0/24',
    ikev2_network VARCHAR(32) NOT NULL DEFAULT '10.10.0.0/24',
    ipsec_psk VARCHAR(128) NULL,
    dns_1 VARCHAR(64) NOT NULL DEFAULT '1.1.1.1',
    dns_2 VARCHAR(64) NOT NULL DEFAULT '8.8.8.8',
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS vpn_profiles (
    id BIGSERIAL PRIMARY KEY,
    type VARCHAR(20) NOT NULL,
    name VARCHAR(80) NOT NULL,
    file_path VARCHAR(255) NULL,
    version INT NOT NULL DEFAULT 1,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_vpn_profiles_type ON vpn_profiles(type);
CREATE INDEX IF NOT EXISTS idx_vpn_profiles_active ON vpn_profiles(is_active);

CREATE TABLE IF NOT EXISTS node_vpn_configs (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL,
    protocol VARCHAR(40) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    port INT NOT NULL DEFAULT 0,
    network VARCHAR(64) NULL,
    network_ipv6 VARCHAR(64) NULL,
    extra_json JSONB NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(node_id, protocol)
);
CREATE INDEX IF NOT EXISTS idx_node_vpn_configs_node ON node_vpn_configs(node_id);

CREATE TABLE IF NOT EXISTS ssh_accounts (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NULL,
    username VARCHAR(64) NOT NULL,
    node_id BIGINT NOT NULL,
    ssh_port INT NOT NULL DEFAULT 22,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    max_connections INT NOT NULL DEFAULT 1,
    expires_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(username, node_id)
);
CREATE INDEX IF NOT EXISTS idx_ssh_accounts_node ON ssh_accounts(node_id);
CREATE INDEX IF NOT EXISTS idx_ssh_accounts_customer ON ssh_accounts(customer_id);
CREATE INDEX IF NOT EXISTS idx_ssh_accounts_status ON ssh_accounts(status);

CREATE TABLE IF NOT EXISTS vpn_certificates (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    type VARCHAR(40) NOT NULL,
    node_id BIGINT NULL,
    content TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ NULL,
    fingerprint VARCHAR(128) NULL,
    cert_path VARCHAR(512) NULL,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_vpn_certs_node ON vpn_certificates(node_id);
CREATE INDEX IF NOT EXISTS idx_vpn_certs_type ON vpn_certificates(type);
CREATE INDEX IF NOT EXISTS idx_vpn_certs_expires ON vpn_certificates(expires_at);
CREATE INDEX IF NOT EXISTS idx_vpn_certs_status ON vpn_certificates(status);

---------------------------------------------------------------------
-- WireGuard
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS wg_peers (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NULL,
    node_id BIGINT NOT NULL,
    public_key VARCHAR(44) NOT NULL,
    preshared_key VARCHAR(44) NULL,
    private_key_encrypted TEXT NULL,
    allowed_ips VARCHAR(128) NOT NULL,
    endpoint VARCHAR(128) NULL,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    last_handshake_at TIMESTAMPTZ NULL,
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(node_id, public_key)
);
CREATE INDEX IF NOT EXISTS idx_wg_peers_customer ON wg_peers(customer_id, status);
CREATE INDEX IF NOT EXISTS idx_wg_peers_node ON wg_peers(node_id);
CREATE INDEX IF NOT EXISTS idx_wg_peers_status ON wg_peers(status);

---------------------------------------------------------------------
-- Xray / VLESS System
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS xray_configs (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL UNIQUE REFERENCES nodes(id) ON DELETE CASCADE,
    enabled BOOLEAN DEFAULT FALSE,
    config_json JSONB NOT NULL,
    reality_config_json JSONB,
    last_synced_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS xray_inbounds (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    uuid VARCHAR(36) NOT NULL UNIQUE,
    protocol VARCHAR(20) NOT NULL,
    transport VARCHAR(20) NOT NULL,
    security VARCHAR(20) NOT NULL DEFAULT 'none',
    port INT NOT NULL,
    server_name VARCHAR(255) DEFAULT NULL,
    public_key VARCHAR(255) DEFAULT NULL,
    short_id VARCHAR(32) DEFAULT NULL,
    private_key VARCHAR(255) DEFAULT NULL,
    path VARCHAR(255) DEFAULT NULL,
    service_name VARCHAR(100) DEFAULT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0,
    core_name VARCHAR(50) NOT NULL DEFAULT 'xray-core',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_xray_inbounds_customer ON xray_inbounds(customer_id);
CREATE INDEX IF NOT EXISTS idx_xray_inbounds_node ON xray_inbounds(node_id);

CREATE TABLE IF NOT EXISTS xray_templates (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    config_json JSONB NOT NULL,
    category VARCHAR(50) DEFAULT 'general',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

---------------------------------------------------------------------
-- Core Plugins & Anti-DPI
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS core_plugins (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    version VARCHAR(20) NOT NULL,
    download_url VARCHAR(512) NOT NULL,
    checksum_sha256 VARCHAR(64) NOT NULL,
    protocols_json TEXT NOT NULL,
    config_template TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(name, version)
);

CREATE TABLE IF NOT EXISTS node_cores (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    core_name VARCHAR(50) NOT NULL,
    version VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    last_health_at TIMESTAMPTZ DEFAULT NULL,
    installed_at TIMESTAMPTZ DEFAULT NULL,
    UNIQUE(node_id, core_name)
);

CREATE TABLE IF NOT EXISTS node_antidpi (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    technique VARCHAR(30) NOT NULL,
    config_json TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(node_id, technique)
);

CREATE TABLE IF NOT EXISTS anti_dpi_configs (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL UNIQUE REFERENCES nodes(id) ON DELETE CASCADE,
    method VARCHAR(40) DEFAULT 'none',
    port INT DEFAULT 0,
    bridge_address VARCHAR(255),
    cert_fingerprint VARCHAR(255),
    enabled BOOLEAN DEFAULT FALSE,
    extra_settings JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

---------------------------------------------------------------------
-- MTProto & Telegram Proxies
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS mtproto_proxies (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL UNIQUE REFERENCES nodes(id) ON DELETE CASCADE,
    port INT NOT NULL DEFAULT 443,
    secret VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    connections INT NOT NULL DEFAULT 0,
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS telegram_proxies (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    port INT NOT NULL,
    secret VARCHAR(64) NOT NULL,
    tag VARCHAR(100) DEFAULT '',
    status VARCHAR(20) DEFAULT 'stopped',
    share_link TEXT,
    tg_link TEXT,
    connections_count INT DEFAULT 0,
    last_health_check TIMESTAMPTZ NULL,
    plan_ids JSONB NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(node_id, port)
);

---------------------------------------------------------------------
-- AnyConnect
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS anyconnect_nodes (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL UNIQUE REFERENCES nodes(id) ON DELETE CASCADE,
    port INT NOT NULL DEFAULT 443,
    cert_path VARCHAR(512) DEFAULT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS anyconnect_sessions (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    username VARCHAR(64) NOT NULL,
    connected_at TIMESTAMPTZ NOT NULL,
    disconnected_at TIMESTAMPTZ DEFAULT NULL,
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0
);

---------------------------------------------------------------------
-- API Keys & Logs
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS api_keys (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(80) NOT NULL UNIQUE,
    key_hash VARCHAR(128) NOT NULL,
    scopes TEXT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last4 VARCHAR(8) NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS api_logs (
    id BIGSERIAL PRIMARY KEY,
    key_name VARCHAR(80) NULL,
    action VARCHAR(80) NULL,
    ip VARCHAR(64) NULL,
    success BOOLEAN NOT NULL DEFAULT FALSE,
    message TEXT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_api_logs_key ON api_logs(key_name);
CREATE INDEX IF NOT EXISTS idx_api_logs_action ON api_logs(action);
CREATE INDEX IF NOT EXISTS idx_api_logs_created ON api_logs(created_at);


---------------------------------------------------------------------
-- Events & Audit
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS events (
    id BIGSERIAL PRIMARY KEY,
    type VARCHAR(40) NOT NULL,
    severity VARCHAR(20) NOT NULL DEFAULT 'info',
    title VARCHAR(160) NOT NULL,
    message TEXT NULL,
    actor VARCHAR(64) DEFAULT '',
    related VARCHAR(128) DEFAULT '',
    seen BOOLEAN NOT NULL DEFAULT FALSE,
    notified BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
CREATE INDEX IF NOT EXISTS idx_events_severity ON events(severity);
CREATE INDEX IF NOT EXISTS idx_events_seen ON events(seen);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at);

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor VARCHAR(64) NOT NULL,
    action VARCHAR(80) NOT NULL,
    entity_type VARCHAR(40) NOT NULL,
    entity_id VARCHAR(80) NULL,
    before_json JSONB NULL,
    after_json JSONB NULL,
    ip VARCHAR(64) NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_logs(actor);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_entity ON audit_logs(entity_type);
CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at);

---------------------------------------------------------------------
-- Deleted Archive (soft-delete)
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS deleted_archive (
    id BIGSERIAL PRIMARY KEY,
    type VARCHAR(32) NOT NULL,
    name VARCHAR(128) NOT NULL,
    archive_key VARCHAR(128) NULL,
    payload TEXT NULL,
    created_by VARCHAR(64) NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    restored_at TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_deleted_type ON deleted_archive(type);
CREATE INDEX IF NOT EXISTS idx_deleted_name ON deleted_archive(name);
CREATE INDEX IF NOT EXISTS idx_deleted_created ON deleted_archive(created_at);

---------------------------------------------------------------------
-- Settings
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS settings (
    name VARCHAR(80) PRIMARY KEY,
    value TEXT NULL,
    type VARCHAR(32) DEFAULT 'string',
    group_name VARCHAR(64) DEFAULT 'general',
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS panel_settings (
    setting_key VARCHAR(64) PRIMARY KEY,
    setting_value TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

---------------------------------------------------------------------
-- Bandwidth Rules & QoS
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS bandwidth_rules (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    download_kbps INT NOT NULL DEFAULT 0,
    upload_kbps INT NOT NULL DEFAULT 0,
    priority VARCHAR(20) NOT NULL DEFAULT 'normal',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

---------------------------------------------------------------------
-- Firewall Rules
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS firewall_rules (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NULL,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(40) NOT NULL,
    direction VARCHAR(20) NOT NULL DEFAULT 'forward',
    source VARCHAR(200) NULL,
    destination VARCHAR(200) NULL,
    protocol VARCHAR(20) NULL,
    port VARCHAR(50) NULL,
    action VARCHAR(20) NOT NULL DEFAULT 'drop',
    priority INT NOT NULL DEFAULT 100,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_fw_node ON firewall_rules(node_id, is_active, priority);

---------------------------------------------------------------------
-- Bandwidth Snapshots
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS user_bandwidth_snapshots (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL,
    username VARCHAR(64) NOT NULL,
    ip VARCHAR(64) NOT NULL,
    rx_bps BIGINT NOT NULL DEFAULT 0,
    tx_bps BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bw_snap_node ON user_bandwidth_snapshots(node_id);
CREATE INDEX IF NOT EXISTS idx_bw_snap_user ON user_bandwidth_snapshots(username);
CREATE INDEX IF NOT EXISTS idx_bw_snap_created ON user_bandwidth_snapshots(created_at);

---------------------------------------------------------------------
-- User Templates
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS user_templates (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    plan_id BIGINT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    connection_limit INT NOT NULL DEFAULT 0,
    radius_checks JSONB NULL,
    radius_replies JSONB NULL,
    created_by VARCHAR(64) NOT NULL,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_templates_deleted ON user_templates(deleted_at);

---------------------------------------------------------------------
-- Agent Releases
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS agent_releases (
    id BIGSERIAL PRIMARY KEY,
    version VARCHAR(32) NOT NULL UNIQUE,
    binary_path VARCHAR(512) NOT NULL,
    checksum_sha256 VARCHAR(64) NOT NULL,
    released_at TIMESTAMPTZ DEFAULT NOW()
);

---------------------------------------------------------------------
-- DNS Failover
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS dns_providers (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(40) NOT NULL DEFAULT 'manual',
    api_token_encrypted VARCHAR(512) NULL,
    zone_id VARCHAR(128) NULL,
    account_id VARCHAR(128) NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS failover_domains (
    id BIGSERIAL PRIMARY KEY,
    domain VARCHAR(255) NOT NULL UNIQUE,
    current_node_id BIGINT NOT NULL,
    dns_provider_id BIGINT NULL,
    dns_record_id VARCHAR(128) NULL,
    ttl INT NOT NULL DEFAULT 60,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    last_failover_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_failover_node ON failover_domains(current_node_id);
CREATE INDEX IF NOT EXISTS idx_failover_provider ON failover_domains(dns_provider_id);

CREATE TABLE IF NOT EXISTS failover_events (
    id BIGSERIAL PRIMARY KEY,
    domain_id BIGINT NOT NULL,
    from_node_id BIGINT NOT NULL,
    to_node_id BIGINT NOT NULL,
    reason VARCHAR(255) NOT NULL DEFAULT '',
    status VARCHAR(40) NOT NULL DEFAULT 'pending',
    dns_propagation_started_at TIMESTAMPTZ NULL,
    dns_propagation_completed_at TIMESTAMPTZ NULL,
    triggered_by VARCHAR(64) NOT NULL DEFAULT 'admin',
    error_message TEXT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_failover_events_domain ON failover_events(domain_id);
CREATE INDEX IF NOT EXISTS idx_failover_events_status ON failover_events(status);
CREATE INDEX IF NOT EXISTS idx_failover_events_created ON failover_events(created_at);

---------------------------------------------------------------------
-- Health Monitor
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS health_scores (
    id BIGSERIAL PRIMARY KEY,
    score INT NOT NULL,
    trend VARCHAR(16) NOT NULL DEFAULT 'stable',
    checks_json JSONB NOT NULL,
    generated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_health_scores_generated ON health_scores(generated_at);

CREATE TABLE IF NOT EXISTS healing_rules (
    id BIGSERIAL PRIMARY KEY,
    rule_key VARCHAR(80) NOT NULL UNIQUE,
    display_name VARCHAR(128) NOT NULL,
    condition_type VARCHAR(80) NOT NULL,
    action_mode VARCHAR(20) NOT NULL DEFAULT 'auto_fix',
    cooldown_seconds INT NOT NULL DEFAULT 300,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    thresholds_json JSONB NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS healing_actions (
    id BIGSERIAL PRIMARY KEY,
    rule_key VARCHAR(80) NOT NULL,
    resource_type VARCHAR(40) NOT NULL,
    resource_id VARCHAR(80) NOT NULL,
    action_performed VARCHAR(128) NOT NULL,
    result_status VARCHAR(20) NOT NULL,
    error_message TEXT NULL,
    execution_ms INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_healing_actions_rule ON healing_actions(rule_key);
CREATE INDEX IF NOT EXISTS idx_healing_actions_resource ON healing_actions(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_healing_actions_result ON healing_actions(result_status);
CREATE INDEX IF NOT EXISTS idx_healing_actions_created ON healing_actions(created_at);

CREATE TABLE IF NOT EXISTS anomaly_events (
    id BIGSERIAL PRIMARY KEY,
    anomaly_type VARCHAR(80) NOT NULL,
    detected_value DECIMAL(12,4) NOT NULL,
    baseline_value DECIMAL(12,4) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    metadata_json JSONB NULL,
    correlated_incident_id BIGINT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_anomaly_type ON anomaly_events(anomaly_type);
CREATE INDEX IF NOT EXISTS idx_anomaly_severity ON anomaly_events(severity);
CREATE INDEX IF NOT EXISTS idx_anomaly_correlated ON anomaly_events(correlated_incident_id);
CREATE INDEX IF NOT EXISTS idx_anomaly_created ON anomaly_events(created_at);

---------------------------------------------------------------------
-- Webhooks
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS webhooks (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    url VARCHAR(512) NOT NULL,
    secret VARCHAR(128) NULL,
    events TEXT NOT NULL DEFAULT 'all',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    last_triggered_at TIMESTAMPTZ NULL,
    last_status INT NULL,
    fail_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_webhook_active ON webhooks(is_active);

CREATE TABLE IF NOT EXISTS webhook_logs (
    id BIGSERIAL PRIMARY KEY,
    webhook_id BIGINT NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    payload_json TEXT NOT NULL,
    response_status INT NULL,
    response_body TEXT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_webhook_log_wh ON webhook_logs(webhook_id, created_at DESC);

---------------------------------------------------------------------
-- Alert Rules
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS alert_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(40) NOT NULL,
    condition_json TEXT NULL,
    channels VARCHAR(200) NOT NULL DEFAULT 'telegram',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    cooldown_minutes INT NOT NULL DEFAULT 30,
    last_fired_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

---------------------------------------------------------------------
-- Backups
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS backups (
    id BIGSERIAL PRIMARY KEY,
    filename VARCHAR(255) NOT NULL,
    status VARCHAR(40) NOT NULL DEFAULT 'in_progress',
    type VARCHAR(40) NOT NULL DEFAULT 'manual',
    size_bytes BIGINT NULL,
    checksum VARCHAR(64) NULL,
    nodes_included JSONB NULL,
    nodes_skipped JSONB NULL,
    error_message TEXT NULL,
    started_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_backups_status ON backups(status);
CREATE INDEX IF NOT EXISTS idx_backups_started ON backups(started_at);

---------------------------------------------------------------------
-- Cleanup Jobs
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS cleanup_jobs (
    id BIGSERIAL PRIMARY KEY,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    targets JSONB NOT NULL,
    config_json JSONB NOT NULL,
    results_json JSONB,
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    created_by VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_cleanup_status ON cleanup_jobs(status);
CREATE INDEX IF NOT EXISTS idx_cleanup_created ON cleanup_jobs(created_at);

---------------------------------------------------------------------
-- Theme Presets
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS theme_presets (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    mode VARCHAR(10) NOT NULL DEFAULT 'light',
    config_json JSONB NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_by VARCHAR(64),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

---------------------------------------------------------------------
-- Statistics (aggregation tables)
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS bandwidth_hourly (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    hour_start TIMESTAMPTZ NOT NULL,
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0,
    peak_rx_bps BIGINT NOT NULL DEFAULT 0,
    peak_tx_bps BIGINT NOT NULL DEFAULT 0,
    online_users_avg INT NOT NULL DEFAULT 0,
    online_users_peak INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(node_id, hour_start)
);
CREATE INDEX IF NOT EXISTS idx_bandwidth_hourly_start ON bandwidth_hourly(hour_start);

CREATE TABLE IF NOT EXISTS revenue_daily (
    id BIGSERIAL PRIMARY KEY,
    day_date DATE NOT NULL UNIQUE,
    total_revenue DECIMAL(12,2) NOT NULL DEFAULT 0,
    subscription_revenue DECIMAL(12,2) NOT NULL DEFAULT 0,
    topup_revenue DECIMAL(12,2) NOT NULL DEFAULT 0,
    refund_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    new_customers INT NOT NULL DEFAULT 0,
    churned_customers INT NOT NULL DEFAULT 0,
    active_customers INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS protocol_usage_daily (
    id BIGSERIAL PRIMARY KEY,
    day_date DATE NOT NULL,
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    protocol VARCHAR(20) NOT NULL,
    session_count INT NOT NULL DEFAULT 0,
    total_bytes BIGINT NOT NULL DEFAULT 0,
    unique_users INT NOT NULL DEFAULT 0,
    UNIQUE(node_id, day_date, protocol)
);
CREATE INDEX IF NOT EXISTS idx_protocol_usage_date ON protocol_usage_daily(day_date);

---------------------------------------------------------------------
-- User Tags & Segments
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS user_tags (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    color VARCHAR(7) NOT NULL DEFAULT '#3b82f6',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS customer_tags (
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    tag_id BIGINT NOT NULL REFERENCES user_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (customer_id, tag_id)
);

CREATE TABLE IF NOT EXISTS filter_presets (
    id BIGSERIAL PRIMARY KEY,
    admin_username VARCHAR(64) NOT NULL,
    name VARCHAR(100) NOT NULL,
    filters_json TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(admin_username, name)
);

CREATE TABLE IF NOT EXISTS custom_fields (
    id BIGSERIAL PRIMARY KEY,
    field_name VARCHAR(100) NOT NULL UNIQUE,
    field_type VARCHAR(20) NOT NULL DEFAULT 'text',
    field_options TEXT NULL,
    required BOOLEAN DEFAULT FALSE,
    display_order INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS customer_custom_values (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    field_id BIGINT NOT NULL REFERENCES custom_fields(id) ON DELETE CASCADE,
    field_value TEXT,
    UNIQUE(customer_id, field_id)
);

CREATE TABLE IF NOT EXISTS user_notes (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    admin_username VARCHAR(64) NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_notes_customer ON user_notes(customer_id);

CREATE TABLE IF NOT EXISTS user_segments (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    rules_json JSONB NOT NULL,
    customer_count INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

---------------------------------------------------------------------
-- Landing Page
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS landing_settings (
    id INT PRIMARY KEY DEFAULT 1,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    title VARCHAR(255) NOT NULL DEFAULT 'KorisPanel',
    description TEXT,
    logo_url VARCHAR(512) DEFAULT NULL,
    hero_content TEXT,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

---------------------------------------------------------------------
-- Cluster
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS cluster_nodes (
    id VARCHAR(64) PRIMARY KEY,
    role VARCHAR(20) DEFAULT 'follower',
    last_heartbeat TIMESTAMPTZ NULL,
    started_at TIMESTAMPTZ DEFAULT NOW(),
    metadata JSONB
);

---------------------------------------------------------------------
-- knode gRPC Integration (new tables from 064-069)
---------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS knode_connections (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    address VARCHAR(255) NOT NULL,
    grpc_port INT NOT NULL DEFAULT 62050,
    api_key_enc BYTEA NOT NULL,
    client_cert TEXT NOT NULL,
    client_key_enc BYTEA NOT NULL,
    ca_cert TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    status VARCHAR(20) NOT NULL DEFAULT 'offline',
    last_seen_at TIMESTAMPTZ,
    owner_worker VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS node_metrics_history (
    time TIMESTAMPTZ NOT NULL,
    node_id BIGINT NOT NULL REFERENCES knode_connections(id),
    cpu_percent DOUBLE PRECISION,
    ram_percent DOUBLE PRECISION,
    disk_percent DOUBLE PRECISION,
    rx_bps BIGINT,
    tx_bps BIGINT,
    active_sessions INT,
    uptime_seconds BIGINT
);
CREATE INDEX IF NOT EXISTS idx_node_metrics_history_node_time
    ON node_metrics_history(node_id, time DESC);

CREATE TABLE IF NOT EXISTS user_traffic_log (
    time TIMESTAMPTZ NOT NULL,
    user_id BIGINT NOT NULL,
    node_id BIGINT NOT NULL,
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_user_traffic_log_user_time
    ON user_traffic_log(user_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_user_traffic_log_node_time
    ON user_traffic_log(node_id, time DESC);

CREATE TABLE IF NOT EXISTS panel_sessions (
    token VARCHAR(64) PRIMARY KEY,
    admin_id BIGINT,
    customer_id BIGINT,
    data JSONB,
    ip_address VARCHAR(45),
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON panel_sessions(expires_at);

CREATE TABLE IF NOT EXISTS sync_failures (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL,
    core_type VARCHAR(50) NOT NULL,
    error_msg TEXT NOT NULL,
    payload JSONB,
    attempts INT NOT NULL DEFAULT 2,
    resolved BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS outbound_tunnels (
    id BIGSERIAL PRIMARY KEY,
    node_id BIGINT NOT NULL REFERENCES knode_connections(id),
    tunnel_id VARCHAR(100) NOT NULL,
    protocol VARCHAR(50) NOT NULL,
    exit_addr VARCHAR(255) NOT NULL,
    exit_port INT NOT NULL,
    state VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

---------------------------------------------------------------------
-- Seed Data
---------------------------------------------------------------------

INSERT INTO vpn_core_settings(id) VALUES(1) ON CONFLICT DO NOTHING;

INSERT INTO landing_settings (id, enabled, title) VALUES (1, FALSE, 'KorisPanel') ON CONFLICT DO NOTHING;

INSERT INTO panel_settings(setting_key, setting_value) VALUES
    ('panel_name', 'KorisPanel'),
    ('panel_description', 'VPN Management Panel'),
    ('theme', 'dark'),
    ('default_theme', 'dark'),
    ('allow_user_theme', 'true'),
    ('ssh_enabled', 'true'),
    ('ssh_default_port', '22'),
    ('telegram_enabled', 'false'),
    ('telegram_bot_token', ''),
    ('telegram_chat_id', ''),
    ('data_warning_thresholds', '[80, 95]'),
    ('dns_failover_enabled', 'false'),
    ('dns_failover_check_interval', '30'),
    ('dns_failover_auto_rollback', 'false'),
    ('dns_failover_propagation_timeout', '300'),
    ('vpn_domain', ''),
    ('passwordless_configs_enabled', 'false'),
    ('default_currency', 'USD'),
    ('toman_rate', '50000'),
    ('trial_enabled', 'false'),
    ('trial_days', '3'),
    ('trial_plan_id', ''),
    ('referral_enabled', 'false'),
    ('referral_credit', '5.00'),
    ('referral_type', 'fixed'),
    ('auto_renew_enabled', 'true'),
    ('maintenance_mode', 'false'),
    ('maintenance_message', 'System is under maintenance. Please try again later.'),
    ('maintenance_ends_at', ''),
    ('qos_enabled', 'false'),
    ('qos_default_priority', 'normal'),
    ('qos_gaming_priority_mark', '0x1'),
    ('firewall_enabled', 'false'),
    ('blocked_countries', ''),
    ('backup_schedule', 'daily:02'),
    ('backup_retention_count', '7')
ON CONFLICT (setting_key) DO NOTHING;

INSERT INTO settings (name, value, type, group_name) VALUES
    ('health_monitor_check_interval', '60', 'number', 'health'),
    ('health_monitor_alert_interval', '30', 'number', 'health'),
    ('health_monitor_score_retention_days', '30', 'number', 'health'),
    ('health_monitor_healing_log_retention_days', '90', 'number', 'health'),
    ('health_monitor_anomaly_multiplier', '3', 'number', 'health'),
    ('health_monitor_dedup_window_minutes', '15', 'number', 'health'),
    ('health_monitor_correlation_window_minutes', '2', 'number', 'health'),
    ('health_monitor_daily_report_hour', '8', 'number', 'health'),
    ('health_monitor_weekly_report_day', '1', 'number', 'health')
ON CONFLICT (name) DO NOTHING;

INSERT INTO healing_rules (rule_key, display_name, condition_type, action_mode, cooldown_seconds, thresholds_json) VALUES
    ('stale_sessions', 'Stale Session Cleanup', 'stale_sessions', 'auto_fix', 300, '{"stale_minutes": 5}'),
    ('vpn_crash_openvpn', 'OpenVPN Service Crash', 'vpn_service_crash', 'auto_fix', 300, '{"service": "openvpn"}'),
    ('vpn_crash_l2tp', 'L2TP Service Crash', 'vpn_service_crash', 'auto_fix', 300, '{"service": "l2tp"}'),
    ('vpn_crash_ikev2', 'IKEv2 Service Crash', 'vpn_service_crash', 'auto_fix', 300, '{"service": "ikev2"}'),
    ('disk_critical', 'Disk Usage Critical', 'disk_usage', 'alert_only', 600, '{"critical_percent": 90}'),
    ('memory_critical', 'Memory Usage Critical', 'memory_usage', 'alert_only', 600, '{"critical_percent": 95}'),
    ('node_offline_failover', 'Node Offline Failover', 'node_offline', 'auto_fix', 600, '{"offline_minutes": 10}')
ON CONFLICT (rule_key) DO NOTHING;

INSERT INTO alert_rules (name, type, channels, cooldown_minutes) VALUES
    ('Node Offline', 'node_down', 'telegram', 5),
    ('User at 95% Data', 'high_usage', 'telegram', 1440),
    ('Subscription Expiring (3 days)', 'expiry_warning', 'telegram', 1440)
ON CONFLICT DO NOTHING;

INSERT INTO sla_config (priority, response_minutes) VALUES
    ('urgent', 60), ('high', 240), ('normal', 1440), ('low', 4320)
ON CONFLICT (priority) DO NOTHING;
