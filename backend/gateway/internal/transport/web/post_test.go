package web

import (
	"bytes"
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

// fakePostClient подменяет gRPC-клиента post-service и запоминает и запрос,
// и ИСХОДЯЩУЮ metadata: проверка «id вызывающего уехал в metadata» без неё
// ничего не стерегла бы.
type fakePostClient struct {
	feed    *postv1.GetFeedResponse
	post    *postv1.Post
	err     error
	lastMD  metadata.MD
	lastReq any
}

func (f *fakePostClient) record(ctx context.Context, req any) {
	md, _ := metadata.FromOutgoingContext(ctx)
	f.lastMD = md
	f.lastReq = req
}

func (f *fakePostClient) CreatePost(ctx context.Context, in *postv1.CreatePostRequest, opts ...grpc.CallOption) (*postv1.CreatePostResponse, error) {
	f.record(ctx, in)
	if f.err != nil {
		return nil, f.err
	}
	return &postv1.CreatePostResponse{Post: f.post}, nil
}

func (f *fakePostClient) GetPost(ctx context.Context, in *postv1.GetPostRequest, opts ...grpc.CallOption) (*postv1.GetPostResponse, error) {
	f.record(ctx, in)
	if f.err != nil {
		return nil, f.err
	}
	return &postv1.GetPostResponse{Post: f.post}, nil
}

func (f *fakePostClient) GetFeed(ctx context.Context, in *postv1.GetFeedRequest, opts ...grpc.CallOption) (*postv1.GetFeedResponse, error) {
	f.record(ctx, in)
	if f.err != nil {
		return nil, f.err
	}
	if f.feed == nil {
		return &postv1.GetFeedResponse{}, nil
	}
	return f.feed, nil
}

func (f *fakePostClient) LikePost(ctx context.Context, in *postv1.LikePostRequest, opts ...grpc.CallOption) (*postv1.LikePostResponse, error) {
	f.record(ctx, in)
	if f.err != nil {
		return nil, f.err
	}
	return &postv1.LikePostResponse{Post: f.post}, nil
}

func (f *fakePostClient) UnlikePost(ctx context.Context, in *postv1.UnlikePostRequest, opts ...grpc.CallOption) (*postv1.UnlikePostResponse, error) {
	f.record(ctx, in)
	if f.err != nil {
		return nil, f.err
	}
	return &postv1.UnlikePostResponse{Post: f.post}, nil
}

func (f *fakePostClient) ListUserPosts(ctx context.Context, in *postv1.ListUserPostsRequest, opts ...grpc.CallOption) (*postv1.ListUserPostsResponse, error) {
	f.record(ctx, in)
	if f.err != nil {
		return nil, f.err
	}
	if f.feed == nil {
		return &postv1.ListUserPostsResponse{}, nil
	}
	return &postv1.ListUserPostsResponse{Posts: f.feed.GetPosts(), NextCursor: f.feed.GetNextCursor()}, nil
}

// batchUserClient — расширение fakeUserClient (он объявлен в user_test.go)
// на пакетный метод: считает вызовы и запоминает переданные id.
type batchUserClient struct {
	fakeUserClient
	batchUsers []*userv1.User
	batchErr   error
	batchCalls int
	batchIDs   []string
}

func (b *batchUserClient) BatchGetUsers(ctx context.Context, in *userv1.BatchGetUsersRequest, opts ...grpc.CallOption) (*userv1.BatchGetUsersResponse, error) {
	b.batchCalls++
	b.batchIDs = in.GetIds()
	if b.batchErr != nil {
		return nil, b.batchErr
	}
	return &userv1.BatchGetUsersResponse{Users: b.batchUsers}, nil
}

func testProtoPost(author uuid.UUID) *postv1.Post {
	return &postv1.Post{
		Id:         uuid.NewString(),
		AuthorId:   author.String(),
		Text:       "текст поста",
		LikesCount: 3,
		Liked:      true,
		CreatedAt:  timestamppb.New(time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)),
		UpdatedAt:  timestamppb.New(time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)),
	}
}

