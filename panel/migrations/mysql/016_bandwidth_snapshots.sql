-- Per-user bandwidth snapshots for real-time display
CREATE TABLE IF NOT EXISTS user_bandwidth_snapshots (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    node_id BIGINT NOT NULL,
    username VARCHAR(64) NOT NULL,
    ip VARCHAR(64) NOT NULL,
    rx_bps BIGINT NOT NULL DEFAULT 0,
    tx_bps BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX(node_id),
    INDEX(username),
    INDEX(created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
