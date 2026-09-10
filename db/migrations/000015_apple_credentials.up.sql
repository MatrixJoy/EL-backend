-- Detached credentials form a durable revocation queue after account deletion.
-- Only AES-GCM ciphertext is retained; it is removed after Apple confirms revocation.
CREATE TABLE apple_credentials (
    id uuid PRIMARY KEY,
    user_id uuid UNIQUE REFERENCES users(id) ON DELETE SET NULL,
    encrypted_refresh_token bytea NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX apple_revocation_pending ON apple_credentials(next_attempt_at) WHERE user_id IS NULL;
