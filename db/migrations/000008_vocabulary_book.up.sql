CREATE TABLE vocabulary_entries (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    word text NOT NULL,
    normalized_word text NOT NULL,
    definition text,
    sentence_context text,
    content_id uuid REFERENCES contents(id) ON DELETE SET NULL,
    content_title text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, normalized_word)
);

CREATE INDEX vocabulary_entries_user_updated_idx
    ON vocabulary_entries (user_id, updated_at DESC);
