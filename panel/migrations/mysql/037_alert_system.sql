-- Migration 037: Alert rules and uptime monitoring

CREATE TABLE IF NOT EXISTS alert_rules (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  type ENUM('node_down','high_usage','expiry_warning','custom') NOT NULL,
  condition_json TEXT NULL,
  channels VARCHAR(200) NOT NULL DEFAULT 'telegram',
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  cooldown_minutes INT NOT NULL DEFAULT 30,
  last_fired_at TIMESTAMP NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Default alert rules
INSERT IGNORE INTO alert_rules (name, type, channels, cooldown_minutes)
VALUES ('Node Offline', 'node_down', 'telegram', 5);

INSERT IGNORE INTO alert_rules (name, type, channels, cooldown_minutes)
VALUES ('User at 95% Data', 'high_usage', 'telegram', 1440);

INSERT IGNORE INTO alert_rules (name, type, channels, cooldown_minutes)
VALUES ('Subscription Expiring (3 days)', 'expiry_warning', 'telegram', 1440);
