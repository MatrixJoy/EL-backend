CREATE TABLE grammar_attempts (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    id uuid NOT NULL,
    content_id uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    content_title text NOT NULL DEFAULT '',
    correct_count integer NOT NULL CHECK (correct_count >= 0),
    question_count integer NOT NULL CHECK (question_count BETWEEN 1 AND 100),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, id),
    CHECK (correct_count <= question_count)
);

CREATE INDEX grammar_attempts_user_updated_idx
    ON grammar_attempts (user_id, updated_at DESC);

CREATE INDEX grammar_attempts_user_content_idx
    ON grammar_attempts (user_id, content_id, created_at DESC);
