package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	postv1 "ahishka/pkg/gen/ahishka/post/v1"
	userv1 "ahishka/pkg/gen/ahishka/user/v1"
	"ahishka/pkg/grpcmeta"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fakeCommentClient — как fakePostClient: запоминает и запрос, и ИСХОДЯЩУЮ
// metadata. Без metadata проверка «зритель уехал из токена» не стерегла бы
// ничего.
type fakeCommentClient struct {
	list    *postv1.ListCommentsResponse
	comment *postv1.Comment
	err     error
	lastMD  metadata.MD
	lastReq any
}

func (f *fakeCommentClient) record(ctx context.Context, req any) {
	md, _ := metadata.FromOutgoingContext(ctx)
	f.lastMD = md
	f.lastReq = req
}

func (f *fakeCommentClient) ListComments(ctx context.Context, in *postv1.ListCommentsRequest, opts ...grpc.CallOption) (*postv1.ListCommentsResponse, error) {
	f.record(ctx, in)
	if f.err != nil {
		return nil, f.err
	}
	if f.list == nil {
		return &postv1.ListCommentsResponse{}, nil
	}
	return f.list, nil
}

func (f *fakeCommentClient) CreateComment(ctx context.Context, in *postv1.CreateCommentRequest, opts ...grpc.CallOption) (*postv1.CreateCommentResponse, error) {
	f.record(ctx, in)
	if f.err != nil {
		return nil, f.err
	}
	return &postv1.CreateCommentResponse{Comment: f.comment}, nil
}

func (f *fakeCommentClient) LikeComment(ctx context.Context, in *postv1.LikeCommentRequest, opts ...grpc.CallOption) (*postv1.LikeCommentResponse, error) {
	f.record(ctx, in)
	if f.err != nil {
		return nil, f.err
	}
	return &postv1.LikeCommentResponse{Comment: f.comment}, nil
}

func (f *fakeCommentClient) UnlikeComment(ctx context.Context, in *postv1.UnlikeCommentRequest, opts ...grpc.CallOption) (*postv1.UnlikeCommentResponse, error) {
	f.record(ctx, in)
	if f.err != nil {
		return nil, f.err
	}
	return &postv1.UnlikeCommentResponse{Comment: f.comment}, nil
}

func testProtoComment(author uuid.UUID) *postv1.Comment {
	return &postv1.Comment{
		Id:         uuid.NewString(),
		PostId:     uuid.NewString(),
		AuthorId:   author.String(),
		Text:       "реплика",
		LikesCount: 2,
		CreatedAt:  timestamppb.New(time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)),
		UpdatedAt:  timestamppb.New(time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)),
	}
}

// Пустой список обязан уехать как [], а не null: фронт делает comments.map().
func TestCommentsEmptyIsArrayNotNull(t *testing.T) {
	handler := NewCommentHandler(&fakeCommentClient{}, &batchUserClient{})

	req := authed(t, http.MethodGet, "/posts/x/comments", "", uuid.New())
	req.SetPathValue("id", uuid.NewString())
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"comments":[]`) {
		t.Errorf("тело = %s, ожидался пустой массив comments", rec.Body.String())
	}
}

// Автор комментария — только из токена. Всё, что клиент положил в тело сверх
// text, обязано быть отброшено.
func TestCreateCommentSendsOnlyTextAndCaller(t *testing.T) {
	caller, other := uuid.New(), uuid.New()
	comments := &fakeCommentClient{comment: testProtoComment(caller)}
	handler := NewCommentHandler(comments, &batchUserClient{})

	postID := uuid.NewString()
	body := `{"text":"моя реплика","authorId":"` + other.String() + `","postId":"` + uuid.NewString() + `"}`
	req := authed(t, http.MethodPost, "/posts/"+postID+"/comments", body, caller)
	req.SetPathValue("id", postID)
	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	in, ok := comments.lastReq.(*postv1.CreateCommentRequest)
	if !ok {
		t.Fatalf("в post-service уехал %T", comments.lastReq)
	}
	if in.GetText() != "моя реплика" {
		t.Errorf("text = %q", in.GetText())
	}
	// post_id — из ПУТИ, а не из тела: тело клиент контролирует целиком.
	if in.GetPostId() != postID {
		t.Errorf("post_id = %q, want %q (из пути)", in.GetPostId(), postID)
	}
	if md := comments.lastMD.Get(grpcmeta.UserID); len(md) == 0 || md[0] != caller.String() {
		t.Errorf("metadata = %v, want %v (только из токена)", md, caller)
	}
}

func TestCommentsHydrateAuthorsWithOneDedupedCall(t *testing.T) {
	author := uuid.New()
	users := &batchUserClient{batchUsers: []*userv1.User{testUser(author)}}
	comments := &fakeCommentClient{list: &postv1.ListCommentsResponse{
		Comments:   []*postv1.Comment{testProtoComment(author), testProtoComment(author)},
		NextCursor: "курсор",
	}}
	handler := NewCommentHandler(comments, users)

	req := authed(t, http.MethodGet, "/posts/x/comments", "", uuid.New())
	req.SetPathValue("id", uuid.NewString())
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if users.batchCalls != 1 {
		t.Errorf("BatchGetUsers вызван %d раз, want 1", users.batchCalls)
	}
	if len(users.batchIDs) != 1 {
		t.Errorf("в пачке %d id, want 1 (дубли автора обязаны схлопнуться)", len(users.batchIDs))
	}

	var resp struct {
		Comments []struct {
			Author *struct {
				Handle string `json:"handle"`
			} `json:"author"`
		} `json:"comments"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("разбор ответа: %v", err)
	}
	for i, c := range resp.Comments {
		if c.Author == nil || c.Author.Handle == "" {
			t.Errorf("комментарий %d: автор не подставлен", i)
		}
	}
	if resp.NextCursor != "курсор" {
		t.Errorf("nextCursor = %q", resp.NextCursor)
	}
}

