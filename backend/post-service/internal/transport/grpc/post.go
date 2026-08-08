package grpc

import (
	"context"
	"errors"

	postv1 "ahishka/pkg/gen/ahishka/post/v1"
	"post-service/internal/domain"
	"post-service/internal/service"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ postv1.PostServiceServer = (*PostHandler)(nil)

type PostHandler struct {
	// Встраивание обязательно: оно даёт forward-compatibility, когда в
	// контракт добавят новый rpc, а этот хендлер ещё его не реализует.
	postv1.UnimplementedPostServiceServer

	svc *service.PostService
}

func NewPostHandler(svc *service.PostService) *PostHandler {
	return &PostHandler{
		svc: svc,
	}
}

func (h *PostHandler) CreatePost(ctx context.Context, req *postv1.CreatePostRequest) (*postv1.CreatePostResponse, error) {
	author, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	post, err := h.svc.Create(ctx, author, req.GetText())
	if err != nil {
		return nil, mapError(err)
	}
	return &postv1.CreatePostResponse{Post: toProto(post)}, nil
}

func (h *PostHandler) GetPost(ctx context.Context, req *postv1.GetPostRequest) (*postv1.GetPostResponse, error) {
	viewer, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "id must be a uuid")
	}

	post, err := h.svc.GetByID(ctx, id, viewer)
	if err != nil {
		return nil, mapError(err)
	}
	return &postv1.GetPostResponse{Post: toProto(post)}, nil
}

func (h *PostHandler) GetFeed(ctx context.Context, req *postv1.GetFeedRequest) (*postv1.GetFeedResponse, error) {
	viewer, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	cursor, err := parseOptionalCursor(req.GetCursor())
	if err != nil {
		return nil, mapError(err)
	}

	posts, hasMore, err := h.svc.Feed(ctx, viewer, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, mapError(err)
	}

	page, next := toPage(posts, hasMore)
	return &postv1.GetFeedResponse{Posts: page, NextCursor: next}, nil
}

func (h *PostHandler) ListUserPosts(ctx context.Context, req *postv1.ListUserPostsRequest) (*postv1.ListUserPostsResponse, error) {
	// Зритель — из metadata. Автор — из поля запроса: это фильтр «чьи посты»,
	// а не личность вызывающего. Совпадают они только на своём профиле.
	viewer, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	author, err := uuid.Parse(req.GetAuthorId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "author_id must be a uuid")
	}

	cursor, err := parseOptionalCursor(req.GetCursor())
	if err != nil {
		return nil, mapError(err)
	}

	posts, hasMore, err := h.svc.UserPosts(ctx, author, viewer, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, mapError(err)
	}

	page, next := toPage(posts, hasMore)
	return &postv1.ListUserPostsResponse{Posts: page, NextCursor: next}, nil
}

// parseOptionalCursor: пустая строка значит «с начала», а не «кривой курсор».
func parseOptionalCursor(raw string) (*domain.FeedCursor, error) {
	if raw == "" {
		return nil, nil
	}
	return domain.ParseCursor(raw)
}

// toPage строит страницу и курсор на неё. Курсор указывает на последний
// ОТДАННЫЙ пост: следующая страница начнётся строго за ним. Пустой курсор =
// страниц больше нет.
func toPage(posts []domain.Post, hasMore bool) ([]*postv1.Post, string) {
	page := make([]*postv1.Post, 0, len(posts))
	for i := range posts {
		page = append(page, toProto(&posts[i]))
	}

	if !hasMore || len(posts) == 0 {
		return page, ""
	}
	last := posts[len(posts)-1]
	return page, domain.FeedCursor{CreatedAt: last.CreatedAt, ID: last.ID}.Encode()
}

func (h *PostHandler) LikePost(ctx context.Context, req *postv1.LikePostRequest) (*postv1.LikePostResponse, error) {
	caller, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "post_id must be a uuid")
	}

	post, err := h.svc.Like(ctx, postID, caller)
	if err != nil {
		return nil, mapError(err)
	}
	return &postv1.LikePostResponse{Post: toProto(post)}, nil
}

func (h *PostHandler) UnlikePost(ctx context.Context, req *postv1.UnlikePostRequest) (*postv1.UnlikePostResponse, error) {
	caller, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "post_id must be a uuid")
	}

	post, err := h.svc.Unlike(ctx, postID, caller)
	if err != nil {
		return nil, mapError(err)
	}
	return &postv1.UnlikePostResponse{Post: toProto(post)}, nil
}

// mapError переводит доменные ошибки в gRPC-коды. Наружу не отдаём текст
// ошибки от БД — только фиксированные сообщения (как у user-service).
func mapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, "post not found")
	case errors.Is(err, domain.ErrInvalidCursor):
		return status.Error(codes.InvalidArgument, "invalid cursor")
	case errors.Is(err, domain.ErrEmptyText):
		return status.Error(codes.InvalidArgument, "text must not be empty")
	case errors.Is(err, domain.ErrTextTooLong):
		return status.Error(codes.InvalidArgument, "text is too long")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "request deadline exceeded")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func toProto(post *domain.Post) *postv1.Post {
	return &postv1.Post{
		Id:            post.ID.String(),
		AuthorId:      post.AuthorID.String(),
		Text:          post.Text,
		LikesCount:    post.LikesCount,
		CommentsCount: post.CommentsCount,
		Liked:         post.Liked,
		CreatedAt:     timestamppb.New(post.CreatedAt),
		UpdatedAt:     timestamppb.New(post.UpdatedAt),
	}
}
