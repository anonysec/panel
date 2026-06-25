-- Explicit references make wallet transactions idempotent and auditable.
ALTER TABLE wallet_transactions ADD COLUMN IF NOT EXISTS reference_type VARCHAR(40) NOT NULL DEFAULT '' AFTER actor;
ALTER TABLE wallet_transactions ADD COLUMN IF NOT EXISTS reference_id BIGINT NULL AFTER reference_type;
CREATE INDEX IF NOT EXISTS wallet_transactions_reference_idx ON wallet_transactions(reference_type, reference_id);
