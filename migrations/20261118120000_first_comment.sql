-- +goose Up

-- POST-002: a comment posted under the post once it is out, where hashtags
-- and links go so they do not crowd the post. One per post; NULL is none.
ALTER TABLE posts
    ADD COLUMN first_comment text;

-- The comment's own progress per destination, separate from the post's, so
-- a comment that fails or is retried never touches the post. NULL means this
-- target gets no comment: the post has none, or the platform takes none.
ALTER TABLE post_targets
    ADD COLUMN comment_status text
        CHECK (comment_status IN ('pending', 'posting', 'posted', 'failed')),
    ADD COLUMN comment_remote_id text,
    ADD COLUMN comment_url text,
    ADD COLUMN comment_error text;

-- +goose Down

ALTER TABLE post_targets
    DROP COLUMN comment_error,
    DROP COLUMN comment_url,
    DROP COLUMN comment_remote_id,
    DROP COLUMN comment_status;

ALTER TABLE posts
    DROP COLUMN first_comment;