// authed делает запрос, у которого в контексте уже лежит id пользователя —
// ровно так, как его кладёт RequireAuth.
func authed(t *testing.T, method, target string, body string, id uuid.UUID) *http.Request {
	t.Helper()

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, bytes.NewBufferString(body))
	}
	return req.WithContext(context.WithValue(req.Context(), userIDKey, id))
}

// Пустая лента обязана сериализоваться как [], а не null: фронт делает
// posts.map(...) и на null падает целиком.
func TestFeedEmptyIsArrayNotNull(t *testing.T) {
	handler := NewPostHandler(&fakePostClient{}, &batchUserClient{})
	rec := httptest.NewRecorder()

	handler.Feed(rec, authed(t, http.MethodGet, "/feed", "", uuid.New()))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"posts":[]`) {
		t.Errorf("тело = %s, ожидался пустой массив posts", rec.Body.String())
	}
}

func TestFeedSendsCallerInMetadata(t *testing.T) {
	posts := &fakePostClient{}
	handler := NewPostHandler(posts, &batchUserClient{})
	caller := uuid.New()

	handler.Feed(httptest.NewRecorder(), authed(t, http.MethodGet, "/feed", "", caller))

	got := posts.lastMD.Get(grpcmeta.UserID)
	if len(got) == 0 || got[0] != caller.String() {
		t.Errorf("metadata %q = %v, want %v", grpcmeta.UserID, got, caller)
	}
}

func TestCreatePostSendsOnlyTextAndCallerFromToken(t *testing.T) {
	caller := uuid.New()
	other := uuid.New()
	posts := &fakePostClient{post: testProtoPost(caller)}
	handler := NewPostHandler(posts, &batchUserClient{})

	// Клиент пытается выдать пост за другого — тело обязано быть проигнорировано.
	body := `{"text":"мой пост","authorId":"` + other.String() + `","author_id":"` + other.String() + `"}`
	rec := httptest.NewRecorder()
	handler.CreatePost(rec, authed(t, http.MethodPost, "/posts", body, caller))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	req, ok := posts.lastReq.(*postv1.CreatePostRequest)
	if !ok {
		t.Fatalf("в post-service уехал %T, want *postv1.CreatePostRequest", posts.lastReq)
	}
	if req.GetText() != "мой пост" {
		t.Errorf("text = %q, want %q", req.GetText(), "мой пост")
	}

	md := posts.lastMD.Get(grpcmeta.UserID)
	if len(md) == 0 || md[0] != caller.String() {
		t.Fatalf("metadata %q = %v, want %v (только из токена)", grpcmeta.UserID, md, caller)
	}
	if len(md) > 0 && md[0] == other.String() {
		t.Fatal("id автора взят из тела запроса")
	}
}

func TestFeedHydratesAuthorsWithOneDedupedCall(t *testing.T) {
	author := uuid.New()
	users := &batchUserClient{batchUsers: []*userv1.User{testUser(author)}}
	posts := &fakePostClient{feed: &postv1.GetFeedResponse{
		Posts:      []*postv1.Post{testProtoPost(author), testProtoPost(author)},
		NextCursor: "курсор",
	}}
	handler := NewPostHandler(posts, users)

	rec := httptest.NewRecorder()
	handler.Feed(rec, authed(t, http.MethodGet, "/feed", "", uuid.New()))

	if users.batchCalls != 1 {
		t.Errorf("BatchGetUsers вызван %d раз, want 1 (одна пачка на страницу)", users.batchCalls)
	}
	if len(users.batchIDs) != 1 {
		t.Errorf("в пачке %d id, want 1 (дубли автора обязаны схлопнуться)", len(users.batchIDs))
	}

	var resp struct {
		Posts []struct {
			AuthorID string `json:"authorId"`
			Author   *struct {
				Handle string `json:"handle"`
			} `json:"author"`
		} `json:"posts"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("разбор ответа: %v", err)
	}
	if len(resp.Posts) != 2 {
		t.Fatalf("в ответе %d постов, want 2", len(resp.Posts))
	}
	for i, p := range resp.Posts {
		if p.Author == nil || p.Author.Handle == "" {
			t.Errorf("пост %d: автор не подставлен", i)
		}
	}
	if resp.NextCursor != "курсор" {
		t.Errorf("nextCursor = %q, want %q", resp.NextCursor, "курсор")
	}
}

