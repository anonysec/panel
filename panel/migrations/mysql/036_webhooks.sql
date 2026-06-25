-- Migration 036: Webhook system for external integrations

CREATE TABLE IF NOT EXISTS webhooks (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  url VARCHAR(512) NOT NULL,
  secret VARCHAR(128) NULL,
  events TEXT NOT NULL DEFAULT 'all',
  is_active TINYINT(1) NOT NULL DEFAULT 1,
  last_triggered_at TIMESTAMP NULL,
  last_status INT NULL,
  fail_count INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_webhook_active (is_active)
);

-- Webhook event log (last 100 per webhook)
CREATE TABLE IF NOT EXISTS webhook_logs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  webhook_id BIGINT NOT NULL,
  event_type VARCHAR(50) NOT NULL,
  payload_json TEXT NOT NULL,
  response_status INT NULL,
  response_body TEXT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_webhook_log_wh (webhook_id, created_at DESC)
);
