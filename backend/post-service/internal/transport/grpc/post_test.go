package grpc

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	postv1 "ahishka/pkg/gen/ahishka/post/v1"
	"ahishka/pkg/grpcmeta"
	"post-service/internal/domain"
	"post-service/internal/service"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

// fakeRepo реализует domain.PostRepository без БД и запоминает аргументы:
// проверки «автор взят из metadata» и «зритель доехал до репозитория»
// иначе ничего не стерегут.
type fakeRepo struct {
	created  *domain.Post
	post     *domain.Post
	feed     []domain.Post
	gotView  uuid.UUID
	gotLiker uuid.UUID
	err      error
}

func (f *fakeRepo) Create(ctx context.Context, post *domain.Post) error {
	f.created = post
	return f.err
}

func (f *fakeRepo) GetByID(ctx context.Context, id, viewerID uuid.UUID) (*domain.Post, error) {
	f.gotView = viewerID
	if f.err != nil {
		return nil, f.err
	}
	return f.post, nil
}

func (f *fakeRepo) ListFeed(ctx context.Context, viewerID uuid.UUID, cursor *domain.FeedCursor, limit int) ([]domain.Post, error) {
	f.gotView = viewerID
	if f.err != nil {
		return nil, f.err
	}
	if len(f.feed) > limit {
		return f.feed[:limit], nil
	}
	return f.feed, nil
}

func (f *fakeRepo) Like(ctx context.Context, postID, userID uuid.UUID) (*domain.Post, error) {
	f.gotLiker = userID
	if f.err != nil {
		return nil, f.err
	}
	return f.post, nil
}

func (f *fakeRepo) Unlike(ctx context.Context, postID, userID uuid.UUID) (*domain.Post, error) {
	f.gotLiker = userID
	if f.err != nil {
		return nil, f.err
	}
	return f.post, nil
}

// newTestClient поднимает настоящий gRPC-сервер поверх bufconn — тесты
// ходят через реальный кодек и реальную metadata, а не зовут метод напрямую.
func newTestClient(t *testing.T, repo domain.PostRepository) postv1.PostServiceClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	srv := NewServer(NewPostHandler(service.NewPostService(repo)))

	go func() {
		if err := srv.Serve(lis); err != nil {
			return // после Stop() Serve возвращает ошибку — это нормально
		}
	}()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}

	t.Cleanup(func() {
		conn.Close()
		srv.Stop()
		lis.Close()
	})
	return postv1.NewPostServiceClient(conn)
}

// callerCtx — контекст «пришёл авторизованный вызывающий», ровно такой,
// какой делает gateway.
func callerCtx(id uuid.UUID) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), grpcmeta.UserID, id.String())
}

func testPost(author uuid.UUID) *domain.Post {
	return &domain.Post{
		ID:         uuid.New(),
		AuthorID:   author,
		Text:       "текст",
		LikesCount: 3,
		Liked:      true,
		CreatedAt:  time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC),
	}
}

// TestContractHasNoCallerFields — страж контракта. Автор и зритель обязаны
// приходить ТОЛЬКО из metadata; появление такого поля в запросе означает,
// что любой клиент gateway сможет писать и лайкать от чужого имени.
// Проверяется по дескриптору, а не по коду хендлера: поле, добавленное в
// .proto, ловится здесь даже раньше, чем им кто-то воспользуется.
func TestContractHasNoCallerFields(t *testing.T) {
	messages := []proto.Message{
		&postv1.CreatePostRequest{},
		&postv1.GetFeedRequest{},
		&postv1.LikePostRequest{},
		&postv1.UnlikePostRequest{},
	}

	forbidden := []string{"author", "user", "viewer", "caller", "actor"}

	for _, msg := range messages {
		desc := msg.ProtoReflect().Descriptor()
		fields := desc.Fields()

		for i := 0; i < fields.Len(); i++ {
			name := string(fields.Get(i).Name())
			for _, bad := range forbidden {
				if strings.Contains(name, bad) {
					t.Errorf("%s содержит поле %q: личность вызывающего должна приходить только в metadata %q",
						desc.Name(), name, grpcmeta.UserID)
				}
			}
		}
	}
}

