-- +goose Up
-- +goose StatementBegin
CREATE TABLE posts (
    id          UUID PRIMARY KEY,
    author_id   UUID        NOT NULL,          -- == users.id из user-service (без FK, database-per-service)
    text        TEXT        NOT NULL,
    likes_count INTEGER     NOT NULL DEFAULT 0 CHECK (likes_count >= 0),
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);

-- Индекс ровно под keyset-пагинацию ленты: ORDER BY created_at DESC, id DESC.
CREATE INDEX posts_feed_idx ON posts (created_at DESC, id DESC);

CREATE TABLE post_likes (
    post_id    UUID        NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL,           -- == users.id, FK нет: другая БД
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (post_id, user_id)             -- он же гарантия «один лайк на пользователя»
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE post_likes;
DROP TABLE posts;
-- +goose StatementEnd
