CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    apple_subject text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id, expires_at);

CREATE TABLE bookmarks (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_id uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, content_id)
);

CREATE TABLE learning_progress (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_id uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    position_seconds integer NOT NULL DEFAULT 0 CHECK (position_seconds >= 0),
    completed boolean NOT NULL DEFAULT false,
    client_updated_at timestamptz NOT NULL,
    server_updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, content_id)
);

CREATE TABLE user_events (
    sequence bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    operation text NOT NULL CHECK (operation IN ('upsert', 'delete')),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX user_events_sync_idx ON user_events (user_id, sequence);