// TestAllRPCsRequireCallerMetadata покрывает ВСЕ пять методов, а не один:
// защита, забытая в одном хендлере, — это дыра ровно того же размера.
func TestAllRPCsRequireCallerMetadata(t *testing.T) {
	client := newTestClient(t, &fakeRepo{post: testPost(uuid.New())})
	id := uuid.New().String()

	calls := map[string]func(context.Context) error{
		"CreatePost": func(ctx context.Context) error {
			_, err := client.CreatePost(ctx, &postv1.CreatePostRequest{Text: "текст"})
			return err
		},
		"GetPost": func(ctx context.Context) error {
			_, err := client.GetPost(ctx, &postv1.GetPostRequest{Id: id})
			return err
		},
		"GetFeed": func(ctx context.Context) error {
			_, err := client.GetFeed(ctx, &postv1.GetFeedRequest{})
			return err
		},
		"LikePost": func(ctx context.Context) error {
			_, err := client.LikePost(ctx, &postv1.LikePostRequest{PostId: id})
			return err
		},
		"UnlikePost": func(ctx context.Context) error {
			_, err := client.UnlikePost(ctx, &postv1.UnlikePostRequest{PostId: id})
			return err
		},
	}

	for name, call := range calls {
		t.Run(name+"/без metadata", func(t *testing.T) {
			if got := status.Code(call(context.Background())); got != codes.Unauthenticated {
				t.Errorf("code = %v, want Unauthenticated", got)
			}
		})
		t.Run(name+"/битый id", func(t *testing.T) {
			// ASCII, не "не-uuid": gRPC-транспорт валидирует значения
			// metadata на печатные ASCII-символы (internal/metadata.ValidatePair)
			// и рвёт RPC на стороне клиента с codes.Internal ДО отправки,
			// если внутри кириллица — тест тогда проверял бы не callerID,
			// а транспортную валидацию. Строка обязана быть непарсящимся
			// uuid.Parse, но состоять только из ASCII.
			ctx := metadata.AppendToOutgoingContext(context.Background(), grpcmeta.UserID, "not-a-uuid")
			if got := status.Code(call(ctx)); got != codes.Unauthenticated {
				t.Errorf("code = %v, want Unauthenticated", got)
			}
		})
	}
}

func TestCreatePostTakesAuthorFromMetadata(t *testing.T) {
	repo := &fakeRepo{}
	client := newTestClient(t, repo)
	caller := uuid.New()

	resp, err := client.CreatePost(callerCtx(caller), &postv1.CreatePostRequest{Text: "новый пост"})
	if err != nil {
		t.Fatalf("CreatePost(): %v", err)
	}
	if repo.created == nil {
		t.Fatal("репозиторий не получил пост")
	}
	if repo.created.AuthorID != caller {
		t.Errorf("сохранённый AuthorID = %v, want %v (id вызывающего из metadata)", repo.created.AuthorID, caller)
	}
	if resp.GetPost().GetAuthorId() != caller.String() {
		t.Errorf("в ответе author_id = %q, want %q", resp.GetPost().GetAuthorId(), caller)
	}
}

func TestCreatePostRejectsEmptyText(t *testing.T) {
	client := newTestClient(t, &fakeRepo{})

	_, err := client.CreatePost(callerCtx(uuid.New()), &postv1.CreatePostRequest{Text: "   "})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestGetPostNotFound(t *testing.T) {
	client := newTestClient(t, &fakeRepo{err: domain.ErrNotFound})

	_, err := client.GetPost(callerCtx(uuid.New()), &postv1.GetPostRequest{Id: uuid.New().String()})
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("code = %v, want NotFound", got)
	}
}

