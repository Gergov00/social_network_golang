package domain

import (
	"context"
	"net/netip"
)

type SessionMeta struct {
	IP        *netip.Addr
	UserAgent *string
}

type AuthService interface {
	Register(ctx context.Context, email, password string, meta SessionMeta) (TokenPair, error)
	Login(ctx context.Context, email, password string, meta SessionMeta) (TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	Refresh(ctx context.Context, refreshToken string, meta SessionMeta) (TokenPair, error)
}
