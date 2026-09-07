ALTER TABLE retell_attempts
    DROP CONSTRAINT IF EXISTS retell_attempts_matched_keywords_check,
    DROP COLUMN IF EXISTS review_client_updated_at,
    DROP COLUMN IF EXISTS completion_score,
    DROP COLUMN IF EXISTS word_count,
    DROP COLUMN IF EXISTS keyword_count,
    DROP COLUMN IF EXISTS matched_keywords,
    DROP COLUMN IF EXISTS transcript;
