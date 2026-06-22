-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id         UUID PRIMARY KEY,                 -- == credentials.id из auth (без FK, database-per-service)
    name       TEXT        NOT NULL DEFAULT '',
    handle     CITEXT      UNIQUE NOT NULL,
    bio        TEXT        NOT NULL DEFAULT '',
    city       TEXT        NOT NULL DEFAULT '',
    work       TEXT        NOT NULL DEFAULT '',
    verified   BOOLEAN     NOT NULL DEFAULT false,
    avatar_url TEXT,                             -- nullable: media отложена, аватар опционален
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE users;
-- +goose StatementEnd
