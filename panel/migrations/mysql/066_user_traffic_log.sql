-- user_traffic_log: Per-user bandwidth accounting
CREATE TABLE IF NOT EXISTS user_traffic_log (
    time        TIMESTAMPTZ NOT NULL,
    user_id     BIGINT NOT NULL,
    node_id     BIGINT NOT NULL,
    rx_bytes    BIGINT NOT NULL DEFAULT 0,
    tx_bytes    BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_user_traffic_log_user_time
    ON user_traffic_log(user_id, time DESC);

CREATE INDEX IF NOT EXISTS idx_user_traffic_log_node_time
    ON user_traffic_log(node_id, time DESC);

-- Convert to hypertable with 90-day retention if TimescaleDB is available
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
    PERFORM create_hypertable('user_traffic_log', 'time', if_not_exists => TRUE);
    PERFORM add_retention_policy('user_traffic_log', INTERVAL '90 days', if_not_exists => TRUE);
  END IF;
END $$;
