CREATE TABLE IF NOT EXISTS reseller_transactions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    reseller_username VARCHAR(64) NOT NULL,
    amount DECIMAL(12,2) NOT NULL,
    type VARCHAR(32) NOT NULL, -- 'allocation', 'deduction', 'refund'
    description VARCHAR(255) NOT NULL,
    actor VARCHAR(64) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    KEY idx_reseller (reseller_username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
