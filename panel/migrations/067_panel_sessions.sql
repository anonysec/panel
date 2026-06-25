-- panel_sessions: Database-backed HTTP sessions for multi-worker support
CREATE TABLE IF NOT EXISTS panel_sessions (
    token       VARCHAR(64) PRIMARY KEY,
    admin_id    BIGINT,
    customer_id BIGINT,
    data        JSONB,
    ip_address  VARCHAR(45),
    user_agent  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ NOT NULL,
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_expires ON panel_sessions(expires_at);
