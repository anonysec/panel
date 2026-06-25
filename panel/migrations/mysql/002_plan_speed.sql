-- Add speed limit metadata for reusable plans.
-- A value of 0 means unlimited/no speed policy.
ALTER TABLE plans ADD COLUMN IF NOT EXISTS speed_mbps DECIMAL(12,2) NOT NULL DEFAULT 0 AFTER data_gb;
