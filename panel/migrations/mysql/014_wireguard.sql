-- WireGuard support: extend protocol enum and create peers table

ALTER TABLE node_vpn_configs
    MODIFY COLUMN protocol ENUM('openvpn','l2tp','ikev2','ssh','wireguard') NOT NULL;

CREATE TABLE IF NOT EXISTS wg_peers (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    customer_id BIGINT NULL,
    node_id BIGINT NOT NULL,
    public_key VARCHAR(44) NOT NULL,
    preshared_key VARCHAR(44) NULL,
    private_key_encrypted TEXT NULL,
    allowed_ips VARCHAR(128) NOT NULL,
    endpoint VARCHAR(128) NULL,
    status ENUM('active','disabled','revoked') NOT NULL DEFAULT 'active',
    last_handshake_at DATETIME NULL,
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY node_pubkey (node_id, public_key),
    INDEX (customer_id),
    INDEX (node_id),
    INDEX (status)
);
