-- Fix: add cert_path and status columns that migration 024 failed to create (was no-op SELECT 1)
ALTER TABLE vpn_certificates ADD COLUMN cert_path VARCHAR(512) NULL AFTER fingerprint;
ALTER TABLE vpn_certificates ADD COLUMN status ENUM('active','revoked','expired') NULL DEFAULT 'active' AFTER cert_path;
ALTER TABLE vpn_certificates ADD INDEX idx_cert_status (status);
