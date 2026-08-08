package domain

import (
	"context"

	"github.com/google/uuid"
)

// PostRepository — порт хранилища. Методы под сценарии, а не generic CRUD:
// viewerID отдельным параметром, потому что Liked — это проекция на
// зрителя, и вычислять её вторым запросом на каждый пост незачем.
type PostRepository interface {
	Create(ctx context.Context, post *Post) error
	GetByID(ctx context.Context, id, viewerID uuid.UUID) (*Post, error)
	// ListFeed отдаёт ровно limit записей (или меньше, если лента
	// кончилась). Про «есть ли ещё» решает вызывающий, запросив limit+1.
	ListFeed(ctx context.Context, viewerID uuid.UUID, cursor *FeedCursor, limit int) ([]Post, error)
	// ListByAuthor — та же keyset-пагинация, что и у ленты, но с фильтром по
	// автору. authorID и viewerID разные не для красоты: первый выбирает
	// строки, второй считает Liked. Совпадают они только на своём профиле.
	ListByAuthor(ctx context.Context, authorID, viewerID uuid.UUID, cursor *FeedCursor, limit int) ([]Post, error)
	// Like и Unlike идемпотентны и возвращают свежий пост: счётчик и флаг
	// зрителя клиенту нужны одним ответом, вторым запросом их читать —
	// гонка.
	Like(ctx context.Context, postID, userID uuid.UUID) (*Post, error)
	Unlike(ctx context.Context, postID, userID uuid.UUID) (*Post, error)
}
