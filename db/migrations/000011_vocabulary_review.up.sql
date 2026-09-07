ALTER TABLE vocabulary_entries
    ADD COLUMN review_stage integer NOT NULL DEFAULT 0 CHECK (review_stage BETWEEN 0 AND 6),
    ADD COLUMN review_count integer NOT NULL DEFAULT 0 CHECK (review_count >= 0),
    ADD COLUMN lapse_count integer NOT NULL DEFAULT 0 CHECK (lapse_count >= 0),
    ADD COLUMN review_due_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN last_reviewed_at timestamptz,
    ADD COLUMN review_client_updated_at timestamptz NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    ADD CONSTRAINT vocabulary_entries_review_counts_check CHECK (lapse_count <= review_count);

CREATE INDEX vocabulary_entries_user_review_due_idx
    ON vocabulary_entries (user_id, review_due_at, word);
