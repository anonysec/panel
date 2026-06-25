-- Migration 049: Statistics aggregation tables
-- Pre-aggregated data for the statistics dashboard

CREATE TABLE IF NOT EXISTS bandwidth_hourly (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    node_id BIGINT NOT NULL,
    hour_start DATETIME NOT NULL,
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0,
    peak_rx_bps BIGINT NOT NULL DEFAULT 0,
    peak_tx_bps BIGINT NOT NULL DEFAULT 0,
    online_users_avg INT NOT NULL DEFAULT 0,
    online_users_peak INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY idx_node_hour (node_id, hour_start),
    KEY idx_hour_start (hour_start),
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS revenue_daily (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    day_date DATE NOT NULL,
    total_revenue DECIMAL(12,2) NOT NULL DEFAULT 0,
    subscription_revenue DECIMAL(12,2) NOT NULL DEFAULT 0,
    topup_revenue DECIMAL(12,2) NOT NULL DEFAULT 0,
    refund_amount DECIMAL(12,2) NOT NULL DEFAULT 0,
    new_customers INT NOT NULL DEFAULT 0,
    churned_customers INT NOT NULL DEFAULT 0,
    active_customers INT NOT NULL DEFAULT 0,
    UNIQUE KEY idx_day (day_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS protocol_usage_daily (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    day_date DATE NOT NULL,
    node_id BIGINT NOT NULL,
    protocol VARCHAR(20) NOT NULL,
    session_count INT NOT NULL DEFAULT 0,
    total_bytes BIGINT NOT NULL DEFAULT 0,
    unique_users INT NOT NULL DEFAULT 0,
    UNIQUE KEY idx_node_day_proto (node_id, day_date, protocol),
    KEY idx_day_date (day_date),
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