func TestGetPostRejectsNonUUID(t *testing.T) {
	client := newTestClient(t, &fakeRepo{post: testPost(uuid.New())})

	_, err := client.GetPost(callerCtx(uuid.New()), &postv1.GetPostRequest{Id: "не-uuid"})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestGetPostPassesViewerFromMetadata(t *testing.T) {
	repo := &fakeRepo{post: testPost(uuid.New())}
	client := newTestClient(t, repo)
	viewer := uuid.New()

	if _, err := client.GetPost(callerCtx(viewer), &postv1.GetPostRequest{Id: uuid.New().String()}); err != nil {
		t.Fatalf("GetPost(): %v", err)
	}
	if repo.gotView != viewer {
		t.Errorf("репозиторий получил viewerID = %v, want %v (liked считается на зрителя)", repo.gotView, viewer)
	}
}

func TestGetFeedRejectsBadCursor(t *testing.T) {
	client := newTestClient(t, &fakeRepo{})

	_, err := client.GetFeed(callerCtx(uuid.New()), &postv1.GetFeedRequest{Cursor: "мусор!!!"})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestGetFeedNextCursorOnlyWhenMore(t *testing.T) {
	// 21 пост при лимите 20: сервис попросит 21, вернёт 20 и скажет hasMore.
	author := uuid.New()
	feed := make([]domain.Post, 0, 21)
	base := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 21; i++ {
		feed = append(feed, domain.Post{
			ID:        uuid.New(),
			AuthorID:  author,
			Text:      "текст",
			CreatedAt: base.Add(-time.Duration(i) * time.Minute),
			UpdatedAt: base,
		})
	}
	client := newTestClient(t, &fakeRepo{feed: feed})

	resp, err := client.GetFeed(callerCtx(uuid.New()), &postv1.GetFeedRequest{Limit: 20})
	if err != nil {
		t.Fatalf("GetFeed(): %v", err)
	}
	if len(resp.GetPosts()) != 20 {
		t.Fatalf("вернулось %d постов, want 20", len(resp.GetPosts()))
	}
	if resp.GetNextCursor() == "" {
		t.Fatal("next_cursor пустой, хотя страница не последняя")
	}

	// Курсор обязан указывать на ПОСЛЕДНИЙ отданный пост, иначе следующая
	// страница либо потеряет запись, либо повторит её.
	cursor, err := domain.ParseCursor(resp.GetNextCursor())
	if err != nil {
		t.Fatalf("ParseCursor(next_cursor): %v", err)
	}
	last := resp.GetPosts()[len(resp.GetPosts())-1]
	if cursor.ID.String() != last.GetId() {
		t.Errorf("курсор указывает на %v, want %v (последний пост страницы)", cursor.ID, last.GetId())
	}
}

func TestGetFeedLastPageHasNoCursor(t *testing.T) {
	client := newTestClient(t, &fakeRepo{feed: []domain.Post{*testPost(uuid.New())}})

	resp, err := client.GetFeed(callerCtx(uuid.New()), &postv1.GetFeedRequest{Limit: 20})
	if err != nil {
		t.Fatalf("GetFeed(): %v", err)
	}
	if resp.GetNextCursor() != "" {
		t.Errorf("next_cursor = %q, ожидался пустой на последней странице", resp.GetNextCursor())
	}
}

func TestLikePassesCallerAsLiker(t *testing.T) {
	repo := &fakeRepo{post: testPost(uuid.New())}
	client := newTestClient(t, repo)
	caller := uuid.New()

	if _, err := client.LikePost(callerCtx(caller), &postv1.LikePostRequest{PostId: uuid.New().String()}); err != nil {
		t.Fatalf("LikePost(): %v", err)
	}
	if repo.gotLiker != caller {
		t.Errorf("лайк записан от %v, want %v (только вызывающий из metadata)", repo.gotLiker, caller)
	}
}

func TestUnlikePassesCallerAsLiker(t *testing.T) {
	repo := &fakeRepo{post: testPost(uuid.New())}
	client := newTestClient(t, repo)
	caller := uuid.New()

	if _, err := client.UnlikePost(callerCtx(caller), &postv1.UnlikePostRequest{PostId: uuid.New().String()}); err != nil {
		t.Fatalf("UnlikePost(): %v", err)
	}
	if repo.gotLiker != caller {
		t.Errorf("снятие лайка записано от %v, want %v", repo.gotLiker, caller)
	}
}

func TestInternalErrorDoesNotLeakDBText(t *testing.T) {
	secret := `pq: relation "posts" does not exist (host=postgres-post user=post)`
	client := newTestClient(t, &fakeRepo{err: errors.New(secret)})

	_, err := client.GetPost(callerCtx(uuid.New()), &postv1.GetPostRequest{Id: uuid.New().String()})
	if got := status.Code(err); got != codes.Internal {
		t.Fatalf("code = %v, want Internal", got)
	}
	if strings.Contains(status.Convert(err).Message(), "posts") ||
		strings.Contains(status.Convert(err).Message(), "postgres") {
		t.Errorf("наружу утёк текст ошибки БД: %q", status.Convert(err).Message())
	}
}
