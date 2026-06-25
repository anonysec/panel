-- Migration 051: Node enhancements — tags, downtimes, bandwidth quotas, geo/alert columns

-- Many-to-many tagging for nodes
CREATE TABLE IF NOT EXISTS node_tags (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  node_id BIGINT NOT NULL,
  tag VARCHAR(50) NOT NULL,
  UNIQUE KEY uk_node_tag (node_id, tag),
  INDEX idx_tag (tag),
  FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- Downtime events for SLA tracking
CREATE TABLE IF NOT EXISTS node_downtimes (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  node_id BIGINT NOT NULL,
  started_at TIMESTAMP NOT NULL,
  ended_at TIMESTAMP NULL,
  duration_seconds INT DEFAULT 0,
  reason VARCHAR(255),
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_node_started (node_id, started_at),
  FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- Monthly bandwidth limits per node
CREATE TABLE IF NOT EXISTS node_bandwidth_quotas (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  node_id BIGINT NOT NULL UNIQUE,
  monthly_limit_gb INT NOT NULL DEFAULT 0,
  current_usage_gb DECIMAL(12,2) DEFAULT 0,
  alert_threshold_pct INT DEFAULT 80,
  reset_day INT DEFAULT 1,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

-- Add geo-location, maintenance mode, and alert threshold columns to nodes
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS latitude DECIMAL(10,7) NULL;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS longitude DECIMAL(10,7) NULL;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS maintenance_mode BOOLEAN DEFAULT FALSE;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS alert_cpu_threshold INT DEFAULT 80;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS alert_ram_threshold INT DEFAULT 90;
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS alert_disk_threshold INT DEFAULT 85;
