-- Migration 041: Server maintenance mode

INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('maintenance_mode', 'false');
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('maintenance_message', 'System is under maintenance. Please try again later.');
INSERT IGNORE INTO panel_settings (setting_key, setting_value) VALUES ('maintenance_ends_at', '');
