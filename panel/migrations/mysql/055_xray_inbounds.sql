-- Migration 055: Xray per-user inbound configurations.
-- Stores per-customer Xray inbound settings including protocol, transport,
-- Reality/TLS security fields, and traffic counters.

CREATE TABLE IF NOT EXISTS xray_inbounds (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    customer_id BIGINT NOT NULL,
    node_id BIGINT NOT NULL,
    uuid VARCHAR(36) NOT NULL,
    protocol VARCHAR(20) NOT NULL,
    transport VARCHAR(20) NOT NULL,
    security VARCHAR(20) NOT NULL DEFAULT 'none',
    port INT NOT NULL,
    -- Reality settings
    server_name VARCHAR(255) DEFAULT NULL,
    public_key VARCHAR(255) DEFAULT NULL,
    short_id VARCHAR(32) DEFAULT NULL,
    private_key VARCHAR(255) DEFAULT NULL,
    -- Transport settings
    path VARCHAR(255) DEFAULT NULL,
    service_name VARCHAR(100) DEFAULT NULL,
    -- Status
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0,
    core_name VARCHAR(50) NOT NULL DEFAULT 'xray-core',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY idx_xray_uuid (uuid),
    KEY idx_xray_customer (customer_id),
    KEY idx_xray_node (node_id),
    CONSTRAINT fk_xray_customer FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE,
    CONSTRAINT fk_xray_node FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
