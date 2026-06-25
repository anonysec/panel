-- outbound_tunnels: Panel-tracked tunnel references
CREATE TABLE IF NOT EXISTS outbound_tunnels (
    id         BIGSERIAL PRIMARY KEY,
    node_id    BIGINT NOT NULL REFERENCES knode_connections(id),
    tunnel_id  VARCHAR(100) NOT NULL,
    protocol   VARCHAR(50) NOT NULL,
    exit_addr  VARCHAR(255) NOT NULL,
    exit_port  INT NOT NULL,
    state      VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
