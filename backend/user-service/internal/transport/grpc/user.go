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

// MaxBatchIDs — потолок на размер пачки. Без него один запрос
// превращается в «прочитай мне всю таблицу users». 100 с запасом
// покрывает страницу ленты (limit там максимум 50).
const MaxBatchIDs = 100

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

	// handle не может быть явно очищен: он либо отсутствует в запросе (не
	// трогаем), либо присутствует и обязан быть непустым.
	if req.Handle != nil && strings.TrimSpace(req.GetHandle()) == "" {
		return nil, status.Error(codes.InvalidArgument, "handle must not be empty")
	}

	// Partial update (PATCH-семантика): читаем текущего пользователя и
	// накладываем только те поля, что реально присутствуют в запросе
	// (req.X != nil), остальные остаются как есть. Отсутствующее поле —
	// "оставить без изменений", присутствующее пустой строкой — "очистить".
	//
	// ВНИМАНИЕ: гонка read-modify-write. Между этим GetByID и UpdateProfile
	// ниже нет ни транзакции, ни optimistic-concurrency токена, так что два
	// конкурентных partial update могут затереть изменения друг друга
	// (last-write-wins на уровне каждого запроса, не поля). Осознанное
	// ограничение на этом этапе, не решаем его здесь.
	current, err := h.svc.GetByID(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}

	merged := *current
	if req.Name != nil {
		merged.Name = req.GetName()
	}
	if req.Handle != nil {
		merged.Handle = strings.TrimSpace(req.GetHandle())
	}
	if req.Bio != nil {
		merged.Bio = req.GetBio()
	}
	if req.City != nil {
		merged.City = req.GetCity()
	}
	if req.Work != nil {
		merged.Work = req.GetWork()
	}

	// service.UpdateProfile ставит UpdatedAt=time.Now() и мутирует merged на
	// месте (тот же указатель), так что updated и &merged — один объект.
	// Он уже несёт verified/avatar_url/created_at (пришли из current, а
	// repo.Update их не трогает) и свежий UpdatedAt. Второй поход в БД
	// (перечитывание после записи) больше не нужен: current, взятый до
	// записи, уже покрывает все поля, которые UPDATE не меняет.
	updated, err := h.svc.UpdateProfile(ctx, &merged)
	if err != nil {
		return nil, mapError(err)
	}
	return &userv1.UpdateProfileResponse{User: toProto(updated)}, nil
}

func (h *UserHandler) BatchGetUsers(ctx context.Context, req *userv1.BatchGetUsersRequest) (*userv1.BatchGetUsersResponse, error) {
	raw := req.GetIds()
	if len(raw) == 0 {
		return &userv1.BatchGetUsersResponse{}, nil
	}
	if len(raw) > MaxBatchIDs {
		return nil, status.Error(codes.InvalidArgument, "too many ids")
	}

	ids := make([]uuid.UUID, 0, len(raw))
	for _, value := range raw {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "ids must be uuids")
		}
		ids = append(ids, id)
	}

	users, err := h.svc.GetByIDs(ctx, ids)
	if err != nil {
		return nil, mapError(err)
	}

	// Отсутствующие профили в ответ просто не попадают — это не NotFound:
	// вызывающий склеивает посты с авторами и обязан переживать пробел.
	resp := &userv1.BatchGetUsersResponse{Users: make([]*userv1.User, 0, len(users))}
	for i := range users {
		resp.Users = append(resp.Users, toProto(&users[i]))
	}
	return resp, nil
}

// mapError переводит доменные ошибки в gRPC-коды. Наружу не отдаём текст
// ошибки от БД — только фиксированные сообщения (как writeError в auth).
func mapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, "user not found")
	case errors.Is(err, domain.ErrHandleTaken):
		return status.Error(codes.AlreadyExists, "handle already taken")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "request deadline exceeded")
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
