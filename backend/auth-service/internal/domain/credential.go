package domain

import (
	
	"net/netip"
	"time"

	"github.com/google/uuid"
)

type Credential struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	FamilyID  uuid.UUID
	UsedAt    *time.Time
	RevokedAt *time.Time
	ExpiresAt time.Time
	UserAgent string
	IP        netip.Addr
	CreatedAt time.Time
}