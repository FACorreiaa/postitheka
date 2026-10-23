-- +goose Up

-- One row per consent screen opened and not yet returned from. The state sent
-- to the platform is stored only as a hash, and the PKCE verifier sealed to
-- that hash: neither is any use to someone reading the table.
CREATE TABLE oauth_states (
    state_hash bytea PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    platform   text NOT NULL,
    verifier   bytea NOT NULL,
    client     text NOT NULL CHECK (client IN ('web', 'ios')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX oauth_states_user_id_idx ON oauth_states (user_id);

-- The platform's own id for what was connected (a LinkedIn member, an X
-- user). Connecting the same one again replaces its credentials instead of
-- adding a second account: OAuth tokens expire, and reconnecting is how a
-- user renews one. NULL for Discord, where the webhook is the account.
ALTER TABLE accounts ADD COLUMN remote_id text;

CREATE UNIQUE INDEX accounts_user_platform_remote_idx
    ON accounts (user_id, platform, remote_id) WHERE remote_id IS NOT NULL;

-- +goose Down

DROP INDEX accounts_user_platform_remote_idx;
ALTER TABLE accounts DROP COLUMN remote_id;
DROP TABLE oauth_states;
