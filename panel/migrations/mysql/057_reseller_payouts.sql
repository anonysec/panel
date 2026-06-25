-- 057_reseller_payouts.sql
CREATE TABLE IF NOT EXISTS reseller_payouts (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    reseller_username VARCHAR(64) NOT NULL,
    amount DECIMAL(12,2) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, approved, rejected
    payment_details TEXT,
    admin_note TEXT,
    requested_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at DATETIME DEFAULT NULL,
    processed_by VARCHAR(64) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

ALTER TABLE admins ADD COLUMN payout_balance DECIMAL(12,2) NOT NULL DEFAULT 0;
ALTER TABLE admins ADD COLUMN commission_percent DECIMAL(5,2) NOT NULL DEFAULT 0;
ALTER TABLE admins ADD COLUMN min_payout_amount DECIMAL(12,2) NOT NULL DEFAULT 0;
