DROP INDEX IF EXISTS contents_release_feed_idx;

ALTER TABLE contents
    DROP COLUMN IF EXISTS released_at;
