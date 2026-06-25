-- 020_ai_health_monitor.sql
-- AI Health Monitor tables for diagnostics, auto-healing, and anomaly detection.

CREATE TABLE IF NOT EXISTS health_scores (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    score INT NOT NULL,                           -- 0-100
    trend VARCHAR(16) NOT NULL DEFAULT 'stable',  -- improving, stable, degrading
    checks_json JSON NOT NULL,                    -- full check results array
    generated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX(generated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS healing_rules (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    rule_key VARCHAR(80) NOT NULL UNIQUE,          -- e.g. "stale_sessions", "vpn_crash"
    display_name VARCHAR(128) NOT NULL,
    condition_type VARCHAR(80) NOT NULL,           -- detection condition identifier
    action_mode ENUM('auto_fix','alert_only') NOT NULL DEFAULT 'auto_fix',
    cooldown_seconds INT NOT NULL DEFAULT 300,     -- 5 minutes default
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    thresholds_json JSON NULL,                    -- configurable thresholds per rule
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS healing_actions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    rule_key VARCHAR(80) NOT NULL,
    resource_type VARCHAR(40) NOT NULL,            -- "session", "node", "service"
    resource_id VARCHAR(80) NOT NULL,
    action_performed VARCHAR(128) NOT NULL,
    result_status ENUM('success','partial','failure') NOT NULL,
    error_message TEXT NULL,
    execution_ms INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX(rule_key),
    INDEX(resource_type, resource_id),
    INDEX(result_status),
    INDEX(created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS anomaly_events (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    anomaly_type VARCHAR(80) NOT NULL,            -- "failed_logins", "disconnections", etc.
    detected_value DECIMAL(12,4) NOT NULL,
    baseline_value DECIMAL(12,4) NOT NULL,
    severity ENUM('warning','critical') NOT NULL,
    metadata_json JSON NULL,                      -- additional context (IP ranges, node IDs, etc.)
    correlated_incident_id BIGINT NULL,           -- links related anomalies
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX(anomaly_type),
    INDEX(severity),
    INDEX(correlated_incident_id),
    INDEX(created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Seed default healing rules
INSERT INTO healing_rules (rule_key, display_name, condition_type, action_mode, cooldown_seconds, thresholds_json) VALUES
('stale_sessions', 'Stale Session Cleanup', 'stale_sessions', 'auto_fix', 300, '{"stale_minutes": 5}'),
('vpn_crash_openvpn', 'OpenVPN Service Crash', 'vpn_service_crash', 'auto_fix', 300, '{"service": "openvpn"}'),
('vpn_crash_l2tp', 'L2TP Service Crash', 'vpn_service_crash', 'auto_fix', 300, '{"service": "l2tp"}'),
('vpn_crash_ikev2', 'IKEv2 Service Crash', 'vpn_service_crash', 'auto_fix', 300, '{"service": "ikev2"}'),
('disk_critical', 'Disk Usage Critical', 'disk_usage', 'alert_only', 600, '{"critical_percent": 90}'),
('memory_critical', 'Memory Usage Critical', 'memory_usage', 'alert_only', 600, '{"critical_percent": 95}'),
('node_offline_failover', 'Node Offline Failover', 'node_offline', 'auto_fix', 600, '{"offline_minutes": 10}');

-- Panel settings for health monitor configuration
INSERT IGNORE INTO settings (name, value, type, group_name) VALUES
('health_monitor_check_interval', '60', 'number', 'health'),
('health_monitor_alert_interval', '30', 'number', 'health'),
('health_monitor_score_retention_days', '30', 'number', 'health'),
('health_monitor_healing_log_retention_days', '90', 'number', 'health'),
('health_monitor_anomaly_multiplier', '3', 'number', 'health'),
('health_monitor_dedup_window_minutes', '15', 'number', 'health'),
('health_monitor_correlation_window_minutes', '2', 'number', 'health'),
('health_monitor_daily_report_hour', '8', 'number', 'health'),
('health_monitor_weekly_report_day', '1', 'number', 'health');
