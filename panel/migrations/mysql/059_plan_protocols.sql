-- Migration 059: Add plan_protocols column to plans table.
-- Stores a JSON array of allowed protocol names for this plan.
-- NULL means all protocols are allowed (no restriction).
-- Example: '["openvpn","wireguard","ikev2"]'
ALTER TABLE plans ADD COLUMN IF NOT EXISTS plan_protocols JSON DEFAULT NULL AFTER features;
