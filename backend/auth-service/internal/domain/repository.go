package domain

import (
	"context"

	"github.com/google/uuid"
)

type CredentialRepository interface {
	GetByEmail(ctx context.Context, email string) (*Credential, error)
	Create(ctx context.Context, credential *Credential) error
	Update(ctx context.Context, credential *Credential) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *RefreshToken) error
	GetByHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	MarkUsed(ctx context.Context, token *RefreshToken) error
	RevokeByID(ctx context.Context, id uuid.UUID) error
	RevokeByUserID(ctx context.Context, userID uuid.UUID) error
	RevokeByFamilyID(ctx context.Context, familyID uuid.UUID) error
}
