-- Migration 027: Passwordless VPN config support
-- Adds global setting and per-plan toggle for passwordless config generation

-- Global setting for passwordless configs (default: disabled)
INSERT IGNORE INTO panel_settings (setting_key, setting_value)
VALUES ('passwordless_configs_enabled', 'false');

-- Per-plan: allow passwordless config generation
ALTER TABLE plans ADD COLUMN IF NOT EXISTS allow_passwordless TINYINT(1) NOT NULL DEFAULT 0 AFTER disconnect_on_zero;
