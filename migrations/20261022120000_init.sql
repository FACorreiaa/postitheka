-- +goose Up

CREATE TABLE users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email      text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Bearer tokens for the API. Only the SHA-256 of a token is stored, so a
-- leaked database does not hand out working credentials. Sign in with Apple
-- replaces minting these by hand; the table stays as the session store.
CREATE TABLE api_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash   bytea NOT NULL UNIQUE,
    name         text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

-- A connected destination: one Discord webhook, one X user, one Facebook Page.
--
-- credentials is a secretbox-sealed blob whose shape belongs to the platform's
-- publisher, with the row id as associated data. The id is therefore generated
-- by the application rather than defaulted here: it has to exist before the
-- blob can be sealed to it, and a blob copied onto another row will not open.
CREATE TABLE accounts (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    platform     text NOT NULL,
    display_name text NOT NULL,
    credentials  bytea NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX accounts_user_id_idx ON accounts (user_id);

-- An uploaded file. The row is written when the upload URL is handed out, and
-- verified_at is set once the object has been seen in storage with a size and
-- type that match the rules. Until then it cannot be attached to a post.
CREATE TABLE media (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    object_key   text NOT NULL UNIQUE,
    content_type text NOT NULL,
    size_bytes   bigint NOT NULL,
    verified_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE posts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    text         text NOT NULL DEFAULT '',
    scheduled_at timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX posts_user_id_created_at_idx ON posts (user_id, created_at DESC);

CREATE TABLE post_media (
    post_id  uuid NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    media_id uuid NOT NULL REFERENCES media (id) ON DELETE RESTRICT,
    position int NOT NULL,
    PRIMARY KEY (post_id, position)
);

-- One row per destination of a post, because the destinations succeed and
-- fail independently: a post can be live on Discord and rejected by X.
--
-- account_id goes NULL rather than taking the row with it when an account is
-- disconnected, so the history of what was posted where survives; platform is
-- copied for the same reason.
--
-- status:
--   pending     waiting for its job, or for a retry
--   publishing  a request to the platform is in flight
--   published   the platform accepted it; remote_id/remote_url say where
--   failed      gave up; last_error says why
CREATE TABLE post_targets (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id      uuid NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    account_id   uuid REFERENCES accounts (id) ON DELETE SET NULL,
    platform     text NOT NULL,
    status       text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'publishing', 'published', 'failed')),
    last_error   text,
    remote_id    text,
    remote_url   text,
    published_at timestamptz,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (post_id, account_id)
);

CREATE INDEX post_targets_post_id_idx ON post_targets (post_id);

-- +goose Down

DROP TABLE post_targets;
DROP TABLE post_media;
DROP TABLE posts;
DROP TABLE media;
DROP TABLE accounts;
DROP TABLE api_tokens;
DROP TABLE users;
