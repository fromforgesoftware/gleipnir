CREATE TABLE gleipnir.credential (
    id            UUID NOT NULL DEFAULT uuid_generate_v4() PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    connection_id UUID NOT NULL REFERENCES gleipnir.connection (id) ON DELETE CASCADE,
    kind          TEXT NOT NULL,
    ciphertext    BYTEA NOT NULL,
    wrapped_key   BYTEA NOT NULL,
    key_id        TEXT NOT NULL,
    expires_at    TIMESTAMPTZ
);

CREATE INDEX idx_credential_connection ON gleipnir.credential (connection_id);
CREATE INDEX idx_credential_expires ON gleipnir.credential (expires_at);
