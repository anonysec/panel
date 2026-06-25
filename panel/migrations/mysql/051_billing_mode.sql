-- Billing mode: reseller default + per-user override
ALTER TABLE admins ADD COLUMN IF NOT EXISTS billing_mode VARCHAR(20) NOT NULL DEFAULT 'manual';
ALTER TABLE customers ADD COLUMN IF NOT EXISTS billing_mode VARCHAR(20) NULL;
