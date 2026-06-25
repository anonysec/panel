-- node_status: Add gRPC-sourced fields for knode integration
ALTER TABLE node_status ADD COLUMN IF NOT EXISTS grpc_connected BOOLEAN DEFAULT FALSE;
ALTER TABLE node_status ADD COLUMN IF NOT EXISTS knode_version VARCHAR(50);
ALTER TABLE node_status ADD COLUMN IF NOT EXISTS last_metrics_at TIMESTAMPTZ;
ALTER TABLE node_status ADD COLUMN IF NOT EXISTS metrics_state VARCHAR(20) DEFAULT 'unknown';
-- metrics_state values: 'streaming', 'stale', 'offline', 'unknown'
