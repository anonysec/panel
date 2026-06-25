-- Migration 039: Auto-renewal support
-- Customers can opt-in to auto-renew from wallet when subscription expires

ALTER TABLE customers ADD COLUMN IF NOT EXISTS auto_renew TINYINT(1) NOT NULL DEFAULT 0 AFTER conn_limit_override;
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('auto_renew_enabled', 'true');
