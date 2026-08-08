package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"post-service/internal/domain"

	"github.com/google/uuid"
)

// fakeRepo реализует domain.PostRepository без БД. Он не только отдаёт
// заготовленное, но и ЗАПОМИНАЕТ аргументы: половина проверок тут — про
// то, что сервис попросил у репозитория, а не про то, что он вернул.
type fakeRepo struct {
	created   *domain.Post
	feed      []domain.Post
	gotLimit  int
	gotView   uuid.UUID
	gotAuthor uuid.UUID
	gotCursor *domain.FeedCursor
	err       error
}

func (f *fakeRepo) Create(ctx context.Context, post *domain.Post) error {
	f.created = post
	return f.err
}

func (f *fakeRepo) GetByID(ctx context.Context, id, viewerID uuid.UUID) (*domain.Post, error) {
	return &domain.Post{ID: id}, f.err
}

func (f *fakeRepo) ListFeed(ctx context.Context, viewerID uuid.UUID, cursor *domain.FeedCursor, limit int) ([]domain.Post, error) {
	f.gotLimit = limit
	f.gotView = viewerID
	if f.err != nil {
		return nil, f.err
	}
	if len(f.feed) > limit {
		return f.feed[:limit], nil
	}
	return f.feed, nil
}

func (f *fakeRepo) ListByAuthor(ctx context.Context, authorID, viewerID uuid.UUID, cursor *domain.FeedCursor, limit int) ([]domain.Post, error) {
	f.gotAuthor = authorID
	f.gotView = viewerID
	f.gotCursor = cursor
	f.gotLimit = limit
	if f.err != nil {
		return nil, f.err
	}
	if len(f.feed) > limit {
		return f.feed[:limit], nil
	}
	return f.feed, nil
}

func (f *fakeRepo) Like(ctx context.Context, postID, userID uuid.UUID) (*domain.Post, error) {
	return &domain.Post{ID: postID, Liked: true, LikesCount: 1}, f.err
}

func (f *fakeRepo) Unlike(ctx context.Context, postID, userID uuid.UUID) (*domain.Post, error) {
	return &domain.Post{ID: postID}, f.err
}

func makeFeed(n int) []domain.Post {
	posts := make([]domain.Post, 0, n)
	for i := 0; i < n; i++ {
		posts = append(posts, domain.Post{ID: uuid.New(), CreatedAt: time.Now()})
	}
	return posts
}

func TestCreateSetsAuthorAndTimestamps(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewPostService(repo)
	author := uuid.New()

	before := time.Now()
	post, err := svc.Create(context.Background(), author, "  привет  ")
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	if post.AuthorID != author {
		t.Errorf("AuthorID = %v, want %v (автор берётся только из аргумента)", post.AuthorID, author)
	}
	if post.ID == uuid.Nil {
		t.Error("ID не сгенерирован")
	}
	if post.Text != "привет" {
		t.Errorf("Text = %q, ожидался обрезанный по краям", post.Text)
	}
	if post.CreatedAt.Before(before) || post.UpdatedAt.Before(before) {
		t.Error("CreatedAt/UpdatedAt не проставлены")
	}
	if repo.created == nil || repo.created.AuthorID != author {
		t.Error("в репозиторий уехала не та сущность")
	}
}

func TestCreateRejectsEmptyText(t *testing.T) {
	svc := NewPostService(&fakeRepo{})

	for _, text := range []string{"", "   ", "\n\t "} {
		if _, err := svc.Create(context.Background(), uuid.New(), text); !errors.Is(err, domain.ErrEmptyText) {
			t.Errorf("Create(%q) err = %v, want ErrEmptyText", text, err)
		}
	}
}

func TestCreateCountsRunesNotBytes(t *testing.T) {
	svc := NewPostService(&fakeRepo{})

	// 3000 кириллических символов — это 6000 байт. Если длина считается в
	// байтах, этот пост будет отвергнут, хотя он вдвое короче лимита.
	ok := strings.Repeat("я", 3000)
	if _, err := svc.Create(context.Background(), uuid.New(), ok); err != nil {
		t.Errorf("Create(3000 рун) err = %v, ожидался успех", err)
	}

	tooLong := strings.Repeat("я", MaxTextRunes+1)
	if _, err := svc.Create(context.Background(), uuid.New(), tooLong); !errors.Is(err, domain.ErrTextTooLong) {
		t.Errorf("Create(%d рун) err = %v, want ErrTextTooLong", MaxTextRunes+1, err)
	}
}

