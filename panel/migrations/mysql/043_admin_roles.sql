-- Migration 043: Admin roles and permissions (RBAC)

-- Expand admins table with granular permissions
ALTER TABLE admins ADD COLUMN IF NOT EXISTS permissions TEXT NULL AFTER role;

-- Available roles: owner > admin > support
-- owner: full access
-- admin: everything except manage other admins
-- support: view customers, manage tickets, view nodes (no billing/settings)

-- Session tracking for admins
CREATE TABLE IF NOT EXISTS admin_sessions (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(64) NOT NULL,
  session_hash VARCHAR(128) NOT NULL,
  ip_address VARCHAR(45) NOT NULL,
  user_agent TEXT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  last_active_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  expires_at TIMESTAMP NOT NULL,
  INDEX idx_admin_session_user (username),
  INDEX idx_admin_session_hash (session_hash),
  INDEX idx_admin_session_expires (expires_at)
);
