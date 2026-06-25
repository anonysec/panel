-- Migration 053: User management enhancements — custom fields, notes, segments

-- Admin-defined metadata fields for customers
CREATE TABLE IF NOT EXISTS custom_fields (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  field_name VARCHAR(100) NOT NULL UNIQUE,
  field_type ENUM('text','number','boolean','date','select') NOT NULL DEFAULT 'text',
  field_options TEXT NULL,
  required BOOLEAN DEFAULT FALSE,
  display_order INT DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Per-customer values for custom fields
CREATE TABLE IF NOT EXISTS customer_custom_values (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  customer_id BIGINT NOT NULL,
  field_id BIGINT NOT NULL,
  field_value TEXT,
  UNIQUE KEY idx_customer_field (customer_id, field_id),
  FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE,
  FOREIGN KEY (field_id) REFERENCES custom_fields(id) ON DELETE CASCADE
);

-- Private admin notes on customers
CREATE TABLE IF NOT EXISTS user_notes (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  customer_id BIGINT NOT NULL,
  admin_username VARCHAR(64) NOT NULL,
  body TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_customer (customer_id),
  FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE
);

-- Rule-based customer segments
CREATE TABLE IF NOT EXISTS user_segments (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  description TEXT,
  rules_json JSON NOT NULL,
  customer_count INT DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
