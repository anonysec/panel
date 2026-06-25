-- Migration 034: Trial period support
-- Admin can enable trial for new users with configurable days

INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('trial_enabled', 'false');
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('trial_days', '3');
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('trial_plan_id', '');

-- Track which users have used their trial
ALTER TABLE customers ADD COLUMN IF NOT EXISTS trial_used TINYINT(1) NOT NULL DEFAULT 0 AFTER preferred_node_id;
