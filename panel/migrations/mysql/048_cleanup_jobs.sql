-- Migration 048: Cleanup jobs tracking table
-- Tracks automated and manual data cleanup operations

CREATE TABLE IF NOT EXISTS cleanup_jobs (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    status ENUM('pending','running','completed','failed') NOT NULL DEFAULT 'pending',
    targets JSON NOT NULL COMMENT 'Array of cleanup target names',
    config_json JSON NOT NULL COMMENT 'Cleanup configuration (older_than, batch_size, etc.)',
    results_json JSON COMMENT 'Results per target after execution',
    started_at TIMESTAMP NULL,
    completed_at TIMESTAMP NULL,
    created_by VARCHAR(64) NOT NULL COMMENT 'Admin username who initiated',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_status (status),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
