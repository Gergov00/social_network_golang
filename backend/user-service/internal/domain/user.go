package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID
	Name      string
	Handle    string
	Bio       string
	City      string
	Work      string
	Verified  bool
	AvatarURL *string
	CreatedAt time.Time
	UpdatedAt time.Time
}
