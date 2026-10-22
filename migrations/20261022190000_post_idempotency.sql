-- +goose Up

-- A client retrying POST /v1/posts after a lost response sends the same
-- Idempotency-Key; the key finds the post the first request created instead of
-- creating, and publishing, a second one. request_hash is a digest of the
-- request body, so a key reused for a different post is refused rather than
-- answered with the wrong post.
ALTER TABLE posts
    ADD COLUMN idempotency_key text,
    ADD COLUMN request_hash    bytea,
    ADD CONSTRAINT posts_idempotency_check
        CHECK ((idempotency_key IS NULL) = (request_hash IS NULL));

CREATE UNIQUE INDEX posts_user_id_idempotency_key_idx
    ON posts (user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- +goose Down

DROP INDEX posts_user_id_idempotency_key_idx;
ALTER TABLE posts
    DROP CONSTRAINT posts_idempotency_check,
    DROP COLUMN request_hash,
    DROP COLUMN idempotency_key;
