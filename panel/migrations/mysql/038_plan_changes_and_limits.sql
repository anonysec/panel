-- Migration 038: Plan upgrade/downgrade tracking + per-user connection limits

-- Plan change history
CREATE TABLE IF NOT EXISTS plan_changes (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  customer_id BIGINT NOT NULL,
  username VARCHAR(64) NOT NULL,
  old_plan_id BIGINT NULL,
  new_plan_id BIGINT NULL,
  change_type ENUM('upgrade','downgrade','cancel','initial') NOT NULL,
  prorated_credit DECIMAL(12,2) NOT NULL DEFAULT 0,
  actor VARCHAR(64) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_plan_change_customer (customer_id),
  INDEX idx_plan_change_date (created_at DESC)
);

-- Per-user connection limit override (admin can set custom limit)
ALTER TABLE customers ADD COLUMN IF NOT EXISTS conn_limit_override INT NULL AFTER trial_used;
