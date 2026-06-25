-- Migration 050: Billing enhancements
-- Payment gateways, invoices, data packs, wallet_transactions invoice link

CREATE TABLE IF NOT EXISTS payment_gateways (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(50) NOT NULL,
  display_name VARCHAR(100) NOT NULL,
  type ENUM('manual','zarinpal','crypto','stripe') NOT NULL,
  config_json JSON NOT NULL,
  is_active BOOLEAN DEFAULT TRUE,
  sort_order INT DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS invoices (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  customer_id BIGINT NOT NULL,
  invoice_number VARCHAR(20) NOT NULL UNIQUE,
  amount DECIMAL(12,2) NOT NULL,
  currency VARCHAR(10) DEFAULT 'IRR',
  status ENUM('draft','paid','cancelled','refunded') DEFAULT 'draft',
  type ENUM('subscription','topup','data_pack','refund') NOT NULL,
  description TEXT NULL,
  plan_id BIGINT NULL,
  gateway_id BIGINT NULL,
  payment_ref VARCHAR(100) NULL,
  pdf_path VARCHAR(255) NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  paid_at TIMESTAMP NULL,
  FOREIGN KEY (customer_id) REFERENCES customers(id),
  INDEX idx_customer (customer_id),
  INDEX idx_status (status)
);

CREATE TABLE IF NOT EXISTS data_packs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  data_gb INT NOT NULL,
  price DECIMAL(12,2) NOT NULL,
  currency VARCHAR(10) DEFAULT 'IRR',
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE wallet_transactions
  ADD COLUMN IF NOT EXISTS invoice_id BIGINT NULL AFTER customer_id,
  ADD INDEX idx_invoice (invoice_id);
