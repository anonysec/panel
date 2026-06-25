-- sync_failures: Failed user sync records for manual review
CREATE TABLE IF NOT EXISTS sync_failures (
    id         BIGSERIAL PRIMARY KEY,
    node_id    BIGINT NOT NULL,
    core_type  VARCHAR(50) NOT NULL,
    error_msg  TEXT NOT NULL,
    payload    JSONB,
    attempts   INT NOT NULL DEFAULT 2,
    resolved   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
