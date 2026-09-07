DROP INDEX IF EXISTS vocabulary_entries_user_review_due_idx;

ALTER TABLE vocabulary_entries
    DROP CONSTRAINT IF EXISTS vocabulary_entries_review_counts_check,
    DROP COLUMN IF EXISTS review_client_updated_at,
    DROP COLUMN IF EXISTS last_reviewed_at,
    DROP COLUMN IF EXISTS review_due_at,
    DROP COLUMN IF EXISTS lapse_count,
    DROP COLUMN IF EXISTS review_count,
    DROP COLUMN IF EXISTS review_stage;