// Падение user-service не должно ронять ленту: посты важнее имени автора.
func TestFeedDegradesWhenUserServiceFails(t *testing.T) {
	author := uuid.New()
	users := &batchUserClient{batchErr: status.Error(codes.Unavailable, "нет связи")}
	posts := &fakePostClient{feed: &postv1.GetFeedResponse{Posts: []*postv1.Post{testProtoPost(author)}}}
	handler := NewPostHandler(posts, users)

	rec := httptest.NewRecorder()
	handler.Feed(rec, authed(t, http.MethodGet, "/feed", "", uuid.New()))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (лента отдаётся и без профилей)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"author":null`) {
		t.Errorf("тело = %s, ожидался author: null", rec.Body.String())
	}
}

func TestFeedRejectsBadLimit(t *testing.T) {
	handler := NewPostHandler(&fakePostClient{}, &batchUserClient{})

	for _, raw := range []string{"abc", "-5", "99999999999999999999"} {
		rec := httptest.NewRecorder()
		handler.Feed(rec, authed(t, http.MethodGet, "/feed?limit="+raw, "", uuid.New()))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("limit=%s: status = %d, want 400", raw, rec.Code)
		}
	}
}

func TestFeedPassesCursorThrough(t *testing.T) {
	posts := &fakePostClient{}
	handler := NewPostHandler(posts, &batchUserClient{})

	handler.Feed(httptest.NewRecorder(), authed(t, http.MethodGet, "/feed?cursor=abc123&limit=7", "", uuid.New()))

	req, ok := posts.lastReq.(*postv1.GetFeedRequest)
	if !ok {
		t.Fatalf("в post-service уехал %T", posts.lastReq)
	}
	if req.GetCursor() != "abc123" || req.GetLimit() != 7 {
		t.Errorf("GetFeedRequest = {cursor: %q, limit: %d}, want {abc123, 7}", req.GetCursor(), req.GetLimit())
	}
}

func TestCreatePostRejectsEmptyBody(t *testing.T) {
	handler := NewPostHandler(&fakePostClient{}, &batchUserClient{})

	rec := httptest.NewRecorder()
	handler.CreatePost(rec, authed(t, http.MethodPost, "/posts", "не json", uuid.New()))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestLikeAndUnlikeSendPostIDAndCaller(t *testing.T) {
	caller := uuid.New()
	postID := uuid.New().String()

	t.Run("like", func(t *testing.T) {
		posts := &fakePostClient{post: testProtoPost(uuid.New())}
		handler := NewPostHandler(posts, &batchUserClient{})

		req := authed(t, http.MethodPost, "/posts/"+postID+"/like", "", caller)
		req.SetPathValue("id", postID)
		rec := httptest.NewRecorder()
		handler.Like(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		in, ok := posts.lastReq.(*postv1.LikePostRequest)
		if !ok || in.GetPostId() != postID {
			t.Errorf("LikePostRequest = %+v, want post_id %s", posts.lastReq, postID)
		}
		if md := posts.lastMD.Get(grpcmeta.UserID); len(md) == 0 || md[0] != caller.String() {
			t.Errorf("metadata = %v, want %v", md, caller)
		}
	})

	t.Run("unlike", func(t *testing.T) {
		posts := &fakePostClient{post: testProtoPost(uuid.New())}
		handler := NewPostHandler(posts, &batchUserClient{})

		req := authed(t, http.MethodDelete, "/posts/"+postID+"/like", "", caller)
		req.SetPathValue("id", postID)
		rec := httptest.NewRecorder()
		handler.Unlike(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		in, ok := posts.lastReq.(*postv1.UnlikePostRequest)
		if !ok || in.GetPostId() != postID {
			t.Errorf("UnlikePostRequest = %+v, want post_id %s", posts.lastReq, postID)
		}
		if md := posts.lastMD.Get(grpcmeta.UserID); len(md) == 0 || md[0] != caller.String() {
			t.Errorf("metadata = %v, want %v", md, caller)
		}
	})
}

func TestPostNotFoundMapsTo404(t *testing.T) {
	posts := &fakePostClient{err: status.Error(codes.NotFound, "post not found")}
	handler := NewPostHandler(posts, &batchUserClient{})

	req := authed(t, http.MethodGet, "/posts/x", "", uuid.New())
	req.SetPathValue("id", uuid.New().String())
	rec := httptest.NewRecorder()
	handler.Post(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// Чьи записи отдавать на /me/posts, решает ТОЛЬКО токен. Query-параметр —
// то, что клиент контролирует напрямую, и он обязан быть проигнорирован.
func TestMyPostsTakesAuthorFromTokenOnly(t *testing.T) {
	caller, other := uuid.New(), uuid.New()
	posts := &fakePostClient{}
	handler := NewPostHandler(posts, &batchUserClient{})

	handler.MyPosts(httptest.NewRecorder(),
		authed(t, http.MethodGet, "/me/posts?authorId="+other.String()+"&author_id="+other.String(), "", caller))

	req, ok := posts.lastReq.(*postv1.ListUserPostsRequest)
	if !ok {
		t.Fatalf("в post-service уехал %T, want *postv1.ListUserPostsRequest", posts.lastReq)
	}
	if req.GetAuthorId() != caller.String() {
		t.Errorf("author_id = %q, want %q (только из токена)", req.GetAuthorId(), caller)
	}
	if md := posts.lastMD.Get(grpcmeta.UserID); len(md) == 0 || md[0] != caller.String() {
		t.Errorf("metadata = %v, want %v", md, caller)
	}
}

// На чужом профиле автор и зритель РАЗНЫЕ. Если их склеить, лайки чужих
// постов будут показаны как чужие «нравится» или наоборот — а посты и вовсе
// окажутся своими.
func TestUserPostsResolvesHandleAndKeepsViewer(t *testing.T) {
	authorID := uuid.New()
	viewer := uuid.New()
	users := &batchUserClient{}
	users.user = testUser(authorID)
	posts := &fakePostClient{}
	handler := NewPostHandler(posts, users)

	req := authed(t, http.MethodGet, "/users/anya/posts", "", viewer)
	req.SetPathValue("handle", "anya")
	rec := httptest.NewRecorder()
	handler.UserPosts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	in, ok := posts.lastReq.(*postv1.ListUserPostsRequest)
	if !ok {
		t.Fatalf("в post-service уехал %T", posts.lastReq)
	}
	if in.GetAuthorId() != authorID.String() {
		t.Errorf("author_id = %q, want %q (id, добытый по handle)", in.GetAuthorId(), authorID)
	}
	if md := posts.lastMD.Get(grpcmeta.UserID); len(md) == 0 || md[0] != viewer.String() {
		t.Errorf("metadata = %v, want %v (зритель — из токена)", md, viewer)
	}
}

// Профиля нет — 404, а не пустой список: «нет такого пользователя» и «у
// пользователя нет записей» — разные ответы, и фронт рисует их по-разному.
func TestUserPostsUnknownHandleIs404(t *testing.T) {
	users := &batchUserClient{}
	users.err = status.Error(codes.NotFound, "user not found")
	handler := NewPostHandler(&fakePostClient{}, users)

	req := authed(t, http.MethodGet, "/users/nobody/posts", "", uuid.New())
	req.SetPathValue("handle", "nobody")
	rec := httptest.NewRecorder()
	handler.UserPosts(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// Пустой список записей обязан уехать как [], а не null: фронт делает
// posts.map(...) ровно так же, как на ленте.
func TestMyPostsEmptyIsArrayNotNull(t *testing.T) {
	handler := NewPostHandler(&fakePostClient{}, &batchUserClient{})
	rec := httptest.NewRecorder()

	handler.MyPosts(rec, authed(t, http.MethodGet, "/me/posts", "", uuid.New()))

	if !strings.Contains(rec.Body.String(), `"posts":[]`) {
		t.Errorf("тело = %s, ожидался пустой массив posts", rec.Body.String())
	}
}

func TestUserPostsRejectsBadLimit(t *testing.T) {
	users := &batchUserClient{}
	users.user = testUser(uuid.New())
	handler := NewPostHandler(&fakePostClient{}, users)

	req := authed(t, http.MethodGet, "/users/anya/posts?limit=abc", "", uuid.New())
	req.SetPathValue("handle", "anya")
	rec := httptest.NewRecorder()
	handler.UserPosts(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
