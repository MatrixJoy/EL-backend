-- name: ListCategories :many
SELECT cat.* FROM categories cat
WHERE cat.status = 'active'
  AND EXISTS (
    SELECT 1 FROM content_categories cc
    JOIN contents c ON c.id = cc.content_id
    JOIN content_assets ca ON ca.content_id = c.id
    JOIN assets a ON a.id = ca.asset_id
    WHERE cc.category_id = cat.id AND c.status = 'published'
      AND a.kind = 'audio' AND a.delivery_policy = 'managed_cache' AND a.availability = 'available'
  )
ORDER BY cat.sort_order, cat.name;

-- name: ListCategoryContents :many
SELECT c.*, s.canonical_url
FROM content_categories cc
JOIN categories cat ON cat.id = cc.category_id
JOIN contents c ON c.id = cc.content_id
JOIN source_items s ON s.id = c.source_item_id
WHERE cat.slug = $1 AND c.status = 'published'
  AND EXISTS (
    SELECT 1 FROM content_assets playable_ca
    JOIN assets playable ON playable.id = playable_ca.asset_id
    WHERE playable_ca.content_id = c.id AND playable.kind = 'audio'
      AND playable.delivery_policy = 'managed_cache' AND playable.availability = 'available'
  )
ORDER BY c.published_at DESC NULLS LAST, c.id DESC
LIMIT $2;

-- name: ListPublishedContents :many
SELECT c.*, s.canonical_url
FROM contents c JOIN source_items s ON s.id = c.source_item_id
WHERE c.status = 'published'
  AND EXISTS (
    SELECT 1 FROM content_assets ca
    JOIN assets a ON a.id = ca.asset_id
    WHERE ca.content_id = c.id AND a.kind = 'audio'
      AND a.delivery_policy = 'managed_cache' AND a.availability = 'available'
  )
  AND (sqlc.narg(before_published_at)::timestamptz IS NULL
       OR (c.published_at, c.id) < (sqlc.narg(before_published_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY c.published_at DESC NULLS LAST, c.id DESC
LIMIT sqlc.arg(page_limit);

-- name: GetPublishedContent :one
SELECT c.*, s.canonical_url
FROM contents c JOIN source_items s ON s.id = c.source_item_id
WHERE c.id = $1 AND c.status = 'published'
  AND EXISTS (
    SELECT 1 FROM content_assets ca
    JOIN assets a ON a.id = ca.asset_id
    WHERE ca.content_id = c.id AND a.kind = 'audio'
      AND a.delivery_policy = 'managed_cache' AND a.availability = 'available'
  );

-- name: ListContentAssets :many
SELECT a.*, ca.role, ca.position
FROM content_assets ca JOIN assets a ON a.id = ca.asset_id
WHERE ca.content_id = $1
ORDER BY ca.position, a.id;

-- name: SearchPublishedContents :many
SELECT c.*, s.canonical_url,
       ts_rank(to_tsvector('english', coalesce(c.title, '') || ' ' || coalesce(c.summary, '')), websearch_to_tsquery('english', $1)) AS rank
FROM contents c JOIN source_items s ON s.id = c.source_item_id
WHERE c.status = 'published'
  AND EXISTS (
    SELECT 1 FROM content_assets ca
    JOIN assets a ON a.id = ca.asset_id
    WHERE ca.content_id = c.id AND a.kind = 'audio'
      AND a.delivery_policy = 'managed_cache' AND a.availability = 'available'
  )
  AND to_tsvector('english', coalesce(c.title, '') || ' ' || coalesce(c.summary, '')) @@ websearch_to_tsquery('english', $1)
ORDER BY rank DESC, c.published_at DESC NULLS LAST
LIMIT $2;

-- name: ListPublishEventsAfter :many
SELECT pe.* FROM publish_events pe
WHERE pe.sequence > $1 AND pe.entity_type = 'content'
  AND EXISTS (
    SELECT 1 FROM contents c
    JOIN content_assets ca ON ca.content_id = c.id
    JOIN assets a ON a.id = ca.asset_id
    WHERE c.id = pe.entity_id AND c.status = 'published' AND a.kind = 'audio'
      AND a.delivery_policy = 'managed_cache' AND a.availability = 'available'
  )
ORDER BY pe.sequence LIMIT $2;

-- name: GetDeliverableAsset :one
SELECT * FROM assets
WHERE id = $1 AND availability = 'available'
  AND ((rights_status = 'public_domain_verified' AND delivery_policy = 'stream_proxy')
       OR (delivery_policy = 'managed_cache' AND object_key IS NOT NULL));

-- name: ListSeries :many
SELECT s.* FROM series s
WHERE s.status = 'active'
  AND EXISTS (
    SELECT 1 FROM content_series cs
    JOIN contents c ON c.id = cs.content_id
    JOIN content_assets ca ON ca.content_id = c.id
    JOIN assets a ON a.id = ca.asset_id
    WHERE cs.series_id = s.id AND c.status = 'published' AND a.kind = 'audio'
      AND a.delivery_policy = 'managed_cache' AND a.availability = 'available'
  )
ORDER BY s.title LIMIT $1;

-- name: GetSeries :one
SELECT s.* FROM series s
WHERE s.id = $1 AND s.status = 'active'
  AND EXISTS (
    SELECT 1 FROM content_series cs
    JOIN contents c ON c.id = cs.content_id
    JOIN content_assets ca ON ca.content_id = c.id
    JOIN assets a ON a.id = ca.asset_id
    WHERE cs.series_id = s.id AND c.status = 'published' AND a.kind = 'audio'
      AND a.delivery_policy = 'managed_cache' AND a.availability = 'available'
  );

-- name: ListSeriesContents :many
SELECT c.*, s.canonical_url, cs.position
FROM content_series cs
JOIN contents c ON c.id = cs.content_id
JOIN source_items s ON s.id = c.source_item_id
WHERE cs.series_id = $1 AND c.status = 'published'
  AND EXISTS (
    SELECT 1 FROM content_assets playable_ca
    JOIN assets playable ON playable.id = playable_ca.asset_id
    WHERE playable_ca.content_id = c.id AND playable.kind = 'audio'
      AND playable.delivery_policy = 'managed_cache' AND playable.availability = 'available'
  )
ORDER BY cs.position, c.published_at, c.id;

-- name: UpsertSeries :one
INSERT INTO series (slug, title) VALUES ($1, $2)
ON CONFLICT (slug) DO UPDATE SET title = EXCLUDED.title, updated_at = now()
RETURNING *;

-- name: LinkContentSeries :exec
INSERT INTO content_series (content_id, series_id, position) VALUES ($1, $2, $3)
ON CONFLICT (content_id) DO UPDATE SET series_id = EXCLUDED.series_id, position = EXCLUDED.position;

-- name: LinkContentCategoryBySlug :exec
INSERT INTO content_categories (content_id, category_id, is_primary)
SELECT $1, id, true FROM categories WHERE slug = $2
ON CONFLICT (content_id, category_id) DO UPDATE SET is_primary = true;
