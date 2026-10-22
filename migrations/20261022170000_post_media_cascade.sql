-- +goose Up

-- RESTRICT is checked the moment a media row is deleted, before the cascade
-- from posts has removed the post_media rows pointing at it. So deleting a
-- user who had attached a file failed: their media could not go because their
-- own posts still referenced it. Media is only ever deleted with its owner,
-- whose posts go in the same statement, so CASCADE loses nothing.
ALTER TABLE post_media
    DROP CONSTRAINT post_media_media_id_fkey,
    ADD CONSTRAINT post_media_media_id_fkey
        FOREIGN KEY (media_id) REFERENCES media (id) ON DELETE CASCADE;

-- +goose Down

ALTER TABLE post_media
    DROP CONSTRAINT post_media_media_id_fkey,
    ADD CONSTRAINT post_media_media_id_fkey
        FOREIGN KEY (media_id) REFERENCES media (id) ON DELETE RESTRICT;
