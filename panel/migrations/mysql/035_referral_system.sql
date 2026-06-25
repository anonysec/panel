-- Migration 035: Referral system
-- Users can refer others and earn credit

-- Referral settings
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('referral_enabled', 'false');
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('referral_credit', '5.00');
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('referral_type', 'fixed');

-- Per-customer referral code
ALTER TABLE customers ADD COLUMN IF NOT EXISTS referral_code VARCHAR(20) NULL AFTER trial_used;
ALTER TABLE customers ADD COLUMN IF NOT EXISTS referred_by VARCHAR(64) NULL AFTER referral_code;

-- Referral tracking
CREATE TABLE IF NOT EXISTS referrals (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  referrer_username VARCHAR(64) NOT NULL,
  referred_username VARCHAR(64) NOT NULL,
  credit_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
  status ENUM('pending','credited','expired') NOT NULL DEFAULT 'pending',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  credited_at TIMESTAMP NULL,
  INDEX idx_referral_referrer (referrer_username),
  INDEX idx_referral_referred (referred_username)
);
