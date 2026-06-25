-- First-connection activation: subscription timer starts on first VPN connect, not creation time.
-- When first_connect_at is NULL and expires_at is NULL, the subscription is "pending activation".
-- On first VPN connect, the auth script sets first_connect_at and calculates expires_at from the plan duration.
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS first_connect_at DATETIME NULL AFTER started_at;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS activate_on_connect TINYINT(1) NOT NULL DEFAULT 0 AFTER first_connect_at;
