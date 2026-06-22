package domain

import (
	"context"

	"github.com/google/uuid"
)

type EventPublisher interface {
	PublishUserRegistered(ctx context.Context, id uuid.UUID, email string) error
}
