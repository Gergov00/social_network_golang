package domain

import (
	uuid "github.com/google/uuid"
)

type TokenProvider interface {
    NewAccessToken(userID uuid.UUID) (string, error)
    ParseAccessToken(raw string) (uuid.UUID, error)
    NewRefreshToken() (raw string, hash string, err error)
    HashRefreshToken(raw string) (hash string)
}