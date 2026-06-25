-- Backup system tables and settings
CREATE TABLE IF NOT EXISTS backups (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    filename VARCHAR(255) NOT NULL,
    status ENUM('in_progress','completed','failed') NOT NULL DEFAULT 'in_progress',
    type ENUM('manual','scheduled','pre_restore') NOT NULL DEFAULT 'manual',
    size_bytes BIGINT NULL,
    checksum VARCHAR(64) NULL,
    nodes_included JSON NULL,
    nodes_skipped JSON NULL,
    error_message TEXT NULL,
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP NULL,
    INDEX idx_status (status),
    INDEX idx_started_at (started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO panel_settings (setting_key, setting_value)
VALUES ('backup_schedule', 'daily:02'),
       ('backup_retention_count', '7')
ON DUPLICATE KEY UPDATE setting_key = setting_key;
