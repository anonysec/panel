CREATE TABLE IF NOT EXISTS cluster_nodes (
    id VARCHAR(64) PRIMARY KEY,
    role ENUM('leader','follower') DEFAULT 'follower',
    last_heartbeat TIMESTAMP NULL,
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    metadata JSON
);
