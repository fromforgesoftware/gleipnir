CREATE TABLE gleipnir.connection (
    id         UUID NOT NULL DEFAULT uuid_generate_v4() PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    owner      TEXT NOT NULL,
    connector  TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'ACTIVE',
    scopes     JSONB NOT NULL DEFAULT '[]',
    expires_at TIMESTAMPTZ
);

CREATE INDEX idx_connection_owner ON gleipnir.connection (owner);
CREATE INDEX idx_connection_status ON gleipnir.connection (status);
