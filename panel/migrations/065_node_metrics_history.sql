-- node_metrics_history: Time-series metrics from knode gRPC streams
CREATE TABLE IF NOT EXISTS node_metrics_history (
    time            TIMESTAMPTZ NOT NULL,
    node_id         BIGINT NOT NULL REFERENCES knode_connections(id),
    cpu_percent     DOUBLE PRECISION,
    ram_percent     DOUBLE PRECISION,
    disk_percent    DOUBLE PRECISION,
    rx_bps          BIGINT,
    tx_bps          BIGINT,
    active_sessions INT,
    uptime_seconds  BIGINT
);

CREATE INDEX IF NOT EXISTS idx_node_metrics_history_node_time
    ON node_metrics_history(node_id, time DESC);

-- Convert to hypertable with 30-day retention if TimescaleDB is available
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
    PERFORM create_hypertable('node_metrics_history', 'time', if_not_exists => TRUE);
    PERFORM add_retention_policy('node_metrics_history', INTERVAL '30 days', if_not_exists => TRUE);
  END IF;
END $$;
