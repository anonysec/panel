-- Migration 032: Per-user preferred node selection
-- Users can pick their preferred VPN node in the portal.
-- Config generation puts preferred node first, others as fallback.

ALTER TABLE customers ADD COLUMN IF NOT EXISTS preferred_node_id INT NULL DEFAULT NULL AFTER plan_id;
