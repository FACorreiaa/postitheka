-- +goose Up
-- Sign in with Apple (AUTH-001). Apple's `sub` is the stable id for a person in this team's
-- apps; an account is found by it, never by email, because a person can change or hide the
-- email Apple shares. Null for users created before Apple sign-in (by `server user create`):
-- the first Apple sign-in with the same verified email claims that row.
ALTER TABLE users ADD COLUMN apple_sub text UNIQUE;

-- +goose Down
ALTER TABLE users DROP COLUMN apple_sub;
