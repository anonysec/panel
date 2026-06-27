-- Add domain column to knode_connections for IKEv2 certificate management.
-- The domain is used to obtain Let's Encrypt certificates and as the IKEv2 server identity.
ALTER TABLE knode_connections ADD COLUMN domain VARCHAR(255) DEFAULT NULL;
