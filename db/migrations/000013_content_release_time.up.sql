ALTER TABLE contents
    ADD COLUMN released_at timestamptz;

-- Existing rows predate an explicit CMS release timestamp. Their creation time is
-- the closest durable record of when they entered our content library.
UPDATE contents
SET released_at = created_at
WHERE status = 'published'
  AND released_at IS NULL;

CREATE INDEX contents_release_feed_idx
    ON contents (status, released_at DESC, id DESC);
