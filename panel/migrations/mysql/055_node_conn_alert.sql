-- Migration 055: Add connection count alert threshold to nodes

ALTER TABLE nodes ADD COLUMN IF NOT EXISTS alert_conn_threshold INT DEFAULT 0;
