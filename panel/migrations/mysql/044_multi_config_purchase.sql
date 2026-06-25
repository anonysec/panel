-- Migration 044: Multi-config purchase (buy extra connection slots)

-- Per-plan default connections and price per extra
ALTER TABLE plans ADD COLUMN IF NOT EXISTS default_connections INT NOT NULL DEFAULT 1 AFTER currency;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS max_connections INT NOT NULL DEFAULT 3 AFTER default_connections;
ALTER TABLE plans ADD COLUMN IF NOT EXISTS price_per_extra_conn DECIMAL(12,2) NOT NULL DEFAULT 0 AFTER max_connections;

-- Track purchased extra connections per customer
ALTER TABLE customers ADD COLUMN IF NOT EXISTS extra_connections INT NOT NULL DEFAULT 0 AFTER conn_limit_override;
