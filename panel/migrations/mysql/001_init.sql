-- Koris Next clean schema. No brand prefixes.
-- FreeRADIUS tables remain standard: radcheck, radreply, radacct, radpostauth, nas.

CREATE TABLE IF NOT EXISTS admins (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(64) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  role ENUM('owner','admin','support') NOT NULL DEFAULT 'admin',
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS admin_login_attempts (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  ip VARCHAR(64) NOT NULL,
  username VARCHAR(64) NULL,
  success TINYINT(1) NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX(ip), INDEX(username)
);

CREATE TABLE IF NOT EXISTS customers (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(64) NOT NULL UNIQUE,
  display_name VARCHAR(128) NULL,
  created_by VARCHAR(64) NULL,
  plan_id BIGINT NULL,
  status ENUM('active','disabled','expired','limited','deleted') NOT NULL DEFAULT 'active',
  sub_token VARCHAR(96) NULL UNIQUE,
  notes TEXT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  deleted_at DATETIME NULL,
  INDEX(created_by), INDEX(plan_id), INDEX(status)
);

CREATE TABLE IF NOT EXISTS plans (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  data_gb DECIMAL(12,2) NOT NULL DEFAULT 0,
  duration_days INT NOT NULL DEFAULT 30,
  price DECIMAL(12,2) NOT NULL DEFAULT 0,
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  sort_order INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS discount_codes (
  code VARCHAR(64) PRIMARY KEY,
  percent INT NOT NULL DEFAULT 0,
  amount DECIMAL(12,2) NOT NULL DEFAULT 0,
  max_uses INT NOT NULL DEFAULT 0,
  used INT NOT NULL DEFAULT 0,
  expires_at DATETIME NULL,
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS subscriptions (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  customer_id BIGINT NULL,
  username VARCHAR(64) NOT NULL,
  plan_id BIGINT NULL,
  status ENUM('active','expired','cancelled') NOT NULL DEFAULT 'active',
  started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  expires_at DATETIME NULL,
  paid_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
  discount_code VARCHAR(64) NULL,
  INDEX(customer_id), INDEX(username), INDEX(plan_id), INDEX(status)
);

CREATE TABLE IF NOT EXISTS wallets (
  customer_id BIGINT NULL,
  username VARCHAR(64) PRIMARY KEY,
  credit DECIMAL(12,2) NOT NULL DEFAULT 0,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX(customer_id)
);

CREATE TABLE IF NOT EXISTS wallet_transactions (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  customer_id BIGINT NULL,
  username VARCHAR(64) NOT NULL,
  amount DECIMAL(12,2) NOT NULL,
  type ENUM('topup','purchase','refund','adjustment','commission') NOT NULL DEFAULT 'adjustment',
  description VARCHAR(255) DEFAULT '',
  actor VARCHAR(64) DEFAULT '',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX(customer_id), INDEX(username), INDEX(type)
);

CREATE TABLE IF NOT EXISTS payment_methods (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(80) NOT NULL,
  type VARCHAR(40) NOT NULL DEFAULT 'manual',
  config_json JSON NULL,
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  sort_order INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS payments (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  customer_id BIGINT NULL,
  username VARCHAR(64) NOT NULL,
  amount DECIMAL(12,2) NOT NULL,
  method VARCHAR(64) DEFAULT 'manual',
  receipt TEXT NULL,
  receipt_file VARCHAR(255) NULL,
  status ENUM('pending','approved','rejected','cancelled') NOT NULL DEFAULT 'pending',
  admin_note TEXT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX(customer_id), INDEX(username), INDEX(status)
);

CREATE TABLE IF NOT EXISTS tickets (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  customer_id BIGINT NULL,
  username VARCHAR(64) NOT NULL,
  subject VARCHAR(160) NOT NULL,
  status ENUM('open','closed') NOT NULL DEFAULT 'open',
  priority ENUM('low','normal','high') NOT NULL DEFAULT 'normal',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  closed_at DATETIME NULL,
  deleted_at DATETIME NULL,
  INDEX(customer_id), INDEX(username), INDEX(status)
);

CREATE TABLE IF NOT EXISTS ticket_messages (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  ticket_id BIGINT NOT NULL,
  sender_type ENUM('customer','admin','system') NOT NULL,
  sender_name VARCHAR(64) NOT NULL,
  message TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX(ticket_id), INDEX(sender_type)
);

CREATE TABLE IF NOT EXISTS nodes (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(64) NOT NULL UNIQUE,
  public_ip VARCHAR(64) NOT NULL,
  domain VARCHAR(255) NULL,
  api_token_hash VARCHAR(128) NOT NULL,
  status ENUM('online','offline','stale','disabled') NOT NULL DEFAULT 'offline',
  last_seen_at DATETIME NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

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
  payload_json JSON NULL,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS node_services (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  node_id BIGINT NOT NULL,
  service VARCHAR(40) NOT NULL,
  status VARCHAR(24) NOT NULL DEFAULT 'unknown',
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY node_service_unique (node_id, service)
);

CREATE TABLE IF NOT EXISTS node_usage_snapshots (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  node_id BIGINT NOT NULL,
  rx_bytes BIGINT NOT NULL DEFAULT 0,
  tx_bytes BIGINT NOT NULL DEFAULT 0,
  online_users INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX(node_id), INDEX(created_at)
);

CREATE TABLE IF NOT EXISTS vpn_core_settings (
  id TINYINT PRIMARY KEY DEFAULT 1,
  openvpn_port INT NOT NULL DEFAULT 1194,
  openvpn_protocol ENUM('udp','tcp') NOT NULL DEFAULT 'udp',
  openvpn_network VARCHAR(32) NOT NULL DEFAULT '10.8.0.0/24',
  l2tp_network VARCHAR(32) NOT NULL DEFAULT '10.9.0.0/24',
  ikev2_network VARCHAR(32) NOT NULL DEFAULT '10.10.0.0/24',
  ipsec_psk VARCHAR(128) NULL,
  dns_1 VARCHAR(64) NOT NULL DEFAULT '1.1.1.1',
  dns_2 VARCHAR(64) NOT NULL DEFAULT '8.8.8.8',
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS vpn_profiles (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  type ENUM('openvpn','l2tp','ikev2') NOT NULL,
  name VARCHAR(80) NOT NULL,
  file_path VARCHAR(255) NULL,
  version INT NOT NULL DEFAULT 1,
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX(type), INDEX(is_active)
);

CREATE TABLE IF NOT EXISTS api_keys (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(80) NOT NULL UNIQUE,
  key_hash VARCHAR(128) NOT NULL,
  scopes TEXT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  last4 VARCHAR(8) NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  last_used_at DATETIME NULL
);

CREATE TABLE IF NOT EXISTS api_logs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  key_name VARCHAR(80) NULL,
  action VARCHAR(80) NULL,
  ip VARCHAR(64) NULL,
  success TINYINT(1) NOT NULL DEFAULT 0,
  message TEXT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX(key_name), INDEX(action), INDEX(created_at)
);

CREATE TABLE IF NOT EXISTS events (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  type VARCHAR(40) NOT NULL,
  severity ENUM('info','warning','error') NOT NULL DEFAULT 'info',
  title VARCHAR(160) NOT NULL,
  message TEXT NULL,
  actor VARCHAR(64) DEFAULT '',
  related VARCHAR(128) DEFAULT '',
  seen TINYINT(1) NOT NULL DEFAULT 0,
  notified TINYINT(1) NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX(type), INDEX(severity), INDEX(seen), INDEX(created_at)
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  actor VARCHAR(64) NOT NULL,
  action VARCHAR(80) NOT NULL,
  entity_type VARCHAR(40) NOT NULL,
  entity_id VARCHAR(80) NULL,
  before_json JSON NULL,
  after_json JSON NULL,
  ip VARCHAR(64) NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX(actor), INDEX(action), INDEX(entity_type), INDEX(created_at)
);

CREATE TABLE IF NOT EXISTS deleted_archive (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  type VARCHAR(32) NOT NULL,
  name VARCHAR(128) NOT NULL,
  archive_key VARCHAR(128) NULL,
  payload LONGTEXT NULL,
  created_by VARCHAR(64) NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  restored_at DATETIME NULL,
  INDEX(type), INDEX(name), INDEX(created_at)
);

CREATE TABLE IF NOT EXISTS settings (
  name VARCHAR(80) PRIMARY KEY,
  value TEXT NULL,
  type VARCHAR(32) DEFAULT 'string',
  group_name VARCHAR(64) DEFAULT 'general',
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

INSERT IGNORE INTO vpn_core_settings(id) VALUES(1);
