package repository

import (
	"context"
	"errors"

	"user-service/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ domain.UserRepository = (*UserRepo)(nil)

const userColumns = `id, name, handle, bio, city, work, verified, avatar_url, created_at, updated_at`

type UserRepo struct {
	pool *pgxpool.Pool
}

func NewUser(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{
		pool: pool,
	}
}

// Create идемпотентен: повторная вставка того же id — не ошибка.
// Нужно для at-least-once доставки user.registered из JetStream.
func (u *UserRepo) Create(ctx context.Context, user *domain.User) error {
	query := `INSERT INTO users (` + userColumns + `)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	          ON CONFLICT (id) DO NOTHING`

	_, err := u.pool.Exec(ctx, query,
		user.ID, user.Name, user.Handle, user.Bio, user.City,
		user.Work, user.Verified, user.AvatarURL, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return mapUserErr(err)
	}
	return nil
}

func (u *UserRepo) Update(ctx context.Context, user *domain.User) error {
	query := `UPDATE users SET name = $1, handle = $2, bio = $3, city = $4, work = $5, updated_at = $6 WHERE id = $7`

	tag, err := u.pool.Exec(ctx, query,
		user.Name, user.Handle, user.Bio, user.City, user.Work, user.UpdatedAt, user.ID)
	if err != nil {
		return mapUserErr(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (u *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = $1`
	return scanUser(u.pool.QueryRow(ctx, query, id))
}

func (u *UserRepo) GetByHandle(ctx context.Context, handle string) (*domain.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE handle = $1`
	return scanUser(u.pool.QueryRow(ctx, query, handle))
}

func (u *UserRepo) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// = ANY($1) вместо IN (...): один параметр вместо склейки плейсхолдеров,
	// и план запроса не меняется от размера пачки.
	query := `SELECT ` + userColumns + ` FROM users WHERE id = ANY($1)`

	rows, err := u.pool.Query(ctx, query, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]domain.User, 0, len(ids))
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Name, &user.Handle, &user.Bio, &user.City,
			&user.Work, &user.Verified, &user.AvatarURL, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func scanUser(row pgx.Row) (*domain.User, error) {
	var user domain.User

	err := row.Scan(&user.ID, &user.Name, &user.Handle, &user.Bio, &user.City,
		&user.Work, &user.Verified, &user.AvatarURL, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

func mapUserErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		if pgErr.ConstraintName == "users_handle_key" {
			return domain.ErrHandleTaken
		}
	}
	return err
}
