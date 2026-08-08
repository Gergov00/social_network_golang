package grpc

import (
	"context"
	"net"
	"testing"

	postv1 "ahishka/pkg/gen/ahishka/post/v1"
	"post-service/internal/domain"
	"post-service/internal/service"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeGRPCCommentRepo — минимальный domain.CommentRepository, запоминающий,
// что именно попросил транспорт. Имя с префиксом, потому что в пакете
// service уже есть свой fakeCommentRepo — это разные пакеты, но одинаковые
// имена в соседних файлах путают при чтении.
type fakeGRPCCommentRepo struct {
	created *domain.Comment
	list    []domain.Comment
	gotPost uuid.UUID
	gotView uuid.UUID
	err     error
}

func (f *fakeGRPCCommentRepo) Create(ctx context.Context, c *domain.Comment) error {
	f.created = c
	return f.err
}

func (f *fakeGRPCCommentRepo) ListByPost(ctx context.Context, postID, viewerID uuid.UUID, cursor *domain.FeedCursor, limit int) ([]domain.Comment, error) {
	f.gotPost = postID
	f.gotView = viewerID
	return f.list, f.err
}

func (f *fakeGRPCCommentRepo) Like(ctx context.Context, commentID, userID uuid.UUID) (*domain.Comment, error) {
	return &domain.Comment{ID: commentID, Liked: true, LikesCount: 1}, f.err
}

func (f *fakeGRPCCommentRepo) Unlike(ctx context.Context, commentID, userID uuid.UUID) (*domain.Comment, error) {
	return &domain.Comment{ID: commentID}, f.err
}

// newTestCommentClient — по образцу newTestClient из post_test.go: тот же
// bufconn, тот же диалер, чистка через t.Cleanup (без возврата cleanup), но
// регистрирует только CommentServiceServer — PostService этому файлу не нужен.
func newTestCommentClient(t *testing.T, repo domain.CommentRepository) postv1.CommentServiceClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	postv1.RegisterCommentServiceServer(srv, NewCommentHandler(service.NewCommentService(repo)))

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
	return postv1.NewCommentServiceClient(conn)
}

// Автор комментария берётся ТОЛЬКО из metadata. Если бы он приезжал полем
// сообщения, любой клиент gateway комментировал бы от чужого имени.
func TestCreateCommentTakesAuthorFromMetadataOnly(t *testing.T) {
	repo := &fakeGRPCCommentRepo{}
	client := newTestCommentClient(t, repo)

	caller := uuid.New()
	post := uuid.New()

	if _, err := client.CreateComment(callerCtx(caller), &postv1.CreateCommentRequest{
		PostId: post.String(),
		Text:   "реплика",
	}); err != nil {
		t.Fatalf("CreateComment(): %v", err)
	}

	if repo.created == nil {
		t.Fatal("комментарий не доехал до репозитория")
	}
	if repo.created.AuthorID != caller {
		t.Errorf("AuthorID = %v, want %v (из metadata)", repo.created.AuthorID, caller)
	}
	if repo.created.PostID != post {
		t.Errorf("PostID = %v, want %v", repo.created.PostID, post)
	}
}

func TestCommentRPCsRequireCallerMetadata(t *testing.T) {
	client := newTestCommentClient(t, &fakeGRPCCommentRepo{})

	id := uuid.NewString()
	calls := map[string]func() error{
		"ListComments": func() error {
			_, err := client.ListComments(context.Background(), &postv1.ListCommentsRequest{PostId: id})
			return err
		},
		"CreateComment": func() error {
			_, err := client.CreateComment(context.Background(), &postv1.CreateCommentRequest{PostId: id, Text: "x"})
			return err
		},
		"LikeComment": func() error {
			_, err := client.LikeComment(context.Background(), &postv1.LikeCommentRequest{CommentId: id})
			return err
		},
		"UnlikeComment": func() error {
			_, err := client.UnlikeComment(context.Background(), &postv1.UnlikeCommentRequest{CommentId: id})
			return err
		},
	}

	// Все четыре, а не один «представительный»: callerID зовётся в каждом
	// хендлере отдельно, и пропавший вызов в одном компилируется и проходит
	// все прочие тесты.
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if code := status.Code(call()); code != codes.Unauthenticated {
				t.Errorf("код = %v, want Unauthenticated", code)
			}
		})
	}
}

func TestListCommentsSplitsPostAndViewer(t *testing.T) {
	repo := &fakeGRPCCommentRepo{}
	client := newTestCommentClient(t, repo)

	post, viewer := uuid.New(), uuid.New()

	if _, err := client.ListComments(callerCtx(viewer), &postv1.ListCommentsRequest{PostId: post.String()}); err != nil {
		t.Fatalf("ListComments(): %v", err)
	}
	if repo.gotPost != post {
		t.Errorf("postID = %v, want %v (из поля запроса)", repo.gotPost, post)
	}
	if repo.gotView != viewer {
		t.Errorf("viewerID = %v, want %v (из metadata)", repo.gotView, viewer)
	}
}

func TestCommentToNonexistentPostIsNotFound(t *testing.T) {
	client := newTestCommentClient(t, &fakeGRPCCommentRepo{err: domain.ErrNotFound})

	_, err := client.CreateComment(callerCtx(uuid.New()), &postv1.CreateCommentRequest{PostId: uuid.NewString(), Text: "x"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("код = %v, want NotFound", status.Code(err))
	}
}

func TestCreateCommentRejectsBadPostID(t *testing.T) {
	client := newTestCommentClient(t, &fakeGRPCCommentRepo{})

	_, err := client.CreateComment(callerCtx(uuid.New()), &postv1.CreateCommentRequest{PostId: "не-uuid", Text: "x"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("код = %v, want InvalidArgument", status.Code(err))
	}
}
