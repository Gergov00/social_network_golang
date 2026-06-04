package repository

import (
	"context"

	"auth-service/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CredentialRepo struct {
	pool *pgxpool.Pool
}

func NewCredential(pool *pgxpool.Pool) *CredentialRepo {
	return &CredentialRepo{
		pool: pool,
	}
}

func (c *CredentialRepo) Create(ctx context.Context, credential *domain.Credential) error {

	query := `INSERT INTO credentials (id, email, password_hash, created_at) VALUES ($1, $2, $3, $4)`

	_, err := c.pool.Exec(ctx, query, credential.ID, credential.Email, credential.PasswordHash, credential.CreatedAt)
	if err != nil {
		return err
	}

	return nil
}

func (c *CredentialRepo) GetByEmail(ctx context.Context, email string) (*domain.Credential, error) {
	var cred domain.Credential

	query := `SELECT id, email, password_hash, created_at FROM credentials WHERE email = $1`
	err := c.pool.QueryRow(ctx, query, email).Scan(&cred.ID, &cred.Email, &cred.PasswordHash, &cred.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &cred, nil
}

func (c *CredentialRepo) Update(ctx context.Context, credential *domain.Credential) error {
	query := `UPDATE credentials SET email = $1, password_hash = $2 WHERE id = $3`
	_, err := c.pool.Exec(ctx, query, credential.Email, credential.PasswordHash, credential.ID)
	return err
}

func (c *CredentialRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM credentials WHERE id = $1`
	_, err := c.pool.Exec(ctx, query, id)
	return err
}
