package repository

import (
	"auth-service/internal/domain"
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

var _ domain.TxManager = (*TxManager)(nil)

type TxManager struct {
	px *pgxpool.Pool
}

func NewTxManager(px *pgxpool.Pool) *TxManager {
	return &TxManager{px: px}
}

type txKey struct{}

func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := m.px.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	ctx = context.WithValue(ctx, txKey{}, tx)

	err = fn(ctx)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
