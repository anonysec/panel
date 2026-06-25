-- 019_dns_failover.sql
-- DNS-based failover system for OpenVPN profiles.
-- Allows using domain names instead of direct IPs in .ovpn configs,
-- enabling transparent server migration when nodes get blocked.

-- DNS provider configurations (e.g., Cloudflare API credentials)
CREATE TABLE IF NOT EXISTS dns_providers (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    type ENUM('cloudflare','manual') NOT NULL DEFAULT 'manual',
    api_token_encrypted VARCHAR(512) NULL,
    zone_id VARCHAR(128) NULL,
    account_id VARCHAR(128) NULL,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Failover domains: maps a domain name to a target node for VPN profiles
CREATE TABLE IF NOT EXISTS failover_domains (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    domain VARCHAR(255) NOT NULL UNIQUE,
    current_node_id BIGINT NOT NULL,
    dns_provider_id BIGINT NULL,
    dns_record_id VARCHAR(128) NULL,
    ttl INT NOT NULL DEFAULT 60,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    last_failover_at DATETIME NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX(current_node_id),
    INDEX(dns_provider_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Failover event log: tracks every DNS failover action
CREATE TABLE IF NOT EXISTS failover_events (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    domain_id BIGINT NOT NULL,
    from_node_id BIGINT NOT NULL,
    to_node_id BIGINT NOT NULL,
    reason VARCHAR(255) NOT NULL DEFAULT '',
    status ENUM('pending','propagating','completed','failed','rolled_back') NOT NULL DEFAULT 'pending',
    dns_propagation_started_at DATETIME NULL,
    dns_propagation_completed_at DATETIME NULL,
    triggered_by VARCHAR(64) NOT NULL DEFAULT 'admin',
    error_message TEXT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX(domain_id),
    INDEX(status),
    INDEX(created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Add failover_domain_id to nodes for quick lookup of which domain points to this node
ALTER TABLE nodes ADD COLUMN failover_domain_id BIGINT NULL AFTER domain;
ALTER TABLE nodes ADD INDEX idx_nodes_failover_domain (failover_domain_id);

-- Store the default failover settings in panel_settings
INSERT IGNORE INTO panel_settings(setting_key, setting_value)
VALUES
    ('dns_failover_enabled', 'false'),
    ('dns_failover_check_interval', '30'),
    ('dns_failover_auto_rollback', 'false'),
    ('dns_failover_propagation_timeout', '300');
