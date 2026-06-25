-- 2FA TOTP for admin users
ALTER TABLE admins ADD COLUMN totp_secret VARCHAR(64) DEFAULT '' AFTER password_hash;
ALTER TABLE admins ADD COLUMN totp_enabled TINYINT(1) DEFAULT 0 AFTER totp_secret;

-- Payment gateway tracking
ALTER TABLE payments ADD COLUMN gateway_authority VARCHAR(128) DEFAULT '' AFTER status;
ALTER TABLE payments ADD COLUMN gateway_ref_id VARCHAR(128) DEFAULT '' AFTER gateway_authority;
ALTER TABLE payments ADD COLUMN gateway_name VARCHAR(32) DEFAULT '' AFTER gateway_ref_id;

-- Connection limit per customer (stored in extra_json but adding explicit column for queries)
ALTER TABLE customers ADD COLUMN conn_limit INT DEFAULT 0 AFTER status;

-- Email field for customers (for notifications)
ALTER TABLE customers ADD COLUMN email VARCHAR(255) DEFAULT '' AFTER display_name;
