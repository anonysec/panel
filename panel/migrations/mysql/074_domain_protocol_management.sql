-- Domain & Protocol Management: independent domain registry, IP rotation audit,
-- protocol-to-domain bindings, per-user MTProto secrets, and IKEv2 certificates.

-- Independent domain registry
CREATE TABLE IF NOT EXISTS vpn_domains (
    id         BIGSERIAL PRIMARY KEY,
    name       VARCHAR(253) NOT NULL,
    ip_address VARCHAR(45) NOT NULL,
    status     VARCHAR(10) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'blocked', 'retired')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_vpn_domains_name UNIQUE (name)
);

-- IP rotation audit log (append-only)
CREATE TABLE IF NOT EXISTS vpn_domain_ip_history (
    id             BIGSERIAL PRIMARY KEY,
    domain_id      BIGINT NOT NULL REFERENCES vpn_domains(id) ON DELETE CASCADE,
    previous_ip    VARCHAR(45) NOT NULL,
    new_ip         VARCHAR(45) NOT NULL,
    admin_username VARCHAR(100) NOT NULL,
    rotated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_domain_ip_history_domain
    ON vpn_domain_ip_history(domain_id, rotated_at DESC);

-- Protocol-to-domain bindings with failover priority
CREATE TABLE IF NOT EXISTS vpn_protocol_bindings (
    id         BIGSERIAL PRIMARY KEY,
    node_id    BIGINT NOT NULL REFERENCES knode_connections(id) ON DELETE CASCADE,
    protocol   VARCHAR(20) NOT NULL
        CHECK (protocol IN ('openvpn-udp', 'openvpn-tcp', 'l2tp', 'ikev2', 'wireguard', 'ssh', 'mtproto')),
    domain_id  BIGINT NOT NULL REFERENCES vpn_domains(id) ON DELETE RESTRICT,
    position   INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_binding_node_proto_domain UNIQUE (node_id, protocol, domain_id),
    CONSTRAINT uq_binding_node_proto_position UNIQUE (node_id, protocol, position)
);

CREATE INDEX idx_protocol_bindings_node
    ON vpn_protocol_bindings(node_id, protocol, position);

-- Per-user MTProto secrets (extension to customers table)
ALTER TABLE customers ADD COLUMN IF NOT EXISTS mtproto_secret VARCHAR(64) DEFAULT NULL;
ALTER TABLE customers ADD COLUMN IF NOT EXISTS mtproto_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE customers ADD COLUMN IF NOT EXISTS mtproto_conn_limit INT NOT NULL DEFAULT 1;

-- IKEv2 certificate lifecycle
CREATE TABLE IF NOT EXISTS vpn_certificates (
    id          BIGSERIAL PRIMARY KEY,
    node_id     BIGINT NOT NULL REFERENCES knode_connections(id) ON DELETE CASCADE,
    domain_id   BIGINT REFERENCES vpn_domains(id) ON DELETE SET NULL,
    cert_type   VARCHAR(20) NOT NULL DEFAULT 'ikev2',
    status      VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'active', 'expired')),
    certificate TEXT,
    private_key TEXT,
    ca_chain    TEXT,
    issued_at   TIMESTAMPTZ,
    expires_at  TIMESTAMPTZ,
    retry_count INT NOT NULL DEFAULT 0,
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vpn_certificates_expiry
    ON vpn_certificates(expires_at) WHERE status = 'active';

CREATE INDEX idx_vpn_certificates_domain
    ON vpn_certificates(domain_id) WHERE domain_id IS NOT NULL;
