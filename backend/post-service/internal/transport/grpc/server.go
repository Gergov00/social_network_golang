package grpc

import (
	postv1 "ahishka/pkg/gen/ahishka/post/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewServer собирает сконфигурированный gRPC-сервер. Слушающий сокет и
// lifecycle — не здесь: их заводит cmd/post/main.go через fx.
//
// reflection нужен, чтобы grpcurl и подобные клиенты работали без раздачи
// .proto файлов вручную.
func NewServer(posts *PostHandler, comments *CommentHandler) *grpc.Server {
	srv := grpc.NewServer()
	postv1.RegisterPostServiceServer(srv, posts)
	postv1.RegisterCommentServiceServer(srv, comments)
	reflection.Register(srv)
	return srv
}
