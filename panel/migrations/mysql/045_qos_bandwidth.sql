-- Migration 045: QoS and bandwidth control schema

-- Per-user bandwidth rules (admin can set custom speeds)
CREATE TABLE IF NOT EXISTS bandwidth_rules (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(64) NOT NULL,
  download_kbps INT NOT NULL DEFAULT 0,
  upload_kbps INT NOT NULL DEFAULT 0,
  priority ENUM('low','normal','high','gaming') NOT NULL DEFAULT 'normal',
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE INDEX idx_bw_rule_user (username)
);

-- QoS settings
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('qos_enabled', 'false');
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('qos_default_priority', 'normal');
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('qos_gaming_priority_mark', '0x1');
