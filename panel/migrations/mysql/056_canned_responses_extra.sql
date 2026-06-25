-- Migration 056: Add created_by and updated_at to canned_responses

ALTER TABLE canned_responses
  ADD COLUMN created_by VARCHAR(64) NULL AFTER category,
  ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP AFTER created_at;
