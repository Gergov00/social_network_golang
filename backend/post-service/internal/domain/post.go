package domain

import (
	"time"

	"github.com/google/uuid"
)

type Post struct {
	ID       uuid.UUID
	AuthorID uuid.UUID
	Text     string
	// LikesCount денормализован в колонке posts.likes_count: лента читается
	// одним запросом без агрегата. Двигается только вместе с post_likes,
	// в одной транзакции.
	LikesCount int32
	// Liked — проекция на конкретного зрителя, а не свойство поста:
	// заполняется запросами, которые получают viewerID.
	Liked     bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
