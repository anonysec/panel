-- Migration 046: Firewall rules management

CREATE TABLE IF NOT EXISTS firewall_rules (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  node_id BIGINT NULL,
  name VARCHAR(100) NOT NULL,
  type ENUM('block_country','block_ip','rate_limit','allow','custom') NOT NULL,
  direction ENUM('input','output','forward') NOT NULL DEFAULT 'forward',
  source VARCHAR(200) NULL,
  destination VARCHAR(200) NULL,
  protocol VARCHAR(20) NULL,
  port VARCHAR(50) NULL,
  action ENUM('accept','drop','reject','limit') NOT NULL DEFAULT 'drop',
  priority INT NOT NULL DEFAULT 100,
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_fw_node (node_id, is_active, priority)
);

-- Country block list (ISO 3166 codes)
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('firewall_enabled', 'false');
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('blocked_countries', '');
