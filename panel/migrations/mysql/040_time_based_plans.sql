-- Migration 040: Time-based plans (hourly/daily/weekly) and data packs

-- Plans can now be time-based (billing_type = 'time')
-- duration_hours allows sub-day plans (1 hour, 6 hours, etc.)
ALTER TABLE plans ADD COLUMN IF NOT EXISTS duration_hours INT NOT NULL DEFAULT 0 AFTER duration_days;

-- Data packs: one-time purchasable add-ons that stack with existing plan
CREATE TABLE IF NOT EXISTS data_packs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  data_gb DECIMAL(10,2) NOT NULL,
  price DECIMAL(12,2) NOT NULL,
  currency VARCHAR(10) NOT NULL DEFAULT 'USD',
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  sort_order INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Track purchased data packs per user
CREATE TABLE IF NOT EXISTS customer_data_packs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  customer_id BIGINT NOT NULL,
  username VARCHAR(64) NOT NULL,
  data_pack_id BIGINT NOT NULL,
  data_bytes BIGINT NOT NULL,
  used_bytes BIGINT NOT NULL DEFAULT 0,
  status ENUM('active','exhausted','expired') NOT NULL DEFAULT 'active',
  purchased_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  expires_at TIMESTAMP NULL,
  INDEX idx_cdp_customer (customer_id, status),
  INDEX idx_cdp_username (username, status)
);
