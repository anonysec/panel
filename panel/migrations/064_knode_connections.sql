-- knode_connections: Node connection credentials for gRPC client pool
CREATE TABLE IF NOT EXISTS knode_connections (
    id            BIGSERIAL PRIMARY KEY,
    name          VARCHAR(255) NOT NULL UNIQUE,
    address       VARCHAR(255) NOT NULL,
    grpc_port     INT NOT NULL DEFAULT 62050,
    api_key_enc   BYTEA NOT NULL,
    client_cert   TEXT NOT NULL,
    client_key_enc BYTEA NOT NULL,
    ca_cert       TEXT NOT NULL,
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    status        VARCHAR(20) NOT NULL DEFAULT 'offline',
    last_seen_at  TIMESTAMPTZ,
    owner_worker  VARCHAR(100),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
