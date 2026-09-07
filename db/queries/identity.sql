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

-- name: UpsertVocabularyEntry :one
INSERT INTO vocabulary_entries (
    id, user_id, word, normalized_word, definition, sentence_context,
    content_id, content_title, created_at
)
VALUES (
    sqlc.arg('id'), sqlc.arg('user_id'), sqlc.arg('word'), sqlc.arg('normalized_word'),
    NULLIF(sqlc.arg('definition')::text, ''),
    NULLIF(sqlc.arg('sentence_context')::text, ''),
    sqlc.narg('content_id'),
    NULLIF(sqlc.arg('content_title')::text, ''),
    sqlc.arg('created_at')
)
ON CONFLICT (user_id, normalized_word) DO UPDATE
SET word = EXCLUDED.word,
    definition = COALESCE(EXCLUDED.definition, vocabulary_entries.definition),
    sentence_context = COALESCE(EXCLUDED.sentence_context, vocabulary_entries.sentence_context),
    content_id = COALESCE(EXCLUDED.content_id, vocabulary_entries.content_id),
    content_title = COALESCE(EXCLUDED.content_title, vocabulary_entries.content_title),
    created_at = LEAST(vocabulary_entries.created_at, EXCLUDED.created_at),
    updated_at = now()
RETURNING *;

-- name: ListVocabularyEntries :many
SELECT * FROM vocabulary_entries
WHERE user_id = $1
ORDER BY review_due_at, updated_at DESC, word;

-- name: UpdateVocabularyReview :one
UPDATE vocabulary_entries
SET review_stage = CASE
        WHEN review_client_updated_at <= sqlc.arg('client_updated_at') THEN sqlc.arg('stage')
        ELSE review_stage
    END,
    review_count = CASE
        WHEN review_client_updated_at <= sqlc.arg('client_updated_at') THEN sqlc.arg('review_count')
        ELSE review_count
    END,
    lapse_count = CASE
        WHEN review_client_updated_at <= sqlc.arg('client_updated_at') THEN sqlc.arg('lapse_count')
        ELSE lapse_count
    END,
    review_due_at = CASE
        WHEN review_client_updated_at <= sqlc.arg('client_updated_at') THEN sqlc.arg('due_at')
        ELSE review_due_at
    END,
    last_reviewed_at = CASE
        WHEN review_client_updated_at <= sqlc.arg('client_updated_at') THEN sqlc.narg('last_reviewed_at')
        ELSE last_reviewed_at
    END,
    review_client_updated_at = GREATEST(review_client_updated_at, sqlc.arg('client_updated_at'))
WHERE user_id = sqlc.arg('user_id') AND id = sqlc.arg('id')
RETURNING *;

-- name: DeleteVocabularyEntry :exec
DELETE FROM vocabulary_entries WHERE user_id = $1 AND id = $2;

-- name: UpsertGrammarAttempt :one
INSERT INTO grammar_attempts (
    user_id, id, content_id, content_title, correct_count, question_count, created_at
) VALUES (
    sqlc.arg('user_id'), sqlc.arg('id'), sqlc.arg('content_id'),
    sqlc.arg('content_title'), sqlc.arg('correct_count'),
    sqlc.arg('question_count'), sqlc.arg('created_at')
)
ON CONFLICT (user_id, id) DO UPDATE
SET content_id = EXCLUDED.content_id,
    content_title = EXCLUDED.content_title,
    correct_count = EXCLUDED.correct_count,
    question_count = EXCLUDED.question_count,
    created_at = EXCLUDED.created_at,
    updated_at = now()
RETURNING *;

-- name: ListGrammarAttempts :many
SELECT * FROM grammar_attempts
WHERE user_id = $1
ORDER BY created_at DESC, id;

-- name: CreateUserEvent :one
INSERT INTO user_events (user_id, entity_type, entity_id, operation)
VALUES ($1, $2, $3, $4) RETURNING sequence;

-- name: ListUserEventsAfter :many
SELECT * FROM user_events WHERE user_id = $1 AND sequence > $2 ORDER BY sequence LIMIT $3;

-- name: UpsertRetellAttempt :one
INSERT INTO retell_attempts (
    user_id, id, content_id, object_key, duration_milliseconds, byte_size, content_type, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (user_id, id) DO UPDATE
SET content_id = EXCLUDED.content_id,
    object_key = EXCLUDED.object_key,
    duration_milliseconds = EXCLUDED.duration_milliseconds,
    byte_size = EXCLUDED.byte_size,
    content_type = EXCLUDED.content_type,
    created_at = EXCLUDED.created_at,
    server_updated_at = now()
RETURNING *;

-- name: ListRetellAttempts :many
SELECT * FROM retell_attempts
WHERE user_id = $1
  AND (sqlc.narg('content_id')::uuid IS NULL OR content_id = sqlc.narg('content_id'))
ORDER BY created_at DESC;

-- name: GetRetellAttempt :one
SELECT * FROM retell_attempts WHERE user_id = $1 AND id = $2;

-- name: UpdateRetellReview :one
UPDATE retell_attempts
SET transcript = CASE
        WHEN review_client_updated_at <= sqlc.arg('client_updated_at') THEN sqlc.arg('transcript')
        ELSE transcript
    END,
    matched_keywords = CASE
        WHEN review_client_updated_at <= sqlc.arg('client_updated_at') THEN sqlc.arg('matched_keywords')
        ELSE matched_keywords
    END,
    keyword_count = CASE
        WHEN review_client_updated_at <= sqlc.arg('client_updated_at') THEN sqlc.arg('keyword_count')
        ELSE keyword_count
    END,
    word_count = CASE
        WHEN review_client_updated_at <= sqlc.arg('client_updated_at') THEN sqlc.arg('word_count')
        ELSE word_count
    END,
    completion_score = CASE
        WHEN review_client_updated_at <= sqlc.arg('client_updated_at') THEN sqlc.arg('completion_score')
        ELSE completion_score
    END,
    review_client_updated_at = GREATEST(review_client_updated_at, sqlc.arg('client_updated_at')),
    server_updated_at = now()
WHERE user_id = sqlc.arg('user_id') AND id = sqlc.arg('id')
RETURNING *;

-- name: DeleteRetellAttempt :one
DELETE FROM retell_attempts WHERE user_id = $1 AND id = $2
RETURNING object_key;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;
