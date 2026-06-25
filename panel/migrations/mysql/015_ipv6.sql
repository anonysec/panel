-- IPv6 dual-stack support
ALTER TABLE node_vpn_configs ADD COLUMN network_ipv6 VARCHAR(64) NULL AFTER network;
