-- +goose Up
-- +goose StatementBegin
-- Индекс ровно под ListByAuthor: WHERE author_id = $ ORDER BY created_at DESC, id DESC.
-- Порядок колонок обязателен именно такой: сначала равенство (author_id),
-- потом ключ сортировки. posts_feed_idx для этого запроса бесполезен —
-- он не умеет отсекать чужие посты до сортировки.
CREATE INDEX posts_author_idx ON posts (author_id, created_at DESC, id DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX posts_author_idx;
-- +goose StatementEnd