func TestFeedRequestsOneExtraAndTrims(t *testing.T) {
	repo := &fakeRepo{feed: makeFeed(21)}
	svc := NewPostService(repo)

	posts, hasMore, err := svc.Feed(context.Background(), uuid.New(), nil, 20)
	if err != nil {
		t.Fatalf("Feed(): %v", err)
	}
	if repo.gotLimit != 21 {
		t.Errorf("репозиторий получил limit = %d, ожидался 21 (limit+1 — так узнаём про «есть ещё»)", repo.gotLimit)
	}
	if len(posts) != 20 {
		t.Errorf("len(posts) = %d, want 20 (лишняя запись обязана быть отброшена)", len(posts))
	}
	if !hasMore {
		t.Error("hasMore = false, хотя записей пришло больше запрошенного")
	}
}

func TestFeedLastPageHasNoMore(t *testing.T) {
	repo := &fakeRepo{feed: makeFeed(5)}
	svc := NewPostService(repo)

	posts, hasMore, err := svc.Feed(context.Background(), uuid.New(), nil, 20)
	if err != nil {
		t.Fatalf("Feed(): %v", err)
	}
	if len(posts) != 5 || hasMore {
		t.Errorf("Feed() = %d постов, hasMore = %v; want 5, false", len(posts), hasMore)
	}
}

func TestFeedClampsLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int // сколько сервис обязан запросить у репозитория
	}{
		{"ноль → дефолт", 0, DefaultFeedLimit + 1},
		{"отрицательный → дефолт", -7, DefaultFeedLimit + 1},
		{"выше потолка → потолок", 500, MaxFeedLimit + 1},
		{"в пределах — как есть", 7, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc := NewPostService(repo)

			if _, _, err := svc.Feed(context.Background(), uuid.New(), nil, tt.limit); err != nil {
				t.Fatalf("Feed(): %v", err)
			}
			if repo.gotLimit != tt.want {
				t.Errorf("репозиторий получил limit = %d, want %d", repo.gotLimit, tt.want)
			}
		})
	}
}

func TestFeedPassesViewer(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewPostService(repo)
	viewer := uuid.New()

	if _, _, err := svc.Feed(context.Background(), viewer, nil, 10); err != nil {
		t.Fatalf("Feed(): %v", err)
	}
	if repo.gotView != viewer {
		t.Errorf("репозиторий получил viewerID = %v, want %v", repo.gotView, viewer)
	}
}

// Автор и зритель — РАЗНЫЕ параметры, и путать их нельзя: автор говорит,
// чьи посты выбирать, зритель — чей лайк подсвечивать. Один аргумент на
// оба места дал бы «на чужом профиле все посты не лайкнуты».
func TestUserPostsPassesAuthorAndViewerSeparately(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewPostService(repo)
	author, viewer := uuid.New(), uuid.New()

	if _, _, err := svc.UserPosts(context.Background(), author, viewer, nil, 10); err != nil {
		t.Fatalf("UserPosts(): %v", err)
	}
	if repo.gotAuthor != author {
		t.Errorf("репозиторий получил authorID = %v, want %v", repo.gotAuthor, author)
	}
	if repo.gotView != viewer {
		t.Errorf("репозиторий получил viewerID = %v, want %v", repo.gotView, viewer)
	}
}

func TestUserPostsRequestsOneExtraAndTrims(t *testing.T) {
	repo := &fakeRepo{feed: makeFeed(21)}
	svc := NewPostService(repo)

	posts, hasMore, err := svc.UserPosts(context.Background(), uuid.New(), uuid.New(), nil, 20)
	if err != nil {
		t.Fatalf("UserPosts(): %v", err)
	}
	if repo.gotLimit != 21 {
		t.Errorf("репозиторий получил limit = %d, want 21 (limit+1 — так узнаём про «есть ещё»)", repo.gotLimit)
	}
	if len(posts) != 20 || !hasMore {
		t.Errorf("UserPosts() = %d постов, hasMore = %v; want 20, true", len(posts), hasMore)
	}
}

func TestUserPostsClampsLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{"ноль → дефолт", 0, DefaultFeedLimit + 1},
		{"отрицательный → дефолт", -7, DefaultFeedLimit + 1},
		{"выше потолка → потолок", 500, MaxFeedLimit + 1},
		{"в пределах — как есть", 7, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc := NewPostService(repo)

			if _, _, err := svc.UserPosts(context.Background(), uuid.New(), uuid.New(), nil, tt.limit); err != nil {
				t.Fatalf("UserPosts(): %v", err)
			}
			if repo.gotLimit != tt.want {
				t.Errorf("репозиторий получил limit = %d, want %d", repo.gotLimit, tt.want)
			}
		})
	}
}
