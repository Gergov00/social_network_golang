package web

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	postv1 "ahishka/pkg/gen/ahishka/post/v1"
	userv1 "ahishka/pkg/gen/ahishka/user/v1"
	"ahishka/pkg/grpcmeta"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

var errInvalidLimit = errors.New("invalid limit")

type PostHandler struct {
	posts postv1.PostServiceClient
	users userv1.UserServiceClient
}

func NewPostHandler(posts postv1.PostServiceClient, users userv1.UserServiceClient) *PostHandler {
	return &PostHandler{
		posts: posts,
		users: users,
	}
}

// withCaller кладёт id вызывающего в исходящую gRPC-metadata. Это
// ЕДИНСТВЕННЫЙ способ, которым post-service узнаёт, кто пришёл: id взят из
// провалидированного JWT и никогда — из тела, query или заголовка клиента.
func withCaller(ctx context.Context, id uuid.UUID) context.Context {
	return metadata.AppendToOutgoingContext(ctx, grpcmeta.UserID, id.String())
}

func (h *PostHandler) Feed(w http.ResponseWriter, r *http.Request) {
	id, ok := UserIDFrom(r.Context())
	if !ok {
		WriteError(w, r, http.StatusUnauthorized)
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(withCaller(r.Context(), id), upstreamTimeout)
	defer cancel()

	resp, err := h.posts.GetFeed(ctx, &postv1.GetFeedRequest{
		Cursor: r.URL.Query().Get("cursor"),
		Limit:  limit,
	})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}

	authors := h.hydrateAuthors(ctx, r, resp.GetPosts())

	// make, а не var: пустая лента обязана уехать как [], иначе фронт
	// получит null и упадёт на posts.map(...).
	posts := make([]postDTO, 0, len(resp.GetPosts()))
	for _, post := range resp.GetPosts() {
		posts = append(posts, toPostDTO(post, authors))
	}

	WriteJSON(w, http.StatusOK, feedResponse{Posts: posts, NextCursor: resp.GetNextCursor()})
}

func (h *PostHandler) CreatePost(w http.ResponseWriter, r *http.Request) {
	id, ok := UserIDFrom(r.Context())
	if !ok {
		WriteError(w, r, http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var body createPostRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, r, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(withCaller(r.Context(), id), upstreamTimeout)
	defer cancel()

	// Автор не передаётся полем — только metadata. Всё, что клиент положил
	// в тело сверх text, json просто отбрасывает.
	resp, err := h.posts.CreatePost(ctx, &postv1.CreatePostRequest{Text: body.Text})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusCreated, toPostDTO(resp.GetPost(), h.hydrateOne(ctx, r, resp.GetPost())))
}

func (h *PostHandler) Post(w http.ResponseWriter, r *http.Request) {
	id, ok := UserIDFrom(r.Context())
	if !ok {
		WriteError(w, r, http.StatusUnauthorized)
		return
	}

	postID := strings.TrimSpace(r.PathValue("id"))
	if postID == "" {
		WriteError(w, r, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(withCaller(r.Context(), id), upstreamTimeout)
	defer cancel()

	resp, err := h.posts.GetPost(ctx, &postv1.GetPostRequest{Id: postID})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, toPostDTO(resp.GetPost(), h.hydrateOne(ctx, r, resp.GetPost())))
}

func (h *PostHandler) Like(w http.ResponseWriter, r *http.Request) {
	id, ok := UserIDFrom(r.Context())
	if !ok {
		WriteError(w, r, http.StatusUnauthorized)
		return
	}

	postID := strings.TrimSpace(r.PathValue("id"))
	if postID == "" {
		WriteError(w, r, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(withCaller(r.Context(), id), upstreamTimeout)
	defer cancel()

	resp, err := h.posts.LikePost(ctx, &postv1.LikePostRequest{PostId: postID})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, toPostDTO(resp.GetPost(), h.hydrateOne(ctx, r, resp.GetPost())))
}

func (h *PostHandler) Unlike(w http.ResponseWriter, r *http.Request) {
	id, ok := UserIDFrom(r.Context())
	if !ok {
		WriteError(w, r, http.StatusUnauthorized)
		return
	}

	postID := strings.TrimSpace(r.PathValue("id"))
	if postID == "" {
		WriteError(w, r, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(withCaller(r.Context(), id), upstreamTimeout)
	defer cancel()

	resp, err := h.posts.UnlikePost(ctx, &postv1.UnlikePostRequest{PostId: postID})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, toPostDTO(resp.GetPost(), h.hydrateOne(ctx, r, resp.GetPost())))
}

// hydrateAuthors добирает профили авторов страницы ОДНИМ вызовом
// user-service: N постов не должны превращаться в N round-trip'ов.
//
// Ошибка user-service ленту не роняет — это деградация, а не отказ: посты
// уедут с author == null. Лента без имён полезнее, чем 500.
func (h *PostHandler) hydrateAuthors(ctx context.Context, r *http.Request, posts []*postv1.Post) map[string]userDTO {
	ids := make([]string, 0, len(posts))
	seen := make(map[string]struct{}, len(posts))

	for _, post := range posts {
		id := post.GetAuthorId()
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil
	}

	resp, err := h.users.BatchGetUsers(ctx, &userv1.BatchGetUsersRequest{Ids: ids})
	if err != nil {
		LoggerFrom(r.Context()).Warn("hydrating post authors failed", "error", err, "ids", len(ids))
		return nil
	}

	authors := make(map[string]userDTO, len(resp.GetUsers()))
	for _, user := range resp.GetUsers() {
		authors[user.GetId()] = toUserDTO(user)
	}
	return authors
}

func (h *PostHandler) hydrateOne(ctx context.Context, r *http.Request, post *postv1.Post) map[string]userDTO {
	if post == nil {
		return nil
	}
	return h.hydrateAuthors(ctx, r, []*postv1.Post{post})
}

// parseLimit: пустая строка → 0, что для post-service значит «свой дефолт».
// Всё, что не влезает в int32 или отрицательно, — ошибка клиента, а не
// повод молча подставить дефолт.
func parseLimit(raw string) (int32, error) {
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || value > math.MaxInt32 {
		return 0, errInvalidLimit
	}
	return int32(value), nil
}
