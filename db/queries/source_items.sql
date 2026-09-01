-- name: UpsertSourceItem :one
INSERT INTO source_items (source, canonical_url, external_id, page_type, last_seen_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (source, canonical_url) DO UPDATE
SET external_id = COALESCE(EXCLUDED.external_id, source_items.external_id),
    page_type = CASE
        WHEN source_items.page_type LIKE 'article:%' AND EXCLUDED.page_type = 'article' THEN source_items.page_type
        ELSE COALESCE(EXCLUDED.page_type, source_items.page_type)
    END,
    last_seen_at = now()
RETURNING *;

-- name: GetSourceItem :one
SELECT * FROM source_items WHERE id = $1;

-- name: ListSourceItemsDueForFetch :many
SELECT *
FROM source_items
WHERE fetch_state IN ('discovered', 'scheduled', 'failed')
  AND (next_fetch_at IS NULL OR next_fetch_at <= now())
ORDER BY next_fetch_at NULLS FIRST, first_seen_at
LIMIT $1;

-- name: SetSourceFetchState :exec
UPDATE source_items
SET fetch_state = $2,
    next_fetch_at = $3
WHERE id = $1;

-- name: CreateSourceSnapshot :one
INSERT INTO source_snapshots (
    source_item_id, http_status, etag, last_modified, content_hash,
    object_key, parser_version, parse_state, error_code
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (source_item_id, content_hash) DO UPDATE
SET fetched_at = now(), http_status = EXCLUDED.http_status,
    etag = EXCLUDED.etag, last_modified = EXCLUDED.last_modified
RETURNING *;

-- name: MarkSnapshotParsed :exec
UPDATE source_snapshots
SET parser_version = $2, parse_state = $3, error_code = $4
WHERE id = $1;
