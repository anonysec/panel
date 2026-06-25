-- 060_cores_antidpi.sql
CREATE TABLE IF NOT EXISTS core_plugins (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    version VARCHAR(20) NOT NULL,
    download_url VARCHAR(512) NOT NULL,
    checksum_sha256 VARCHAR(64) NOT NULL,
    protocols_json TEXT NOT NULL,          -- JSON array: ["vless","vmess","trojan"]
    config_template TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY idx_core_name_version (name, version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS node_cores (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    node_id BIGINT NOT NULL,
    core_name VARCHAR(50) NOT NULL,
    version VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, running, stopped, failed
    last_health_at DATETIME DEFAULT NULL,
    installed_at DATETIME DEFAULT NULL,
    UNIQUE KEY idx_node_core (node_id, core_name),
    CONSTRAINT fk_nc_node FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS node_antidpi (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    node_id BIGINT NOT NULL,
    technique VARCHAR(30) NOT NULL,   -- reality, fragment, domain_fronting, warp
    config_json TEXT NOT NULL,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY idx_antidpi_node_tech (node_id, technique),
    CONSTRAINT fk_antidpi_node FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS landing_settings (
    id INT PRIMARY KEY DEFAULT 1,
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    title VARCHAR(255) NOT NULL DEFAULT 'KorisPanel',
    description TEXT,
    logo_url VARCHAR(512) DEFAULT NULL,
    hero_content TEXT,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO landing_settings (id, enabled, title) VALUES (1, 0, 'KorisPanel');
