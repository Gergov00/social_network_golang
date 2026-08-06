package grpc

import (
	"context"

	"ahishka/pkg/grpcmeta"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// callerID — ЕДИНСТВЕННЫЙ источник ответа на вопросы «кто автор» и «кто
// смотрит». Значение кладёт gateway, разобрав подпись JWT; в сообщениях
// контракта полей автора нет намеренно, иначе любой клиент gateway писал бы
// посты и ставил лайки от чужого имени.
//
// Отсутствие/непарсящийся id — Unauthenticated, а не InvalidArgument: это
// не «клиент прислал кривой аргумент», а «мы не знаем, кто пришёл».
func callerID(ctx context.Context) (uuid.UUID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "caller id is required")
	}

	values := md.Get(grpcmeta.UserID)
	if len(values) == 0 {
		return uuid.Nil, status.Error(codes.Unauthenticated, "caller id is required")
	}

	id, err := uuid.Parse(values[0])
	if err != nil {
		return uuid.Nil, status.Error(codes.Unauthenticated, "caller id must be a uuid")
	}
	return id, nil
}
