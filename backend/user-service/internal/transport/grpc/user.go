package grpc

import (
	"context"
	"errors"
	"strings"

	userv1 "ahishka/pkg/gen/ahishka/user/v1"
	"user-service/internal/domain"
	"user-service/internal/service"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ userv1.UserServiceServer = (*UserHandler)(nil)

type UserHandler struct {
	// Встраивание обязательно: оно даёт forward-compatibility, когда в
	// контракт добавят новый rpc, а этот хендлер ещё его не реализует.
	userv1.UnimplementedUserServiceServer

	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{
		svc: svc,
	}
}

func (h *UserHandler) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "id must be a uuid")
	}

	user, err := h.svc.GetByID(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.GetUserResponse{User: toProto(user)}, nil
}

func (h *UserHandler) GetUserByHandle(ctx context.Context, req *userv1.GetUserByHandleRequest) (*userv1.GetUserByHandleResponse, error) {
	handle := strings.TrimSpace(req.GetHandle())
	if handle == "" {
		return nil, status.Error(codes.InvalidArgument, "handle must not be empty")
	}

	user, err := h.svc.GetByHandle(ctx, handle)
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.GetUserByHandleResponse{User: toProto(user)}, nil
}

func (h *UserHandler) UpdateProfile(ctx context.Context, req *userv1.UpdateProfileRequest) (*userv1.UpdateProfileResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "id must be a uuid")
	}

	handle := strings.TrimSpace(req.GetHandle())
	if handle == "" {
		return nil, status.Error(codes.InvalidArgument, "handle must not be empty")
	}

	if _, err := h.svc.UpdateProfile(ctx, &domain.User{
		ID:     id,
		Name:   req.GetName(),
		Handle: handle,
		Bio:    req.GetBio(),
		City:   req.GetCity(),
		Work:   req.GetWork(),
	}); err != nil {
		return nil, mapError(err)
	}

	// Перечитываем: service.UpdateProfile возвращает ту же сущность, что ему
	// передали, а в неё не входят verified/avatar_url/created_at (репозиторий
	// их не обновляет и не читает при UPDATE). Без перечитывания ответ врал бы
	// нулями по этим полям.
	updated, err := h.svc.GetByID(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.UpdateProfileResponse{User: toProto(updated)}, nil
}

// mapError переводит доменные ошибки в gRPC-коды. Наружу не отдаём текст
// ошибки от БД — только фиксированные сообщения (как writeError в auth).
func mapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, "user not found")
	case errors.Is(err, domain.ErrHandleTaken):
		return status.Error(codes.AlreadyExists, "handle already taken")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func toProto(user *domain.User) *userv1.User {
	return &userv1.User{
		Id:        user.ID.String(),
		Name:      user.Name,
		Handle:    user.Handle,
		Bio:       user.Bio,
		City:      user.City,
		Work:      user.Work,
		Verified:  user.Verified,
		AvatarUrl: user.AvatarURL,
		CreatedAt: timestamppb.New(user.CreatedAt),
		UpdatedAt: timestamppb.New(user.UpdatedAt),
	}
}
