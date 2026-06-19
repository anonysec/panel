-- Migration 033: Business & Billing features
-- Grace period, promo codes, multi-currency support

-- Grace period: days after expiry where user still has access (limited)
ALTER TABLE plans ADD COLUMN IF NOT EXISTS grace_days INT NOT NULL DEFAULT 0 AFTER allow_passwordless;

-- Promo codes / discount coupons
CREATE TABLE IF NOT EXISTS promo_codes (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  code VARCHAR(50) NOT NULL UNIQUE,
  type ENUM('percent', 'fixed') NOT NULL DEFAULT 'percent',
  value DECIMAL(12,2) NOT NULL DEFAULT 0,
  max_uses INT NOT NULL DEFAULT 0,
  used_count INT NOT NULL DEFAULT 0,
  min_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
  applicable_plans TEXT NULL,
  starts_at TIMESTAMP NULL,
  expires_at TIMESTAMP NULL,
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  created_by VARCHAR(64) NOT NULL DEFAULT 'admin',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_promo_code (code),
  INDEX idx_promo_active (is_active, expires_at)
);

-- Promo code usage tracking
CREATE TABLE IF NOT EXISTS promo_usage (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  promo_id BIGINT NOT NULL,
  customer_id BIGINT NOT NULL,
  username VARCHAR(64) NOT NULL,
  discount_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
  used_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_promo_usage_promo (promo_id),
  INDEX idx_promo_usage_customer (customer_id)
);

-- Multi-currency: per-plan currency and global default
ALTER TABLE plans ADD COLUMN IF NOT EXISTS currency VARCHAR(10) NOT NULL DEFAULT 'USD' AFTER grace_days;
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('default_currency', 'USD');
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('toman_rate', '50000');