// Падение user-service не должно ронять ветку обсуждения: тексты важнее имён.
func TestCommentsDegradeWhenUserServiceFails(t *testing.T) {
	users := &batchUserClient{batchErr: status.Error(codes.Unavailable, "нет связи")}
	comments := &fakeCommentClient{list: &postv1.ListCommentsResponse{
		Comments: []*postv1.Comment{testProtoComment(uuid.New())},
	}}
	handler := NewCommentHandler(comments, users)

	req := authed(t, http.MethodGet, "/posts/x/comments", "", uuid.New())
	req.SetPathValue("id", uuid.NewString())
	rec := httptest.NewRecorder()
	handler.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (комментарии отдаются и без профилей)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"author":null`) {
		t.Errorf("тело = %s, ожидался author: null", rec.Body.String())
	}
}

func TestCommentLikeAndUnlikeSendIDAndCaller(t *testing.T) {
	caller := uuid.New()
	commentID := uuid.NewString()

	t.Run("like", func(t *testing.T) {
		comments := &fakeCommentClient{comment: testProtoComment(uuid.New())}
		handler := NewCommentHandler(comments, &batchUserClient{})

		req := authed(t, http.MethodPost, "/comments/"+commentID+"/like", "", caller)
		req.SetPathValue("id", commentID)
		rec := httptest.NewRecorder()
		handler.Like(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		in, ok := comments.lastReq.(*postv1.LikeCommentRequest)
		if !ok || in.GetCommentId() != commentID {
			t.Errorf("LikeCommentRequest = %+v, want comment_id %s", comments.lastReq, commentID)
		}
		if md := comments.lastMD.Get(grpcmeta.UserID); len(md) == 0 || md[0] != caller.String() {
			t.Errorf("metadata = %v, want %v", md, caller)
		}
	})

	t.Run("unlike", func(t *testing.T) {
		comments := &fakeCommentClient{comment: testProtoComment(uuid.New())}
		handler := NewCommentHandler(comments, &batchUserClient{})

		req := authed(t, http.MethodDelete, "/comments/"+commentID+"/like", "", caller)
		req.SetPathValue("id", commentID)
		rec := httptest.NewRecorder()
		handler.Unlike(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		in, ok := comments.lastReq.(*postv1.UnlikeCommentRequest)
		if !ok || in.GetCommentId() != commentID {
			t.Errorf("UnlikeCommentRequest = %+v, want comment_id %s", comments.lastReq, commentID)
		}
		if md := comments.lastMD.Get(grpcmeta.UserID); len(md) == 0 || md[0] != caller.String() {
			t.Errorf("metadata = %v, want %v", md, caller)
		}
	})
}

func TestCreateCommentRejectsBadBody(t *testing.T) {
	handler := NewCommentHandler(&fakeCommentClient{}, &batchUserClient{})

	req := authed(t, http.MethodPost, "/posts/x/comments", "не json", uuid.New())
	req.SetPathValue("id", uuid.NewString())
	rec := httptest.NewRecorder()
	handler.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestGarbagePathIDMapsToBadRequest — задел из ревью задачи 5: gateway сам
// не парсит {id} из пути, он уходит в post-service как есть, а тот отвечает
// InvalidArgument на невалидный uuid (там свой uuid.Parse на каждый из
// четырёх RPC). Здесь проверяем ТОЛЬКО перевод кода на границе gateway:
// InvalidArgument от апстрима обязан стать 4xx, а не 500 и не паникой, ни в
// одном из четырёх хендлеров.
func TestGarbagePathIDMapsToBadRequest(t *testing.T) {
	badArg := status.Error(codes.InvalidArgument, "invalid id")

	cases := []struct {
		name string
		call func(h *CommentHandler, rec *httptest.ResponseRecorder, req *http.Request)
	}{
		{"List", func(h *CommentHandler, rec *httptest.ResponseRecorder, req *http.Request) { h.List(rec, req) }},
		{"Create", func(h *CommentHandler, rec *httptest.ResponseRecorder, req *http.Request) { h.Create(rec, req) }},
		{"Like", func(h *CommentHandler, rec *httptest.ResponseRecorder, req *http.Request) { h.Like(rec, req) }},
		{"Unlike", func(h *CommentHandler, rec *httptest.ResponseRecorder, req *http.Request) { h.Unlike(rec, req) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewCommentHandler(&fakeCommentClient{err: badArg}, &batchUserClient{})

			req := authed(t, http.MethodPost, "/x/garbage-not-a-uuid/comments", `{"text":"x"}`, uuid.New())
			req.SetPathValue("id", "garbage-not-a-uuid")
			rec := httptest.NewRecorder()

			tc.call(handler, rec, req)

			if rec.Code < 400 || rec.Code >= 500 {
				t.Errorf("status = %d, want 4xx (InvalidArgument from upstream must not become 5xx)", rec.Code)
			}
		})
	}
}
