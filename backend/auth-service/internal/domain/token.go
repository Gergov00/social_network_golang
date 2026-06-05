package domain

import (
	"time"

	uuid "github.com/google/uuid"
)

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type TokenProvider interface {
	NewAccessToken(userID uuid.UUID) (string, error)
	ParseAccessToken(raw string) (uuid.UUID, error)
	NewRefreshToken() (raw string, hash string, err error)
	HashRefreshToken(raw string) (hash string)
	RefreshTokenTTL() time.Duration
}
