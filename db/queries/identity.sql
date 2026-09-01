-- name: UpsertAppleUser :one
INSERT INTO users (apple_subject) VALUES ($1)
ON CONFLICT (apple_subject) DO UPDATE SET updated_at = now()
RETURNING *;

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)
RETURNING *;

-- name: RecordAppleIdentityTokenUse :exec
INSERT INTO apple_identity_token_uses (token_hash, user_id) VALUES ($1, $2);

-- name: GetUserBySessionHash :one
SELECT u.* FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: UpsertBookmark :exec
INSERT INTO bookmarks (user_id, content_id) VALUES ($1, $2)
ON CONFLICT (user_id, content_id) DO UPDATE SET updated_at = now();

-- name: DeleteBookmark :exec
DELETE FROM bookmarks WHERE user_id = $1 AND content_id = $2;

-- name: ListBookmarks :many
SELECT b.content_id, b.updated_at FROM bookmarks b
JOIN contents c ON c.id = b.content_id
WHERE b.user_id = $1 AND c.status = 'published'
ORDER BY b.updated_at DESC;

-- name: UpsertProgress :one
INSERT INTO learning_progress (user_id, content_id, position_seconds, completed, client_updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, content_id) DO UPDATE
SET position_seconds = CASE WHEN learning_progress.client_updated_at <= EXCLUDED.client_updated_at THEN EXCLUDED.position_seconds ELSE learning_progress.position_seconds END,
    completed = CASE WHEN learning_progress.client_updated_at <= EXCLUDED.client_updated_at THEN EXCLUDED.completed ELSE learning_progress.completed END,
    client_updated_at = GREATEST(learning_progress.client_updated_at, EXCLUDED.client_updated_at),
    server_updated_at = now()
RETURNING *;

-- name: ListProgress :many
SELECT * FROM learning_progress WHERE user_id = $1 ORDER BY server_updated_at DESC;

-- name: CreateUserEvent :one
INSERT INTO user_events (user_id, entity_type, entity_id, operation)
VALUES ($1, $2, $3, $4) RETURNING sequence;

-- name: ListUserEventsAfter :many
SELECT * FROM user_events WHERE user_id = $1 AND sequence > $2 ORDER BY sequence LIMIT $3;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;
