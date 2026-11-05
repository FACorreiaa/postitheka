-- name: CreateUser :one
INSERT INTO users (email) VALUES ($1)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: CreateAPIToken :exec
INSERT INTO api_tokens (user_id, token_hash, name) VALUES ($1, $2, $3);

-- Resolves a token and records its use in one statement, so authenticating a
-- request costs one round trip.
-- name: UseAPIToken :one
UPDATE api_tokens SET last_used_at = now()
WHERE token_hash = $1
RETURNING user_id;

-- name: CreateAccount :one
INSERT INTO accounts (id, user_id, platform, display_name, credentials, remote_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, platform, display_name, created_at;

-- name: GetAccountByRemoteID :one
SELECT id, user_id, platform, display_name, created_at
FROM accounts
WHERE user_id = $1 AND platform = $2 AND remote_id = $3;

-- name: ReplaceAccountCredentials :one
UPDATE accounts SET credentials = $3, display_name = $4
WHERE id = $1 AND user_id = $2
RETURNING id, user_id, platform, display_name, created_at;

-- Held while credentials are refreshed, so two jobs for one account do not
-- both spend a refresh token that rotates.
-- name: LockAccountCredentials :one
SELECT credentials FROM accounts WHERE id = $1 FOR UPDATE;

-- name: UpdateAccountCredentials :exec
UPDATE accounts SET credentials = $2 WHERE id = $1;

-- name: CreateOAuthState :exec
INSERT INTO oauth_states (state_hash, user_id, platform, verifier, client)
VALUES ($1, $2, $3, $4, $5);

-- Taking a state deletes it, so a callback replayed with the same state
-- finds nothing.
-- name: TakeOAuthState :one
DELETE FROM oauth_states
WHERE state_hash = $1 AND platform = $2
RETURNING *;

-- name: DeleteStaleOAuthStates :exec
DELETE FROM oauth_states WHERE user_id = $1 AND created_at < $2;

-- name: ListAccounts :many
SELECT id, user_id, platform, display_name, created_at
FROM accounts
WHERE user_id = $1
ORDER BY created_at;

-- name: GetAccountsByIDs :many
SELECT id, user_id, platform, display_name, created_at
FROM accounts
WHERE user_id = $1 AND id = ANY(sqlc.arg(ids)::uuid[]);

-- name: DeleteAccount :execrows
DELETE FROM accounts WHERE id = $1 AND user_id = $2;

-- name: CreateMedia :exec
INSERT INTO media (id, user_id, object_key, content_type, size_bytes)
VALUES ($1, $2, $3, $4, $5);

-- name: GetMediaByIDs :many
SELECT * FROM media
WHERE user_id = $1 AND id = ANY(sqlc.arg(ids)::uuid[]);

-- name: MarkMediaVerified :exec
UPDATE media SET verified_at = now(), size_bytes = $2, content_type = $3
WHERE id = $1;

-- Not scoped by user: the sweep is the system cleaning up after everyone.
-- SKIP LOCKED passes over a row a post is attaching right now (the post_media
-- insert holds a key-share lock on it), and FOR UPDATE stops one being
-- attached while the sweep holds it.
-- name: LockUnattachedMedia :many
SELECT m.id FROM media m
WHERE m.created_at < $1
  AND NOT EXISTS (SELECT 1 FROM post_media pm WHERE pm.media_id = m.id)
ORDER BY m.created_at
LIMIT $2
FOR UPDATE OF m SKIP LOCKED;

-- The NOT EXISTS is checked again with a fresh snapshot: a post that
-- committed between the lock query's snapshot and its lock would otherwise
-- have its media deleted, and post_media cascades.
-- name: DeleteUnattachedMedia :many
DELETE FROM media m
WHERE m.id = ANY(sqlc.arg(ids)::uuid[])
  AND NOT EXISTS (SELECT 1 FROM post_media pm WHERE pm.media_id = m.id)
RETURNING m.object_key;

-- name: CreatePost :one
INSERT INTO posts (user_id, text, scheduled_at, idempotency_key, request_hash)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: AddPostMedia :exec
INSERT INTO post_media (post_id, media_id, position) VALUES ($1, $2, $3);

-- name: CreatePostTarget :one
INSERT INTO post_targets (post_id, account_id, platform) VALUES ($1, $2, $3)
RETURNING *;

-- name: CreateAssistedTarget :one
INSERT INTO post_targets (post_id, platform, status) VALUES ($1, $2, 'handing_off')
RETURNING *;

-- name: GetTargetForUser :one
SELECT t.* FROM post_targets t
JOIN posts p ON p.id = t.post_id
WHERE t.id = $1 AND p.user_id = $2;

-- Conditional on handing_off, so a late or repeated report cannot overwrite
-- an outcome already recorded.
-- name: CompleteHandoff :one
UPDATE post_targets t
SET status = sqlc.arg(status),
    last_error = sqlc.narg(last_error),
    published_at = CASE WHEN sqlc.arg(status) = 'handed_off' THEN now() END,
    updated_at = now()
FROM posts p
WHERE t.id = sqlc.arg(id) AND p.id = t.post_id AND p.user_id = sqlc.arg(user_id)
  AND t.status = 'handing_off'
RETURNING t.*;

-- A handoff the phone never reported (app killed with the share sheet open)
-- would otherwise wait forever.
-- name: ExpireHandoffs :execrows
UPDATE post_targets
SET status = 'failed', last_error = sqlc.arg(last_error), updated_at = now()
WHERE status = 'handing_off' AND updated_at < sqlc.arg(cutoff);

-- name: GetPost :one
SELECT * FROM posts WHERE id = $1 AND user_id = $2;

-- name: GetPostByIdempotencyKey :one
SELECT * FROM posts WHERE user_id = $1 AND idempotency_key = $2;

-- name: ListPosts :many
SELECT * FROM posts
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListPostTargets :many
SELECT * FROM post_targets
WHERE post_id = ANY(sqlc.arg(post_ids)::uuid[])
ORDER BY post_id, platform, id;

-- name: ListPostMedia :many
SELECT pm.post_id, m.*
FROM post_media pm
JOIN media m ON m.id = pm.media_id
WHERE pm.post_id = ANY(sqlc.arg(post_ids)::uuid[])
ORDER BY pm.post_id, pm.position;

-- Everything the publish job needs, in one read. LEFT JOIN because the
-- account may have been disconnected since the post was made.
-- name: GetTargetForPublish :one
SELECT
    t.id, t.post_id, t.account_id, t.platform, t.status,
    p.user_id, p.text,
    a.credentials
FROM post_targets t
JOIN posts p ON p.id = t.post_id
LEFT JOIN accounts a ON a.id = t.account_id
WHERE t.id = $1;

-- The claim before a request goes out. Conditional on 'pending' so that two
-- runs of the same job cannot both get past it.
-- name: MarkTargetPublishing :execrows
UPDATE post_targets SET status = 'publishing', updated_at = now()
WHERE id = $1 AND status = 'pending';

-- name: MarkTargetPublished :exec
UPDATE post_targets
SET status = 'published', remote_id = $2, remote_url = $3, last_error = NULL,
    published_at = now(), updated_at = now()
WHERE id = $1;

-- name: MarkTargetPending :exec
UPDATE post_targets SET status = 'pending', last_error = $2, updated_at = now()
WHERE id = $1;

-- name: MarkTargetFailed :exec
UPDATE post_targets SET status = 'failed', last_error = $2, updated_at = now()
WHERE id = $1;
