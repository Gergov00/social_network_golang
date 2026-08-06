package grpc

import (
	userv1 "ahishka/pkg/gen/ahishka/user/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewServer собирает сконфигурированный gRPC-сервер. Слушающий сокет и
// lifecycle — не здесь: их заводит cmd/user/main.go через fx.
//
// reflection нужен, чтобы grpcurl и подобные клиенты работали без
// раздачи .proto файлов вручную.
func NewServer(handler *UserHandler) *grpc.Server {
	srv := grpc.NewServer()
	userv1.RegisterUserServiceServer(srv, handler)
	reflection.Register(srv)
	return srv
}
