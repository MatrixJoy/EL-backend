-- name: UpsertContent :one
INSERT INTO contents (
    source_item_id, slug, type, title, level, published_at, body_blocks,
    rights_status, attribution, status, source_updated_at, released_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::rights_status, $9,
    CASE WHEN $8::rights_status = 'public_domain_verified'::rights_status THEN 'published'::content_status ELSE 'review'::content_status END,
    $6,
    CASE WHEN $8::rights_status = 'public_domain_verified'::rights_status THEN now() ELSE NULL END)
ON CONFLICT (source_item_id) DO UPDATE
SET title = EXCLUDED.title,
    type = EXCLUDED.type,
    level = EXCLUDED.level,
    published_at = EXCLUDED.published_at,
    body_blocks = EXCLUDED.body_blocks,
    rights_status = EXCLUDED.rights_status,
    attribution = EXCLUDED.attribution,
    status = CASE WHEN EXCLUDED.rights_status = 'public_domain_verified'
                  THEN 'published'::content_status ELSE 'review'::content_status END,
    released_at = CASE
        WHEN contents.status <> 'published'::content_status
             AND EXCLUDED.status = 'published'::content_status THEN now()
        ELSE contents.released_at
    END,
    source_updated_at = EXCLUDED.source_updated_at,
    revision = contents.revision + 1,
    updated_at = now()
RETURNING *;

-- name: UpsertAsset :one
INSERT INTO assets (
    kind, source_url, source_host, mime_type, byte_size, quality_label,
    delivery_policy, rights_status, attribution
) VALUES ($1, $2, $3, $4, $5, $6,
    CASE WHEN $7::rights_status = 'public_domain_verified'::rights_status THEN 'stream_proxy'::delivery_policy
         ELSE 'source_page_only'::delivery_policy END,
    $7::rights_status, $8)
ON CONFLICT (source_url) DO UPDATE
SET kind = EXCLUDED.kind, mime_type = EXCLUDED.mime_type,
    byte_size = EXCLUDED.byte_size, quality_label = EXCLUDED.quality_label,
    delivery_policy = EXCLUDED.delivery_policy,
    rights_status = EXCLUDED.rights_status,
    attribution = EXCLUDED.attribution,
    updated_at = now()
RETURNING *;

-- name: LinkContentAsset :exec
INSERT INTO content_assets (content_id, asset_id, role, position)
VALUES ($1, $2, $3, $4)
ON CONFLICT (content_id, asset_id, role) DO UPDATE SET position = EXCLUDED.position;

-- name: CreatePublishEvent :one
INSERT INTO publish_events (entity_type, entity_id, operation, revision)
VALUES ($1, $2, $3, $4)
RETURNING sequence;

-- name: ListContentsByStatus :many
SELECT * FROM contents
WHERE status = $1
ORDER BY updated_at DESC, id
LIMIT $2;

-- name: SetContentStatus :one
UPDATE contents
SET status = $2,
    released_at = CASE
        WHEN status <> 'published'::content_status
             AND $2 = 'published'::content_status THEN now()
        ELSE released_at
    END,
    revision = revision + 1,
    updated_at = now()
WHERE id = $1
RETURNING *;
