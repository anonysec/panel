-- Add proxy_config JSON column to nodes table for per-node API proxy settings.
-- Stores HTTP/SOCKS5 proxy config as a JSON blob (nullable).
ALTER TABLE nodes ADD COLUMN proxy_config TEXT DEFAULT NULL;
