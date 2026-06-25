-- Payment intents describe what approval should do.
-- wallet_topup: approval credits wallet only.
-- plan_renewal: approval credits wallet and then activates intent_id plan if wallet is sufficient.
ALTER TABLE payments ADD COLUMN IF NOT EXISTS intent_type VARCHAR(40) NOT NULL DEFAULT 'wallet_topup' AFTER status;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS intent_id BIGINT NULL AFTER intent_type;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS metadata_json JSON NULL AFTER intent_id;
UPDATE payments SET intent_type='wallet_topup' WHERE intent_type IS NULL OR intent_type='';
CREATE INDEX IF NOT EXISTS payments_intent_idx ON payments(intent_type, intent_id);
