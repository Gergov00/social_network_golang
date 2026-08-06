package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"post-service/internal/domain"

	"github.com/google/uuid"
)

const (
	// MaxTextRunes — предел в СИМВОЛАХ, не байтах: 5000 кириллических
	// символов это 10000 байт, и байтовый лимит резал бы русский текст
	// вдвое раньше английского.
	MaxTextRunes = 5000

	DefaultFeedLimit = 20
	MaxFeedLimit     = 50
)

type PostService struct {
	repo domain.PostRepository
}

func NewPostService(repo domain.PostRepository) *PostService {
	return &PostService{
		repo: repo,
	}
}

// Create заводит пост. authorID приходит аргументом и всегда из
// gRPC-metadata (см. transport/grpc/metadata.go) — сервис его не выдумывает
// и не берёт из тела запроса.
func (s *PostService) Create(ctx context.Context, authorID uuid.UUID, text string) (*domain.Post, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, domain.ErrEmptyText
	}
	if utf8.RuneCountInString(text) > MaxTextRunes {
		return nil, domain.ErrTextTooLong
	}

	now := time.Now()
	post := &domain.Post{
		ID:        uuid.New(),
		AuthorID:  authorID,
		Text:      text,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.Create(ctx, post); err != nil {
		return nil, err
	}
	return post, nil
}

func (s *PostService) GetByID(ctx context.Context, id, viewerID uuid.UUID) (*domain.Post, error) {
	return s.repo.GetByID(ctx, id, viewerID)
}

// Feed отдаёт страницу и признак «есть ещё». Чтобы узнать про «ещё» без
// второго запроса (COUNT по всей ленте), просим limit+1 записей и лишнюю
// отбрасываем.
func (s *PostService) Feed(ctx context.Context, viewerID uuid.UUID, cursor *domain.FeedCursor, limit int) ([]domain.Post, bool, error) {
	limit = clampLimit(limit)

	posts, err := s.repo.ListFeed(ctx, viewerID, cursor, limit+1)
	if err != nil {
		return nil, false, err
	}

	hasMore := len(posts) > limit
	if hasMore {
		posts = posts[:limit]
	}
	return posts, hasMore, nil
}

func (s *PostService) Like(ctx context.Context, postID, userID uuid.UUID) (*domain.Post, error) {
	return s.repo.Like(ctx, postID, userID)
}

func (s *PostService) Unlike(ctx context.Context, postID, userID uuid.UUID) (*domain.Post, error) {
	return s.repo.Unlike(ctx, postID, userID)
}

// clampLimit защищает БД от `?limit=1000000` из адресной строки.
func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultFeedLimit
	case limit > MaxFeedLimit:
		return MaxFeedLimit
	default:
		return limit
	}
}
