-- Migration 028: Seed default VPN configs for existing nodes that are missing them
-- Ensures all protocols show in the Cores tab for every node

INSERT IGNORE INTO node_vpn_configs (node_id, protocol, enabled, port, network, extra_json)
SELECT n.id, 'openvpn', 1, 1194, '10.8.0.0/20', '{"transport":"udp","cipher":"AES-256-GCM","tls_mode":"tls-crypt","dns1":"8.8.8.8","dns2":"8.8.4.4","comp_lzo":false,"topology":"subnet","verb":3,"keepalive":"10 120"}'
FROM nodes n LEFT JOIN node_vpn_configs c ON c.node_id = n.id AND c.protocol = 'openvpn'
WHERE c.id IS NULL;

INSERT IGNORE INTO node_vpn_configs (node_id, protocol, enabled, port, network, extra_json)
SELECT n.id, 'l2tp', 0, 1701, '10.9.0.0/20', '{"ipsec_mode":"ipsec","psk":"","auth_method":"CHAP","dns1":"8.8.8.8","dns2":"8.8.4.4","lcp_echo_interval":30,"lcp_echo_failure":4}'
FROM nodes n LEFT JOIN node_vpn_configs c ON c.node_id = n.id AND c.protocol = 'l2tp'
WHERE c.id IS NULL;

INSERT IGNORE INTO node_vpn_configs (node_id, protocol, enabled, port, network, extra_json)
SELECT n.id, 'ikev2', 0, 500, '10.10.0.0/20', '{"auth_type":"psk","psk":"","dns1":"8.8.8.8","dns2":"8.8.4.4","dpd_interval":30,"dpd_timeout":150,"rekey_time":"4h","ike_proposals":"aes256-sha256-modp2048","esp_proposals":"aes256-sha256"}'
FROM nodes n LEFT JOIN node_vpn_configs c ON c.node_id = n.id AND c.protocol = 'ikev2'
WHERE c.id IS NULL;

INSERT IGNORE INTO node_vpn_configs (node_id, protocol, enabled, port, network, extra_json)
SELECT n.id, 'ssh', 0, 2222, '', '{"listen_address":"0.0.0.0","key_type":"ed25519","max_sessions":10,"idle_timeout":0,"shell_access":false,"accounting_enabled":true,"accounting_interval":300}'
FROM nodes n LEFT JOIN node_vpn_configs c ON c.node_id = n.id AND c.protocol = 'ssh'
WHERE c.id IS NULL;

INSERT IGNORE INTO node_vpn_configs (node_id, protocol, enabled, port, network, extra_json)
SELECT n.id, 'wireguard', 0, 51820, '10.66.66.0/20', '{"dns_1":"1.1.1.1","dns_2":"8.8.8.8","gaming_optimize":false}'
FROM nodes n LEFT JOIN node_vpn_configs c ON c.node_id = n.id AND c.protocol = 'wireguard'
WHERE c.id IS NULL;
