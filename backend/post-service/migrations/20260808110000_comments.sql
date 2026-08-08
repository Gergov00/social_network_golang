-- +goose Up
-- +goose StatementBegin
CREATE TABLE comments (
    id          UUID PRIMARY KEY,
    post_id     UUID        NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    author_id   UUID        NOT NULL,          -- == users.id, FK нет: другая БД
    text        TEXT        NOT NULL,
    likes_count INTEGER     NOT NULL DEFAULT 0 CHECK (likes_count >= 0),
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);

-- Комментарии читаются ПО ВОЗРАСТАНИЮ (ветка обсуждения читается сверху вниз),
-- в отличие от ленты. Поэтому и порядок в индексе прямой, а не DESC.
CREATE INDEX comments_post_idx ON comments (post_id, created_at, id);

CREATE TABLE comment_likes (
    comment_id UUID        NOT NULL REFERENCES comments (id) ON DELETE CASCADE,
    user_id    UUID        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (comment_id, user_id)          -- он же гарантия «один лайк на пользователя»
);

-- Денормализованный счётчик — по той же причине, что и likes_count: лента
-- обязана читаться одним запросом без агрегата по comments.
ALTER TABLE posts ADD COLUMN comments_count INTEGER NOT NULL DEFAULT 0 CHECK (comments_count >= 0);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE posts DROP COLUMN comments_count;
DROP TABLE comment_likes;
DROP TABLE comments;
-- +goose StatementEnd
