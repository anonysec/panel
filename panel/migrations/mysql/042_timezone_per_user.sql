-- Migration 042: Per-user timezone support

ALTER TABLE customers ADD COLUMN IF NOT EXISTS timezone VARCHAR(50) NULL DEFAULT NULL AFTER auto_renew;
