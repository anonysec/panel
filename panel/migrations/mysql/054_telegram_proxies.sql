CREATE TABLE IF NOT EXISTS telegram_proxies (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    node_id BIGINT NOT NULL,
    port INT NOT NULL,
    secret VARCHAR(64) NOT NULL,
    tag VARCHAR(100) DEFAULT '',
    status ENUM('active','stopped','error') DEFAULT 'stopped',
    share_link TEXT,
    tg_link TEXT,
    connections_count INT DEFAULT 0,
    last_health_check TIMESTAMP NULL,
    plan_ids JSON NULL COMMENT 'Array of plan IDs that have access',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE,
    UNIQUE KEY idx_node_port (node_id, port)
);
