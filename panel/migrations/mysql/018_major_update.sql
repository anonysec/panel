-- 018_major_update.sql
-- Panel Major Update: user templates, node diagnostics, agent releases, data warning thresholds.

CREATE TABLE IF NOT EXISTS user_templates (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL UNIQUE,
  plan_id BIGINT NULL,
  status ENUM('active','disabled') NOT NULL DEFAULT 'active',
  connection_limit INT NOT NULL DEFAULT 0,
  radius_checks JSON NULL,
  radius_replies JSON NULL,
  created_by VARCHAR(64) NOT NULL,
  deleted_at DATETIME NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX(deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS node_diagnostics (
  node_id BIGINT PRIMARY KEY,
  agent_version VARCHAR(32) NOT NULL DEFAULT '',
  uptime_seconds BIGINT NOT NULL DEFAULT 0,
  go_version VARCHAR(32) NOT NULL DEFAULT '',
  goroutines INT NOT NULL DEFAULT 0,
  mem_alloc_bytes BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS agent_releases (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  version VARCHAR(32) NOT NULL UNIQUE,
  binary_path VARCHAR(512) NOT NULL,
  checksum_sha256 VARCHAR(64) NOT NULL,
  released_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Add data warning thresholds to panel_settings
INSERT IGNORE INTO panel_settings(setting_key, setting_value)
VALUES ('data_warning_thresholds', '[80, 95]');
