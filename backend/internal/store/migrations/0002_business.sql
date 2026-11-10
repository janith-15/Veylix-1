-- 0002_business.sql — offers, VPN instances, assignments, quota, outbox
--
-- Source: docs/REQUIREMENTS.md §5, D-012 (idempotency pair), F-009 (outbox), F-010 (lifetime totals)

CREATE TABLE vpn_offers (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    kind          TEXT    NOT NULL CHECK (kind IN ('FREE', 'PREMIUM')),
    name          TEXT    NOT NULL,
    server_id     INTEGER REFERENCES server_integrations (id) ON DELETE SET NULL,
    inbound_id    INTEGER REFERENCES inbounds (id) ON DELETE SET NULL,
    traffic_gb    INTEGER NOT NULL DEFAULT 0,    -- 0 = unlimited
    duration_days INTEGER NOT NULL DEFAULT 0,    -- 0 = no expiry
    device_limit  INTEGER NOT NULL DEFAULT 0,    -- 0 = unlimited (maps to limitIp)
    active        INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);

CREATE TABLE vpn_instances (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_user_id           INTEGER NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    server_id               INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE RESTRICT,
    inbound_id              INTEGER REFERENCES inbounds (id) ON DELETE SET NULL,
    offer_id                INTEGER REFERENCES vpn_offers (id) ON DELETE SET NULL,
    -- idempotency pair (D-012): a retry cannot create a second panel client
    panel_email             TEXT    NOT NULL,
    panel_sub_id            TEXT    NOT NULL,
    status                  TEXT    NOT NULL DEFAULT 'PENDING'
                            CHECK (status IN ('PENDING', 'ACTIVE', 'TRAFFIC_DEPLETED', 'EXPIRED',
                                              'SUSPENDED', 'SERVER_UNAVAILABLE', 'REVOCATION_PENDING',
                                              'REVOKED', 'REMOVED_FROM_CATALOG')),
    traffic_limit_bytes     INTEGER NOT NULL DEFAULT 0,
    expires_at              INTEGER,
    device_limit            INTEGER NOT NULL DEFAULT 0,
    lifetime_up_bytes       INTEGER NOT NULL DEFAULT 0,   -- survives remote resets (F-010)
    lifetime_down_bytes     INTEGER NOT NULL DEFAULT 0,
    last_remote_reset_count INTEGER NOT NULL DEFAULT 0,
    created_at              INTEGER NOT NULL,
    updated_at              INTEGER NOT NULL,
    UNIQUE (server_id, panel_email)
);
CREATE INDEX idx_vpn_owner  ON vpn_instances (owner_user_id, status);
CREATE INDEX idx_vpn_server ON vpn_instances (server_id, status);

CREATE TABLE vpn_assignments (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    vpn_instance_id INTEGER NOT NULL REFERENCES vpn_instances (id) ON DELETE RESTRICT,
    client_user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    seller_user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    status          TEXT    NOT NULL DEFAULT 'PENDING',
    price_cents     INTEGER NOT NULL DEFAULT 0,
    currency        TEXT    NOT NULL DEFAULT 'USD',
    started_at      INTEGER,
    expires_at      INTEGER,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);
CREATE INDEX idx_assign_client ON vpn_assignments (client_user_id, status);
CREATE INDEX idx_assign_seller ON vpn_assignments (seller_user_id, status);

CREATE TABLE quota_ledger (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    admin_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    delta         INTEGER NOT NULL,              -- +1 consume, -1 release
    reason        TEXT    NOT NULL,              -- ASSIGN | EXPIRE | REVOKE | ADJUST
    assignment_id INTEGER REFERENCES vpn_assignments (id) ON DELETE SET NULL,
    created_at    INTEGER NOT NULL
);
CREATE INDEX idx_quota_admin ON quota_ledger (admin_id, created_at);

CREATE TABLE outbox (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    op              TEXT    NOT NULL,            -- CREATE_CLIENT | UPDATE_CLIENT | DELETE_CLIENT
    server_id       INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE CASCADE,
    idempotency_key TEXT    NOT NULL,            -- e.g. "<server>:<panel_email>"
    payload_json    TEXT    NOT NULL DEFAULT '{}',
    state           TEXT    NOT NULL DEFAULT 'PENDING'
                    CHECK (state IN ('PENDING', 'IN_FLIGHT', 'APPLIED', 'RETRY', 'FAILED')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT    NOT NULL DEFAULT '',
    next_attempt_at INTEGER,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    UNIQUE (server_id, op, idempotency_key)
);
CREATE INDEX idx_outbox_state ON outbox (state, next_attempt_at);

-- --------------------------------------------------------------------------
-- Payments (manual confirmation, auditable — REQUIREMENTS §2.5)
-- --------------------------------------------------------------------------
CREATE TABLE payment_methods (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_user_id INTEGER REFERENCES users (id) ON DELETE CASCADE,  -- NULL = global
    kind          TEXT    NOT NULL,              -- BANK | EZCASH | MOBILE | OTHER
    label         TEXT    NOT NULL,
    details       TEXT    NOT NULL DEFAULT '',
    instructions  TEXT    NOT NULL DEFAULT '',
    active        INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    created_at    INTEGER NOT NULL
);

CREATE TABLE payment_records (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    assignment_id INTEGER REFERENCES vpn_assignments (id) ON DELETE SET NULL,
    payer_user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    method_id     INTEGER REFERENCES payment_methods (id) ON DELETE SET NULL,
    amount_cents  INTEGER NOT NULL,
    currency      TEXT    NOT NULL DEFAULT 'USD',
    reference     TEXT    NOT NULL DEFAULT '',
    status        TEXT    NOT NULL DEFAULT 'PENDING'
                  CHECK (status IN ('PENDING', 'AWAITING_CONFIRMATION', 'CONFIRMED', 'REJECTED',
                                    'CANCELLED', 'REFUND_PENDING', 'REFUNDED')),
    confirmed_by  INTEGER REFERENCES users (id) ON DELETE SET NULL,
    confirmed_at  INTEGER,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);
CREATE INDEX idx_pay_status ON payment_records (status, created_at);
