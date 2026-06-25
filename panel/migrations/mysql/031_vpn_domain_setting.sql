-- Migration 031: Global VPN domain for static config files
-- When set, all VPN configs use this domain instead of per-node IPs.
-- This enables DNS-based failover: point the domain to any active node.

INSERT IGNORE INTO panel_settings (setting_key, setting_value)
VALUES ('vpn_domain', '');
