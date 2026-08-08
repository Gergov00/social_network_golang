package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Comment struct {
	ID       uuid.UUID
	PostID   uuid.UUID
	AuthorID uuid.UUID
	Text     string
	// LikesCount денормализован в comments.likes_count и двигается только
	// вместе с comment_likes, в одной транзакции.
	LikesCount int32
	// Liked — проекция на конкретного зрителя, а не свойство комментария.
	Liked     bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CommentRepository — порт хранилища комментариев. Форма повторяет
// PostRepository намеренно: viewerID отдельным параметром (Liked — проекция
// на зрителя), Like/Unlike идемпотентны и возвращают свежую сущность.
type CommentRepository interface {
	// Create вставляет комментарий и двигает posts.comments_count в ОДНОЙ
	// транзакции. Комментарий к несуществующему посту — ErrNotFound.
	// Инвариант: если появится удаление комментария, оно ОБЯЗАНО декрементить
	// posts.comments_count в той же транзакции, что и DELETE — симметрично
	// этому Create. Сегодня это не нужно: удалить можно только весь пост
	// (comments.post_id ... ON DELETE CASCADE в миграции уносит счётчик
	// вместе с постом), эндпоинта удаления одного комментария ещё нет.
	Create(ctx context.Context, comment *Comment) error
	// ListByPost отдаёт страницу ПО ВОЗРАСТАНИЮ created_at: обсуждение
	// читается сверху вниз, в отличие от ленты. Про «есть ли ещё» решает
	// вызывающий, запросив limit+1.
	ListByPost(ctx context.Context, postID, viewerID uuid.UUID, cursor *FeedCursor, limit int) ([]Comment, error)
	Like(ctx context.Context, commentID, userID uuid.UUID) (*Comment, error)
	Unlike(ctx context.Context, commentID, userID uuid.UUID) (*Comment, error)
}
