-- Migration 029: Performance indexes for high-user-count deployments
-- These composite indexes optimize the most frequent queries at scale

-- Active sessions lookup (dashboard, online users, session enforcement)
CREATE INDEX IF NOT EXISTS idx_radacct_active ON radacct(username, acctstoptime);
CREATE INDEX IF NOT EXISTS idx_radacct_active_nas ON radacct(nasipaddress, acctstoptime);

-- Today's usage stats (dashboard)
CREATE INDEX IF NOT EXISTS idx_radacct_starttime ON radacct(acctstarttime, acctinputoctets, acctoutputoctets);

-- Customer lookups (list, search, detail)
CREATE INDEX IF NOT EXISTS idx_customers_status ON customers(status, deleted_at);
CREATE INDEX IF NOT EXISTS idx_customers_plan ON customers(plan_id, deleted_at);

-- Wallet transactions (customer detail, billing)
CREATE INDEX IF NOT EXISTS idx_wallet_tx_user ON wallet_transactions(username, id DESC);

-- Payments (pending count, list)
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status, created_at DESC);

-- Node tasks (polling — most frequent query from agents)
CREATE INDEX IF NOT EXISTS idx_node_tasks_poll ON node_tasks(node_id, status, id);

-- WireGuard peers per customer
CREATE INDEX IF NOT EXISTS idx_wg_peers_customer ON wg_peers(customer_id, status);

-- Events (unseen count on dashboard)
CREATE INDEX IF NOT EXISTS idx_events_seen ON events(seen);

-- Subscriptions (expiry checks)
CREATE INDEX IF NOT EXISTS idx_subs_expires ON subscriptions(username, expires_at);
