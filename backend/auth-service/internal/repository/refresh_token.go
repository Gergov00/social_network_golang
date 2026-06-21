package repository

import (
	"auth-service/internal/domain"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ domain.RefreshTokenRepository = (*RefreshRepo)(nil)

type RefreshRepo struct {
	pool *pgxpool.Pool
}

func NewRefresh(pool *pgxpool.Pool) *RefreshRepo {
	return &RefreshRepo{
		pool: pool,
	}
}

func (r *RefreshRepo) Create(ctx context.Context, token *domain.RefreshToken) error {
	query := `INSERT INTO refresh_tokens (id, user_id, token_hash, family_id, used_at, revoked_at, expires_at, user_agent, ip, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := r.db(ctx).Exec(ctx, query, token.ID, token.UserID, token.TokenHash, token.FamilyID, token.UsedAt, token.RevokedAt, token.ExpiresAt, token.UserAgent, token.IP, token.CreatedAt)
	if err != nil {
		return err
	}
	return nil
}

func (r *RefreshRepo) GetByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	var token domain.RefreshToken

	query := `SELECT id, user_id, token_hash, family_id, used_at, revoked_at, expires_at, user_agent, ip, created_at FROM refresh_tokens WHERE token_hash = $1`
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(&token.ID, &token.UserID, &token.TokenHash, &token.FamilyID, &token.UsedAt, &token.RevokedAt, &token.ExpiresAt, &token.UserAgent, &token.IP, &token.CreatedAt)
	if err != nil {
		if errors.Is(err,
			pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &token, nil
}

func (r *RefreshRepo) MarkUsed(ctx context.Context, id uuid.UUID) (bool, error) {
	query := `UPDATE refresh_tokens SET used_at = now() WHERE id = $1 AND used_at IS NULL`
	tag, err := r.db(ctx).Exec(ctx, query, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *RefreshRepo) RevokeByID(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *RefreshRepo) RevokeByUserID(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`
	_, err := r.pool.Exec(ctx, query, userID)
	return err
}

func (r *RefreshRepo) RevokeByFamilyID(ctx context.Context, familyID uuid.UUID) error {
	query := `UPDATE refresh_tokens SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL`
	_, err := r.pool.Exec(ctx, query, familyID)
	return err
}

func (r *RefreshRepo) db(ctx context.Context) executor {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return r.pool
}
