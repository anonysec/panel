-- Per-node VPN configuration. Each node can have its own protocol settings.
-- Replaces the single global vpn_core_settings for multi-node deployments.
CREATE TABLE IF NOT EXISTS node_vpn_configs (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    node_id BIGINT NOT NULL,
    protocol ENUM('openvpn','l2tp','ikev2','ssh') NOT NULL,
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    port INT NOT NULL DEFAULT 0,
    network VARCHAR(64) NULL,
    extra_json JSON NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY node_protocol (node_id, protocol),
    INDEX(node_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- SSH VPN accounts (for SSH tunnel protocol support)
CREATE TABLE IF NOT EXISTS ssh_accounts (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    customer_id BIGINT NULL,
    username VARCHAR(64) NOT NULL,
    node_id BIGINT NOT NULL,
    ssh_port INT NOT NULL DEFAULT 22,
    status ENUM('active','disabled','expired') NOT NULL DEFAULT 'active',
    max_connections INT NOT NULL DEFAULT 1,
    expires_at DATETIME NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY user_node (username, node_id),
    INDEX(node_id), INDEX(customer_id), INDEX(status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Static certificates storage (for OpenVPN configs that work across servers)
CREATE TABLE IF NOT EXISTS vpn_certificates (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    type ENUM('ca','tls_crypt','client_cert','client_key') NOT NULL,
    node_id BIGINT NULL,
    content TEXT NOT NULL,
    is_default TINYINT(1) NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX(node_id), INDEX(type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Insert default configs for existing nodes (migrate from global settings)
INSERT IGNORE INTO node_vpn_configs (node_id, protocol, enabled, port, network, extra_json)
SELECT n.id, 'openvpn', 1, COALESCE(v.openvpn_port, 1194),
       COALESCE(v.openvpn_network, '10.8.0.0/24'),
       JSON_OBJECT('protocol', COALESCE(v.openvpn_protocol, 'udp'), 'dns_1', COALESCE(v.dns_1, '1.1.1.1'), 'dns_2', COALESCE(v.dns_2, '8.8.8.8'))
FROM nodes n
CROSS JOIN vpn_core_settings v
WHERE v.id = 1
ON DUPLICATE KEY UPDATE port=VALUES(port);

INSERT IGNORE INTO node_vpn_configs (node_id, protocol, enabled, port, network)
SELECT n.id, 'l2tp', 1, 1701, COALESCE(v.l2tp_network, '10.9.0.0/24')
FROM nodes n
CROSS JOIN vpn_core_settings v
WHERE v.id = 1
ON DUPLICATE KEY UPDATE port=VALUES(port);

INSERT IGNORE INTO node_vpn_configs (node_id, protocol, enabled, port, network)
SELECT n.id, 'ikev2', 1, 500, COALESCE(v.ikev2_network, '10.10.0.0/24')
FROM nodes n
CROSS JOIN vpn_core_settings v
WHERE v.id = 1
ON DUPLICATE KEY UPDATE port=VALUES(port);

INSERT IGNORE INTO node_vpn_configs (node_id, protocol, enabled, port)
SELECT n.id, 'ssh', 0, 22
FROM nodes n
ON DUPLICATE KEY UPDATE node_id=VALUES(node_id);
