CREATE TABLE IF NOT EXISTS anti_dpi_configs (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    node_id BIGINT NOT NULL UNIQUE,
    method ENUM('none','obfs4','quic','ws_tunnel') DEFAULT 'none',
    port INT DEFAULT 0,
    bridge_address VARCHAR(255),
    cert_fingerprint VARCHAR(255),
    enabled BOOLEAN DEFAULT FALSE,
    extra_settings JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);
