-- Migration 060: Xray config templates.
-- Admin-editable JSON templates for transport/security/fallback settings.

CREATE TABLE IF NOT EXISTS xray_templates (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    config_json JSON NOT NULL,
    category VARCHAR(50) DEFAULT 'general',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

-- Seed built-in default templates.
INSERT INTO xray_templates (name, description, config_json, category) VALUES
(
    'VLESS + Reality',
    'Basic VLESS+Reality single inbound on port 443 with TCP transport',
    '{"inbounds":[{"protocol":"vless","port":443,"transport":"tcp","tag":"vless-reality","settings":{"decryption":"none","clients":[]}}],"routing":{"domain_strategy":"AsIs","rules":[]},"tls":{},"reality":{"server_names":["www.google.com"],"short_ids":["abcdef12"]}}',
    'protocol'
),
(
    'Multi-Protocol (Fallback)',
    'VLESS on 443 with VMess-WS and Trojan-WS fallbacks for maximum compatibility',
    '{"inbounds":[{"protocol":"vless","port":443,"transport":"tcp","tag":"vless-main","settings":{"decryption":"none","clients":[],"fallbacks":[{"dest":8080,"xver":1},{"path":"/vmess-ws","dest":8081,"xver":1},{"path":"/trojan-ws","dest":8082,"xver":1}]}},{"protocol":"vmess","port":8081,"transport":"ws","tag":"vmess-ws","settings":{"clients":[]}},{"protocol":"trojan","port":8082,"transport":"ws","tag":"trojan-ws","settings":{"clients":[]}}],"routing":{"domain_strategy":"AsIs","rules":[]},"tls":{"server_name":"","alpn":["h2","http/1.1"]}}',
    'protocol'
),
(
    'VMess WebSocket',
    'Simple VMess-WS on port 8080 for CDN-friendly deployments',
    '{"inbounds":[{"protocol":"vmess","port":8080,"transport":"ws","tag":"vmess-ws","settings":{"clients":[]}}],"routing":{"domain_strategy":"AsIs","rules":[]},"tls":{}}',
    'protocol'
);
