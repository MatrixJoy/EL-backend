CREATE TABLE retell_attempts (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    content_id uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    object_key text NOT NULL UNIQUE,
    duration_milliseconds integer NOT NULL CHECK (duration_milliseconds >= 0),
    byte_size bigint NOT NULL CHECK (byte_size > 0),
    content_type text NOT NULL,
    created_at timestamptz NOT NULL,
    server_updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, id)
);

CREATE INDEX retell_attempts_user_content_idx
    ON retell_attempts (user_id, content_id, created_at DESC);
