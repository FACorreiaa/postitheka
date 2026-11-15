-- +goose Up

-- ACC-001: credentials that have expired and cannot be refreshed, or were
-- revoked, fail every post until the user connects the account again. The
-- time a platform first said so is kept here, so clients can show the account
-- as needing reconnection and the server can stop sending to it. NULL means
-- the account works as far as anyone knows. Connecting it again clears it.
ALTER TABLE accounts ADD COLUMN needs_reconnect_at timestamptz;

-- +goose Down

ALTER TABLE accounts DROP COLUMN needs_reconnect_at;
