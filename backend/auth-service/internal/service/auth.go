package service

import (
	"auth-service/internal/domain"
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var _ domain.AuthService = (*AuthService)(nil)

type AuthService struct {
	credRepo    domain.CredentialRepository
	refreshRepo domain.RefreshTokenRepository
	tx          domain.TxManager
	tokenProv   domain.TokenProvider
	logger      *slog.Logger
}

func NewAuthService(credRepo domain.CredentialRepository, refreshRepo domain.RefreshTokenRepository, tokenProv domain.TokenProvider, tx domain.TxManager, logger *slog.Logger) *AuthService {
	return &AuthService{
		credRepo:    credRepo,
		refreshRepo: refreshRepo,
		tokenProv:   tokenProv,
		tx:          tx,
		logger:      logger,
	}
}

func (s *AuthService) Register(ctx context.Context, email, password string, meta domain.SessionMeta) (domain.TokenPair, error) {

	hash, err := s.hashPassword(password)
	if err != nil {
		return domain.TokenPair{}, err
	}

	cred := &domain.Credential{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    time.Now(),
	}

	err = s.credRepo.Create(ctx, cred)
	if err != nil {
		return domain.TokenPair{}, err
	}

	familyID := uuid.New()
	return s.issueTokens(ctx, cred.ID, familyID, meta)
}

func (s *AuthService) Login(ctx context.Context, email, password string, meta domain.SessionMeta) (domain.TokenPair, error) {
	cred, err := s.credRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.TokenPair{}, domain.ErrInvalidCredentials
		}
		return domain.TokenPair{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(password)); err != nil {
		return domain.TokenPair{}, domain.ErrInvalidCredentials
	}

	familyID := uuid.New()
	return s.issueTokens(ctx, cred.ID, familyID, meta)
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	hash := s.tokenProv.HashRefreshToken(refreshToken)
	storedToken, err := s.refreshRepo.GetByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrInvalidToken
		}
		return err
	}

	err = s.refreshRepo.RevokeByFamilyID(ctx, storedToken.FamilyID)
	if err != nil {
		return err
	}

	return nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string, meta domain.SessionMeta) (domain.TokenPair, error) {
	hash := s.tokenProv.HashRefreshToken(refreshToken)
	storedToken, err := s.refreshRepo.GetByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.TokenPair{}, domain.ErrInvalidToken
		}
		return domain.TokenPair{}, err
	}

	if storedToken.RevokedAt != nil || time.Now().After(storedToken.ExpiresAt) {
		return domain.TokenPair{}, domain.ErrInvalidToken
	}

	var tokenPair domain.TokenPair
	reuse := false
	err = s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		marked, err := s.refreshRepo.MarkUsed(txCtx, storedToken.ID)
		if err != nil {
			return err
		}
		if !marked {
			reuse = true
			return domain.ErrTokenReused
		}

		tokenPair, err = s.issueTokens(txCtx, storedToken.UserID, storedToken.FamilyID, meta)
		return err
	})
	if reuse {
		s.logger.Warn("refresh token reuse detected",
			"user_id", storedToken.UserID,
			"family_id", storedToken.FamilyID,
		)
		if err := s.refreshRepo.RevokeByFamilyID(ctx, storedToken.FamilyID); err != nil {
			return domain.TokenPair{}, err
		}
		return domain.TokenPair{}, domain.ErrTokenReused

	}
	return tokenPair, err
}

func (s *AuthService) issueTokens(ctx context.Context, userID uuid.UUID, familyID uuid.UUID, meta domain.SessionMeta) (domain.TokenPair, error) {
	accessToken, err := s.tokenProv.NewAccessToken(userID)
	if err != nil {
		return domain.TokenPair{}, err
	}

	refreshToken, refreshTokenHash, err := s.tokenProv.NewRefreshToken()
	if err != nil {
		return domain.TokenPair{}, err
	}

	err = s.refreshRepo.Create(ctx, &domain.RefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: refreshTokenHash,
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(s.tokenProv.RefreshTokenTTL()),
		CreatedAt: time.Now(),
		UserAgent: meta.UserAgent,
		IP:        meta.IP,
	})
	if err != nil {
		return domain.TokenPair{}, err
	}

	return domain.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}
