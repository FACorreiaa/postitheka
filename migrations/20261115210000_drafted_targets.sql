-- +goose Up

-- A video sent to a TikTok inbox is not published: its author finishes and
-- posts it in TikTok. Such a target ends as drafted, with published_at
-- holding when the platform took it.
ALTER TABLE post_targets
    DROP CONSTRAINT post_targets_status_check,
    ADD CONSTRAINT post_targets_status_check
        CHECK (status IN ('pending', 'publishing', 'published', 'failed', 'handing_off', 'handed_off', 'drafted'));

-- +goose Down

UPDATE post_targets SET status = 'published' WHERE status = 'drafted';
ALTER TABLE post_targets
    DROP CONSTRAINT post_targets_status_check,
    ADD CONSTRAINT post_targets_status_check
        CHECK (status IN ('pending', 'publishing', 'published', 'failed', 'handing_off', 'handed_off'));
