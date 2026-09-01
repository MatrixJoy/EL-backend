CREATE TABLE apple_identity_token_uses (
    token_hash bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    used_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX apple_identity_token_uses_user_idx ON apple_identity_token_uses (user_id, used_at);
