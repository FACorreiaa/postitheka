-- +goose Up

-- WEB-001: a failed target can be sent again. interrupted marks a failure
-- where the attempt died mid-request, so the post may already be live and the
-- client warns before a retry; until now that was only in last_error's text.
-- retries counts the times it was sent again, and goes into the new job's
-- arguments so River does not take it for the job that already ran.
ALTER TABLE post_targets
    ADD COLUMN interrupted boolean NOT NULL DEFAULT false,
    ADD COLUMN retries integer NOT NULL DEFAULT 0;

UPDATE post_targets SET interrupted = true
WHERE status = 'failed' AND last_error LIKE 'an earlier attempt was interrupted%';

-- +goose Down

ALTER TABLE post_targets
    DROP COLUMN interrupted,
    DROP COLUMN retries;
