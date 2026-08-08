package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"post-service/internal/domain"

	"github.com/google/uuid"
)

// MaxCommentRunes — предел в СИМВОЛАХ, не байтах (та же причина, что у
// MaxTextRunes). Комментарий короче поста намеренно: это реплика, а не пост.
const MaxCommentRunes = 2000

type CommentService struct {
	repo domain.CommentRepository
}

func NewCommentService(repo domain.CommentRepository) *CommentService {
	return &CommentService{
		repo: repo,
	}
}

// Create заводит комментарий. authorID приходит аргументом и всегда из
// gRPC-metadata — сервис его не выдумывает и не берёт из тела запроса.
func (s *CommentService) Create(ctx context.Context, postID, authorID uuid.UUID, text string) (*domain.Comment, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, domain.ErrEmptyText
	}
	if utf8.RuneCountInString(text) > MaxCommentRunes {
		return nil, domain.ErrTextTooLong
	}

	now := time.Now()
	comment := &domain.Comment{
		ID:        uuid.New(),
		PostID:    postID,
		AuthorID:  authorID,
		Text:      text,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.Create(ctx, comment); err != nil {
		return nil, err
	}
	return comment, nil
}

// List отдаёт страницу и признак «есть ещё» — тем же приёмом limit+1, что и
// лента. Клампы общие с лентой (clampLimit): числа для комментариев те же.
func (s *CommentService) List(ctx context.Context, postID, viewerID uuid.UUID, cursor *domain.FeedCursor, limit int) ([]domain.Comment, bool, error) {
	limit = clampLimit(limit)

	comments, err := s.repo.ListByPost(ctx, postID, viewerID, cursor, limit+1)
	if err != nil {
		return nil, false, err
	}

	hasMore := len(comments) > limit
	if hasMore {
		comments = comments[:limit]
	}
	return comments, hasMore, nil
}

func (s *CommentService) Like(ctx context.Context, commentID, userID uuid.UUID) (*domain.Comment, error) {
	return s.repo.Like(ctx, commentID, userID)
}

func (s *CommentService) Unlike(ctx context.Context, commentID, userID uuid.UUID) (*domain.Comment, error) {
	return s.repo.Unlike(ctx, commentID, userID)
}
