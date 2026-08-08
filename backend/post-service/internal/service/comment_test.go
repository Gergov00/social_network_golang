package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"post-service/internal/domain"

	"github.com/google/uuid"
)

// fakeCommentRepo запоминает аргументы: половина проверок тут — про то, что
// сервис ПОПРОСИЛ у репозитория, а не про то, что тот вернул.
type fakeCommentRepo struct {
	created  *domain.Comment
	list     []domain.Comment
	gotPost  uuid.UUID
	gotView  uuid.UUID
	gotLimit int
	err      error
}

func (f *fakeCommentRepo) Create(ctx context.Context, comment *domain.Comment) error {
	f.created = comment
	return f.err
}

func (f *fakeCommentRepo) ListByPost(ctx context.Context, postID, viewerID uuid.UUID, cursor *domain.FeedCursor, limit int) ([]domain.Comment, error) {
	f.gotPost = postID
	f.gotView = viewerID
	f.gotLimit = limit
	if f.err != nil {
		return nil, f.err
	}
	if len(f.list) > limit {
		return f.list[:limit], nil
	}
	return f.list, nil
}

func (f *fakeCommentRepo) Like(ctx context.Context, commentID, userID uuid.UUID) (*domain.Comment, error) {
	return &domain.Comment{ID: commentID, Liked: true, LikesCount: 1}, f.err
}

func (f *fakeCommentRepo) Unlike(ctx context.Context, commentID, userID uuid.UUID) (*domain.Comment, error) {
	return &domain.Comment{ID: commentID}, f.err
}

func makeComments(n int) []domain.Comment {
	out := make([]domain.Comment, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, domain.Comment{ID: uuid.New()})
	}
	return out
}

func TestCreateCommentSetsAuthorPostAndTimestamps(t *testing.T) {
	repo := &fakeCommentRepo{}
	svc := NewCommentService(repo)
	post, author := uuid.New(), uuid.New()

	comment, err := svc.Create(context.Background(), post, author, "  мысль  ")
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	if comment.AuthorID != author {
		t.Errorf("AuthorID = %v, want %v (автор только из аргумента)", comment.AuthorID, author)
	}
	if comment.PostID != post {
		t.Errorf("PostID = %v, want %v", comment.PostID, post)
	}
	if comment.ID == uuid.Nil {
		t.Error("ID не сгенерирован")
	}
	if comment.Text != "мысль" {
		t.Errorf("Text = %q, ожидался обрезанный по краям", comment.Text)
	}
	if comment.CreatedAt.IsZero() || comment.UpdatedAt.IsZero() {
		t.Error("CreatedAt/UpdatedAt не проставлены")
	}
}

func TestCreateCommentRejectsEmptyText(t *testing.T) {
	svc := NewCommentService(&fakeCommentRepo{})

	for _, text := range []string{"", "   ", "\n\t "} {
		if _, err := svc.Create(context.Background(), uuid.New(), uuid.New(), text); !errors.Is(err, domain.ErrEmptyText) {
			t.Errorf("Create(%q) err = %v, want ErrEmptyText", text, err)
		}
	}
}

// Лимит в СИМВОЛАХ, не байтах: 1500 кириллических символов — это 3000 байт,
// и байтовый лимит зарезал бы русский комментарий вдвое раньше английского.
func TestCreateCommentCountsRunesNotBytes(t *testing.T) {
	svc := NewCommentService(&fakeCommentRepo{})

	ok := strings.Repeat("я", MaxCommentRunes)
	if _, err := svc.Create(context.Background(), uuid.New(), uuid.New(), ok); err != nil {
		t.Errorf("Create(%d рун) err = %v, ожидался успех", MaxCommentRunes, err)
	}

	tooLong := strings.Repeat("я", MaxCommentRunes+1)
	if _, err := svc.Create(context.Background(), uuid.New(), uuid.New(), tooLong); !errors.Is(err, domain.ErrTextTooLong) {
		t.Errorf("Create(%d рун) err = %v, want ErrTextTooLong", MaxCommentRunes+1, err)
	}
}

func TestListCommentsRequestsOneExtraAndTrims(t *testing.T) {
	repo := &fakeCommentRepo{list: makeComments(21)}
	svc := NewCommentService(repo)

	comments, hasMore, err := svc.List(context.Background(), uuid.New(), uuid.New(), nil, 20)
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if repo.gotLimit != 21 {
		t.Errorf("репозиторий получил limit = %d, want 21", repo.gotLimit)
	}
	if len(comments) != 20 || !hasMore {
		t.Errorf("List() = %d комментариев, hasMore = %v; want 20, true", len(comments), hasMore)
	}
}

func TestListCommentsPassesPostAndViewerSeparately(t *testing.T) {
	repo := &fakeCommentRepo{}
	svc := NewCommentService(repo)
	post, viewer := uuid.New(), uuid.New()

	if _, _, err := svc.List(context.Background(), post, viewer, nil, 10); err != nil {
		t.Fatalf("List(): %v", err)
	}
	if repo.gotPost != post {
		t.Errorf("postID = %v, want %v", repo.gotPost, post)
	}
	if repo.gotView != viewer {
		t.Errorf("viewerID = %v, want %v", repo.gotView, viewer)
	}
}
