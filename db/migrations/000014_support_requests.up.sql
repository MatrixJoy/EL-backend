CREATE TABLE support_requests (
    id uuid PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('problem', 'privacy', 'content')),
    email text NOT NULL DEFAULT '',
    message text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz
);
CREATE INDEX support_requests_open ON support_requests (created_at) WHERE resolved_at IS NULL;
