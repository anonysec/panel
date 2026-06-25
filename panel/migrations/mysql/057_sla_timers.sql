-- Migration 057: SLA timers for support tickets
-- Adds sla_alerted_at to track which tickets have already triggered an SLA alert.
-- Stores SLA response targets in panel_settings.

ALTER TABLE tickets ADD COLUMN sla_alerted_at TIMESTAMP NULL DEFAULT NULL;

-- Seed default SLA response targets (minutes)
INSERT INTO panel_settings (setting_key, setting_value) VALUES
  ('sla_response_minutes_low', '480'),
  ('sla_response_minutes_medium', '120'),
  ('sla_response_minutes_high', '30')
ON DUPLICATE KEY UPDATE setting_value = setting_value;
