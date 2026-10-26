-- +goose Up
CREATE TABLE IF NOT EXISTS sessions (
    token_hash BYTEA PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id);
CREATE INDEX IF NOT EXISTS sessions_absolute_expires_at_idx ON sessions(absolute_expires_at);
CREATE INDEX IF NOT EXISTS sessions_revoked_at_idx ON sessions(revoked_at) WHERE revoked_at IS NOT NULL;

-- +goose Down
DROP TABLE sessions;
