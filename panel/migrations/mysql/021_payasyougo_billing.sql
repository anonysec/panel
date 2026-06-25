-- Pay-as-you-go billing support
ALTER TABLE plans ADD COLUMN billing_type ENUM('quota','payg') NOT NULL DEFAULT 'quota' AFTER price;
ALTER TABLE plans ADD COLUMN price_per_gb DECIMAL(10,2) NOT NULL DEFAULT 0 AFTER billing_type;
ALTER TABLE plans ADD COLUMN price_per_day DECIMAL(10,2) NOT NULL DEFAULT 0 AFTER price_per_gb;
ALTER TABLE plans ADD COLUMN disconnect_on_zero TINYINT(1) NOT NULL DEFAULT 1 AFTER price_per_day;

-- Track PAYG usage deductions
CREATE TABLE IF NOT EXISTS payg_deductions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    customer_id BIGINT NOT NULL,
    username VARCHAR(64) NOT NULL,
    plan_id BIGINT NOT NULL,
    deduction_type ENUM('data','time') NOT NULL,
    amount DECIMAL(10,4) NOT NULL,
    usage_value DECIMAL(14,4) NOT NULL,
    balance_before DECIMAL(10,2) NOT NULL,
    balance_after DECIMAL(10,2) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX(customer_id),
    INDEX(username),
    INDEX(created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
