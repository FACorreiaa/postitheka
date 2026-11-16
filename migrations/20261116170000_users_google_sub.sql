-- +goose Up
-- Sign in with Google (AUTH-002). Google's `sub` is the stable id for a person; an account is
-- found by it, never by email, as with apple_sub. Null for users who have not signed in with
-- Google: the first Google sign-in with the same verified email claims the row, whether the
-- CLI or Apple sign-in made it.
ALTER TABLE users ADD COLUMN google_sub text UNIQUE;

-- +goose Down
ALTER TABLE users DROP COLUMN google_sub;
