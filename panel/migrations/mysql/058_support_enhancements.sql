-- Migration 058: Support Enhancements
-- Requirements: 14.1, 15.1, 16.1, 17.1

CREATE TABLE IF NOT EXISTS canned_responses (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    title VARCHAR(200) NOT NULL,
    body TEXT NOT NULL,
    category VARCHAR(100) NOT NULL DEFAULT 'general',
    usage_count INT NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS sla_config (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    priority VARCHAR(20) NOT NULL,
    response_minutes INT NOT NULL,
    UNIQUE KEY idx_sla_priority (priority)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO sla_config (priority, response_minutes) VALUES
    ('urgent', 60), ('high', 240), ('normal', 1440), ('low', 4320);

ALTER TABLE tickets ADD COLUMN sla_breached TINYINT(1) NOT NULL DEFAULT 0;
ALTER TABLE tickets ADD COLUMN sla_deadline_at DATETIME DEFAULT NULL;
ALTER TABLE tickets ADD COLUMN auto_close_days INT NOT NULL DEFAULT 7;

CREATE TABLE IF NOT EXISTS kb_articles (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    body TEXT NOT NULL,
    category VARCHAR(100) NOT NULL DEFAULT 'general',
    status VARCHAR(20) NOT NULL DEFAULT 'draft',  -- draft, published
    locale VARCHAR(10) NOT NULL DEFAULT 'en',
    parent_id BIGINT DEFAULT NULL,  -- for i18n: points to the primary-language article
    view_count INT NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY idx_kb_category (category),
    KEY idx_kb_status (status),
    FULLTEXT KEY idx_kb_search (title, body)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
