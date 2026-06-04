-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE credentials (
    id            UUID PRIMARY KEY,
    email         CITEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS credentials;