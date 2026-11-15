-- +goose Up

-- What the author chose for one destination, beyond the post itself: the
-- title of a Discord forum post today, a subreddit or a privacy level later.
-- Keys and limits are the platform's own (publisher.Rules.Options); the
-- column only carries them from the request to the publish job.
ALTER TABLE post_targets
    ADD COLUMN options jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down

ALTER TABLE post_targets
    DROP COLUMN options;
