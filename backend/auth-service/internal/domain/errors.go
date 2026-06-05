package domain

import (
	"errors"
)

var (
	ErrNotFound = errors.New("not found")

	ErrEmailTaken         = errors.New("email is already taken")
	ErrInvalidCredentials = errors.New("invalid email or password")

	ErrInvalidToken = errors.New("invalid refresh token")
	ErrTokenReused  = errors.New("refresh token has been reused")
)
