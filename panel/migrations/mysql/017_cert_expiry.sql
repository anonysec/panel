-- Certificate expiry tracking for automatic rotation
ALTER TABLE vpn_certificates ADD COLUMN expires_at DATETIME NULL AFTER is_default;
ALTER TABLE vpn_certificates ADD COLUMN fingerprint VARCHAR(128) NULL AFTER expires_at;
ALTER TABLE vpn_certificates ADD INDEX idx_expires_at (expires_at);
