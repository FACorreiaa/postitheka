-- +goose Up

-- VID-001: platforms limit a video's length and shape, and this server has no
-- way to measure either (there is no ffmpeg in the cluster). The client reads
-- them from the file before uploading and declares them with the upload. NULL
-- means the client could not tell, and the checks that need the value are
-- skipped; the platform still refuses what it does not accept.
ALTER TABLE media
    ADD COLUMN duration_ms integer CHECK (duration_ms > 0),
    ADD COLUMN width       integer CHECK (width > 0),
    ADD COLUMN height      integer CHECK (height > 0);

-- +goose Down

ALTER TABLE media
    DROP COLUMN duration_ms,
    DROP COLUMN width,
    DROP COLUMN height;
