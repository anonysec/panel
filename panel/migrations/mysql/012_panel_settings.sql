-- Panel-wide settings (theme, branding, etc.)
-- Stored as key-value pairs for flexibility.
CREATE TABLE IF NOT EXISTS panel_settings (
    setting_key VARCHAR(64) PRIMARY KEY,
    setting_value TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Default settings
INSERT IGNORE INTO panel_settings(setting_key, setting_value) VALUES
('panel_name', 'KorisPanel'),
('panel_description', 'VPN Management Panel'),
('theme', 'dark'),
('default_theme', 'dark'),
('allow_user_theme', 'true'),
('ssh_enabled', 'true'),
('ssh_default_port', '22'),
('telegram_enabled', 'false'),
('telegram_bot_token', ''),
('telegram_chat_id', '');
