package grpc

import (
	"context"

	postv1 "ahishka/pkg/gen/ahishka/post/v1"
	"post-service/internal/domain"
	"post-service/internal/service"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ postv1.CommentServiceServer = (*CommentHandler)(nil)

type CommentHandler struct {
	// Встраивание обязательно: forward-compatibility, когда в контракт
	// добавят новый rpc, а этот хендлер ещё его не реализует.
	postv1.UnimplementedCommentServiceServer

	svc *service.CommentService
}

func NewCommentHandler(svc *service.CommentService) *CommentHandler {
	return &CommentHandler{
		svc: svc,
	}
}

func (h *CommentHandler) ListComments(ctx context.Context, req *postv1.ListCommentsRequest) (*postv1.ListCommentsResponse, error) {
	viewer, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "post_id must be a uuid")
	}

	cursor, err := parseOptionalCursor(req.GetCursor())
	if err != nil {
		return nil, mapError(err)
	}

	comments, hasMore, err := h.svc.List(ctx, postID, viewer, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, mapError(err)
	}

	page := make([]*postv1.Comment, 0, len(comments))
	for i := range comments {
		page = append(page, commentToProto(&comments[i]))
	}

	// Курсор указывает на последний ОТДАННЫЙ комментарий; следующая страница
	// начнётся строго за ним. Пустой курсор = страниц больше нет.
	var next string
	if hasMore && len(comments) > 0 {
		last := comments[len(comments)-1]
		next = domain.FeedCursor{CreatedAt: last.CreatedAt, ID: last.ID}.Encode()
	}

	return &postv1.ListCommentsResponse{Comments: page, NextCursor: next}, nil
}

func (h *CommentHandler) CreateComment(ctx context.Context, req *postv1.CreateCommentRequest) (*postv1.CreateCommentResponse, error) {
	author, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "post_id must be a uuid")
	}

	comment, err := h.svc.Create(ctx, postID, author, req.GetText())
	if err != nil {
		return nil, mapError(err)
	}
	return &postv1.CreateCommentResponse{Comment: commentToProto(comment)}, nil
}

func (h *CommentHandler) LikeComment(ctx context.Context, req *postv1.LikeCommentRequest) (*postv1.LikeCommentResponse, error) {
	caller, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetCommentId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "comment_id must be a uuid")
	}

	comment, err := h.svc.Like(ctx, id, caller)
	if err != nil {
		return nil, mapError(err)
	}
	return &postv1.LikeCommentResponse{Comment: commentToProto(comment)}, nil
}

func (h *CommentHandler) UnlikeComment(ctx context.Context, req *postv1.UnlikeCommentRequest) (*postv1.UnlikeCommentResponse, error) {
	caller, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetCommentId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "comment_id must be a uuid")
	}

	comment, err := h.svc.Unlike(ctx, id, caller)
	if err != nil {
		return nil, mapError(err)
	}
	return &postv1.UnlikeCommentResponse{Comment: commentToProto(comment)}, nil
}

func commentToProto(c *domain.Comment) *postv1.Comment {
	return &postv1.Comment{
		Id:         c.ID.String(),
		PostId:     c.PostID.String(),
		AuthorId:   c.AuthorID.String(),
		Text:       c.Text,
		LikesCount: c.LikesCount,
		Liked:      c.Liked,
		CreatedAt:  timestamppb.New(c.CreatedAt),
		UpdatedAt:  timestamppb.New(c.UpdatedAt),
	}
}
