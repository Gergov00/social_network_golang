package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	postv1 "ahishka/pkg/gen/ahishka/post/v1"
	userv1 "ahishka/pkg/gen/ahishka/user/v1"

	"github.com/google/uuid"
)

// CommentHandler — отдельный хендлер, а не метод PostHandler: у него свой
// gRPC-клиент (CommentService), и склеивать их значило бы тащить оба клиента
// туда, где нужен один.
type CommentHandler struct {
	comments postv1.CommentServiceClient
	users    userv1.UserServiceClient
}

func NewCommentHandler(comments postv1.CommentServiceClient, users userv1.UserServiceClient) *CommentHandler {
	return &CommentHandler{
		comments: comments,
		users:    users,
	}
}

func (h *CommentHandler) List(w http.ResponseWriter, r *http.Request) {
	caller, postID, ok := commentContext(w, r)
	if !ok {
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(withCaller(r.Context(), caller), upstreamTimeout)
	defer cancel()

	resp, err := h.comments.ListComments(ctx, &postv1.ListCommentsRequest{
		PostId: postID,
		Cursor: r.URL.Query().Get("cursor"),
		Limit:  limit,
	})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}

	authors := h.hydrate(ctx, r, resp.GetComments())

	// make, а не var: пустая ветка обязана уехать как [].
	comments := make([]commentDTO, 0, len(resp.GetComments()))
	for _, comment := range resp.GetComments() {
		comments = append(comments, toCommentDTO(comment, authors))
	}

	WriteJSON(w, http.StatusOK, commentsResponse{Comments: comments, NextCursor: resp.GetNextCursor()})
}

func (h *CommentHandler) Create(w http.ResponseWriter, r *http.Request) {
	caller, postID, ok := commentContext(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var body createCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, r, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(withCaller(r.Context(), caller), upstreamTimeout)
	defer cancel()

	// Автор — только metadata, пост — только путь. Всё, что клиент положил в
	// тело сверх text, json отбрасывает.
	resp, err := h.comments.CreateComment(ctx, &postv1.CreateCommentRequest{
		PostId: postID,
		Text:   body.Text,
	})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusCreated, toCommentDTO(resp.GetComment(), h.hydrateOne(ctx, r, resp.GetComment())))
}

func (h *CommentHandler) Like(w http.ResponseWriter, r *http.Request) {
	caller, commentID, ok := commentContext(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(withCaller(r.Context(), caller), upstreamTimeout)
	defer cancel()

	resp, err := h.comments.LikeComment(ctx, &postv1.LikeCommentRequest{CommentId: commentID})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, toCommentDTO(resp.GetComment(), h.hydrateOne(ctx, r, resp.GetComment())))
}

func (h *CommentHandler) Unlike(w http.ResponseWriter, r *http.Request) {
	caller, commentID, ok := commentContext(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(withCaller(r.Context(), caller), upstreamTimeout)
	defer cancel()

	resp, err := h.comments.UnlikeComment(ctx, &postv1.UnlikeCommentRequest{CommentId: commentID})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}

	WriteJSON(w, http.StatusOK, toCommentDTO(resp.GetComment(), h.hydrateOne(ctx, r, resp.GetComment())))
}

// commentContext достаёт id вызывающего из токена и id из пути — два
// обязательных входа всех четырёх хендлеров. Возвращает ok=false, когда
// ответ клиенту уже записан.
func commentContext(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, bool) {
	id, ok := UserIDFrom(r.Context())
	if !ok {
		WriteError(w, r, http.StatusUnauthorized)
		return uuid.Nil, "", false
	}

	pathID := strings.TrimSpace(r.PathValue("id"))
	if pathID == "" {
		WriteError(w, r, http.StatusBadRequest)
		return uuid.Nil, "", false
	}
	return id, pathID, true
}

// hydrate добирает профили авторов ветки ОДНИМ вызовом user-service.
// Логика дедупа и деградации — общая с постами (hydrateByIDs), поэтому
// повторяется только сбор id.
func (h *CommentHandler) hydrate(ctx context.Context, r *http.Request, comments []*postv1.Comment) map[string]userDTO {
	ids := make([]string, 0, len(comments))
	for _, comment := range comments {
		ids = append(ids, comment.GetAuthorId())
	}
	return hydrateByIDs(ctx, r, h.users, ids)
}

func (h *CommentHandler) hydrateOne(ctx context.Context, r *http.Request, comment *postv1.Comment) map[string]userDTO {
	if comment == nil {
		return nil
	}
	return h.hydrate(ctx, r, []*postv1.Comment{comment})
}
