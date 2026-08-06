package domain

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"
)

// FeedCursor — позиция в ленте. Пара (CreatedAt, ID), а не offset: при
// вставке новых постов offset сдвигает окно, и клиент получает дубли и
// пропуски. Пара со вторичным ключом нужна, чтобы посты с одинаковым
// created_at не зациклили страницу.
type FeedCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// cursorSep разделяет части внутри закодированного курсора. Ни в RFC3339,
// ни в uuid этого символа быть не может.
const cursorSep = "|"

// Encode делает курсор непрозрачным для клиента: формат внутри — деталь
// реализации, клиент обязан возвращать строку как есть, не разбирая её.
// base64 URL-safe без паддинга — курсор ездит в query-строке.
func (c FeedCursor) Encode() string {
	raw := c.CreatedAt.UTC().Format(time.RFC3339Nano) + cursorSep + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// ParseCursor разбирает то, что вернул Encode. Любая некорректная строка —
// ErrInvalidCursor: курсор пришёл от клиента, доверия к нему нет.
func ParseCursor(s string) (*FeedCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, ErrInvalidCursor
	}

	parts := strings.Split(string(decoded), cursorSep)
	if len(parts) != 2 {
		return nil, ErrInvalidCursor
	}

	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, ErrInvalidCursor
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, ErrInvalidCursor
	}

	return &FeedCursor{CreatedAt: createdAt, ID: id}, nil
}
