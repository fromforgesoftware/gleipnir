CREATE TABLE gleipnir.oauth_state (
    id            UUID NOT NULL DEFAULT uuid_generate_v4() PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- state is the CSRF token echoed by the provider on callback; unique so a
    -- collision cannot let one transaction's callback satisfy another.
    state         TEXT NOT NULL UNIQUE,
    connection_id UUID NOT NULL REFERENCES gleipnir.connection (id) ON DELETE CASCADE,
    connector     TEXT NOT NULL,
    redirect_uri  TEXT NOT NULL,
    -- code_verifier is the PKCE secret held server-side; empty for non-PKCE
    -- flows. It is never returned over the API.
    code_verifier TEXT NOT NULL DEFAULT '',
    expires_at    TIMESTAMPTZ NOT NULL,
    -- consumed_at marks the state as spent; the callback consumes a state
    -- atomically so it can be used at most once (replay protection).
    consumed_at   TIMESTAMPTZ
);

CREATE INDEX idx_oauth_state_connection ON gleipnir.oauth_state (connection_id);
CREATE INDEX idx_oauth_state_expires ON gleipnir.oauth_state (expires_at);
