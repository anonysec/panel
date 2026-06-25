-- Migration 055: Xray/VLESS system tables.
-- Adds xray_configs table for per-node Xray configuration
-- and xray_uuid column to customers for VLESS/VMess user identification.

CREATE TABLE IF NOT EXISTS xray_configs (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    node_id BIGINT NOT NULL UNIQUE,
    enabled BOOLEAN DEFAULT FALSE,
    config_json JSON NOT NULL,
    reality_config_json JSON,
    last_synced_at TIMESTAMP NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
);

ALTER TABLE customers ADD COLUMN IF NOT EXISTS xray_uuid VARCHAR(36) AFTER username;
