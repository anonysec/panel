-- Enable TimescaleDB extension and create hypertables
CREATE EXTENSION IF NOT EXISTS timescaledb;

SELECT create_hypertable('node_metrics_history', 'time', if_not_exists => TRUE);
SELECT create_hypertable('user_traffic_log', 'time', if_not_exists => TRUE);

-- Retention policies
SELECT add_retention_policy('node_metrics_history', INTERVAL '30 days', if_not_exists => TRUE);
SELECT add_retention_policy('user_traffic_log', INTERVAL '90 days', if_not_exists => TRUE);
