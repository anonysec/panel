-- Migration 047: Theme presets and branding settings
-- Adds theme_presets table for storing custom and built-in themes

CREATE TABLE IF NOT EXISTS theme_presets (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    mode ENUM('light','dark') NOT NULL DEFAULT 'light',
    config_json JSON NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_by VARCHAR(64),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Insert default theme presets
INSERT INTO theme_presets (id, name, mode, config_json, is_default) VALUES
('default-light', 'Default Light', 'light', JSON_OBJECT(
    'colors', JSON_OBJECT(
        'primary', '#3b82f6',
        'primaryHover', '#2563eb',
        'secondary', '#64748b',
        'background', '#ffffff',
        'surface', '#f8fafc',
        'surfaceHover', '#f1f5f9',
        'text', '#1e293b',
        'textMuted', '#64748b',
        'border', '#e2e8f0',
        'success', '#22c55e',
        'warning', '#f59e0b',
        'error', '#ef4444',
        'info', '#3b82f6',
        'accent', '#8b5cf6'
    ),
    'borderRadius', '8px',
    'shadows', JSON_OBJECT(
        'sm', '0 1px 2px rgba(0,0,0,0.05)',
        'md', '0 4px 6px rgba(0,0,0,0.07)',
        'lg', '0 10px 15px rgba(0,0,0,0.1)'
    )
), TRUE),
('default-dark', 'Default Dark', 'dark', JSON_OBJECT(
    'colors', JSON_OBJECT(
        'primary', '#60a5fa',
        'primaryHover', '#93c5fd',
        'secondary', '#94a3b8',
        'background', '#0f172a',
        'surface', '#1e293b',
        'surfaceHover', '#334155',
        'text', '#f1f5f9',
        'textMuted', '#94a3b8',
        'border', '#334155',
        'success', '#4ade80',
        'warning', '#fbbf24',
        'error', '#f87171',
        'info', '#60a5fa',
        'accent', '#a78bfa'
    ),
    'borderRadius', '8px',
    'shadows', JSON_OBJECT(
        'sm', '0 1px 2px rgba(0,0,0,0.3)',
        'md', '0 4px 6px rgba(0,0,0,0.4)',
        'lg', '0 10px 15px rgba(0,0,0,0.5)'
    )
), FALSE),
('ocean', 'Ocean', 'dark', JSON_OBJECT(
    'colors', JSON_OBJECT(
        'primary', '#06b6d4',
        'primaryHover', '#22d3ee',
        'secondary', '#7dd3fc',
        'background', '#0c1222',
        'surface', '#162032',
        'surfaceHover', '#1e3048',
        'text', '#e0f2fe',
        'textMuted', '#7dd3fc',
        'border', '#1e3a5f',
        'success', '#34d399',
        'warning', '#fbbf24',
        'error', '#fb7185',
        'info', '#06b6d4',
        'accent', '#a78bfa'
    ),
    'borderRadius', '10px',
    'shadows', JSON_OBJECT(
        'sm', '0 1px 3px rgba(6,182,212,0.1)',
        'md', '0 4px 8px rgba(6,182,212,0.15)',
        'lg', '0 10px 20px rgba(6,182,212,0.2)'
    )
), FALSE),
('forest', 'Forest', 'dark', JSON_OBJECT(
    'colors', JSON_OBJECT(
        'primary', '#22c55e',
        'primaryHover', '#4ade80',
        'secondary', '#86efac',
        'background', '#0a1a0f',
        'surface', '#142b1a',
        'surfaceHover', '#1e3d26',
        'text', '#ecfdf5',
        'textMuted', '#86efac',
        'border', '#1e4d2b',
        'success', '#22c55e',
        'warning', '#eab308',
        'error', '#f87171',
        'info', '#38bdf8',
        'accent', '#c084fc'
    ),
    'borderRadius', '6px',
    'shadows', JSON_OBJECT(
        'sm', '0 1px 3px rgba(34,197,94,0.1)',
        'md', '0 4px 8px rgba(34,197,94,0.15)',
        'lg', '0 10px 20px rgba(34,197,94,0.2)'
    )
), FALSE),
('sunset', 'Sunset', 'light', JSON_OBJECT(
    'colors', JSON_OBJECT(
        'primary', '#f97316',
        'primaryHover', '#fb923c',
        'secondary', '#f59e0b',
        'background', '#fffbeb',
        'surface', '#fef3c7',
        'surfaceHover', '#fde68a',
        'text', '#451a03',
        'textMuted', '#92400e',
        'border', '#fcd34d',
        'success', '#16a34a',
        'warning', '#f97316',
        'error', '#dc2626',
        'info', '#0891b2',
        'accent', '#7c3aed'
    ),
    'borderRadius', '12px',
    'shadows', JSON_OBJECT(
        'sm', '0 1px 3px rgba(249,115,22,0.1)',
        'md', '0 4px 8px rgba(249,115,22,0.12)',
        'lg', '0 10px 20px rgba(249,115,22,0.15)'
    )
), FALSE),
('monochrome', 'Monochrome', 'light', JSON_OBJECT(
    'colors', JSON_OBJECT(
        'primary', '#374151',
        'primaryHover', '#1f2937',
        'secondary', '#6b7280',
        'background', '#ffffff',
        'surface', '#f9fafb',
        'surfaceHover', '#f3f4f6',
        'text', '#111827',
        'textMuted', '#6b7280',
        'border', '#d1d5db',
        'success', '#059669',
        'warning', '#d97706',
        'error', '#dc2626',
        'info', '#374151',
        'accent', '#374151'
    ),
    'borderRadius', '4px',
    'shadows', JSON_OBJECT(
        'sm', '0 1px 2px rgba(0,0,0,0.05)',
        'md', '0 2px 4px rgba(0,0,0,0.06)',
        'lg', '0 4px 8px rgba(0,0,0,0.08)'
    )
), FALSE)
ON DUPLICATE KEY UPDATE updated_at=NOW();

-- Branding settings stored in existing panel_settings table
-- Keys: 'theme_active_id', 'branding_logo_url', 'branding_app_name', 'branding_primary_color'
