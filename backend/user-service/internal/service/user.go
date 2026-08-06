package service

import (
	"context"
	"time"

	"user-service/internal/domain"

	"github.com/google/uuid"
)

type UserService struct {
	repo domain.UserRepository
}

func NewUserService(repo domain.UserRepository) *UserService {
	return &UserService{
		repo: repo,
	}
}

// CreateProfile заводит профиль по сквозному id из auth-service.
// Профиль-заготовка: пустое имя и стаб-хендл, пользователь заполнит их позже.
func (s *UserService) CreateProfile(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	now := time.Now()

	user := &domain.User{
		ID:        id,
		Handle:    stubHandle(id),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *UserService) GetByHandle(ctx context.Context, handle string) (*domain.User, error) {
	return s.repo.GetByHandle(ctx, handle)
}

func (s *UserService) UpdateProfile(ctx context.Context, user *domain.User) (*domain.User, error) {
	user.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// stubHandle: "u_" + первые 8 hex-символов uuid.
// uuid.String() имеет вид "xxxxxxxx-xxxx-...", первые 8 символов — hex первой группы.
func stubHandle(id uuid.UUID) string {
	return "u_" + id.String()[:8]
}
