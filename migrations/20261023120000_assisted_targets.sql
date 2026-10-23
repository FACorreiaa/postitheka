-- +goose Up

-- Assisted platforms (WhatsApp) have no posting API: the iOS app hands the
-- post over through the share sheet. Their targets have no account and no
-- publish job; they wait in handing_off until the app reports whether the
-- user sent it (handed_off) or not (failed).
ALTER TABLE post_targets
    DROP CONSTRAINT post_targets_status_check,
    ADD CONSTRAINT post_targets_status_check
        CHECK (status IN ('pending', 'publishing', 'published', 'failed', 'handing_off', 'handed_off'));

-- +goose Down

UPDATE post_targets SET status = 'failed', last_error = 'handoff states removed'
WHERE status IN ('handing_off', 'handed_off');
ALTER TABLE post_targets
    DROP CONSTRAINT post_targets_status_check,
    ADD CONSTRAINT post_targets_status_check
        CHECK (status IN ('pending', 'publishing', 'published', 'failed'));
