ALTER TABLE retell_attempts
    ADD COLUMN transcript text,
    ADD COLUMN matched_keywords text[] NOT NULL DEFAULT '{}',
    ADD COLUMN keyword_count integer NOT NULL DEFAULT 0 CHECK (keyword_count BETWEEN 0 AND 100),
    ADD COLUMN word_count integer NOT NULL DEFAULT 0 CHECK (word_count BETWEEN 0 AND 5000),
    ADD COLUMN completion_score integer NOT NULL DEFAULT 0 CHECK (completion_score BETWEEN 0 AND 100),
    ADD COLUMN review_client_updated_at timestamptz NOT NULL DEFAULT '1970-01-01T00:00:00Z',
    ADD CONSTRAINT retell_attempts_matched_keywords_check CHECK (cardinality(matched_keywords) <= keyword_count);
