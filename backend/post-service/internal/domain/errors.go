package domain

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrInvalidCursor = errors.New("invalid cursor")
	ErrEmptyText     = errors.New("text is empty")
	ErrTextTooLong   = errors.New("text is too long")
)
