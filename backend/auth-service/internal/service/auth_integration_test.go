//go:build integration

package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"auth-service/internal/domain"
	"auth-service/internal/repository"
	"auth-service/internal/service"
	"auth-service/internal/token"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each test creates its own database on an explicitly supplied test server.
// Existing application databases and data are never used or truncated.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("integration tests require AUTH_TEST_DATABASE_URL pointing to a dedicated PostgreSQL test server")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "auth_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("clean test database: %v", err)
		}
		admin.Close(cleanupCtx)
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, source, _, _ := runtime.Caller(0)
	migrations, err := filepath.Glob(filepath.Join(filepath.Dir(source), "../../migrations/*.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find real migrations: %v", err)
	}
	for _, migration := range migrations {
		sql, err := os.ReadFile(migration)
		if err != nil {
			t.Fatal(err)
		}
		up := strings.Split(strings.TrimPrefix(string(sql), "-- +goose Up"), "-- +goose Down")[0]
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(migration), err)
		}
	}
	return pool
}

type publisher struct{ count atomic.Int32 }

func (p *publisher) PublishUserRegistered(context.Context, uuid.UUID, string) error {
	p.count.Add(1)
	return nil
}

type refreshHooks struct {
	domain.RefreshTokenRepository
	get    func(context.Context, string) (*domain.RefreshToken, error)
	create func(context.Context, *domain.RefreshToken) error
	mark   func(context.Context, uuid.UUID) (bool, error)
}

func (r *refreshHooks) GetByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	if r.get != nil {
		return r.get(ctx, hash)
	}
	return r.RefreshTokenRepository.GetByHash(ctx, hash)
}

func (r *refreshHooks) Create(ctx context.Context, tok *domain.RefreshToken) error {
	if r.create != nil {
		return r.create(ctx, tok)
	}
	return r.RefreshTokenRepository.Create(ctx, tok)
}

func (r *refreshHooks) MarkUsed(ctx context.Context, id uuid.UUID) (bool, error) {
	if r.mark != nil {
		return r.mark(ctx, id)
	}
	return r.RefreshTokenRepository.MarkUsed(ctx, id)
}

func fixture(t *testing.T) (*pgxpool.Pool, *service.AuthService, *refreshHooks, *token.Manager, *publisher) {
	t.Helper()
	pool := testPool(t)
	refresh := &refreshHooks{RefreshTokenRepository: repository.NewRefresh(pool)}
	tokens := token.NewManager("integration-test-only", time.Minute, time.Hour, "test")
	pub := &publisher{}
	svc := service.NewAuthService(repository.NewCredential(pool), refresh, tokens, repository.NewTxManager(pool), pub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return pool, svc, refresh, tokens, pub
}

func register(t *testing.T, svc *service.AuthService) domain.TokenPair {
	t.Helper()
	pair, err := svc.Register(t.Context(), "test@example.com", "password123", domain.SessionMeta{})
	if err != nil {
		t.Fatal(err)
	}
	return pair
}

func TestRefresh_LogoutInvalidatesPreviouslyReadToken(t *testing.T) {
	pool, svc, refresh, _, _ := fixture(t)
	pair := register(t, svc)
	read, resume := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	refresh.get = func(ctx context.Context, hash string) (*domain.RefreshToken, error) {
		stored, err := refresh.RefreshTokenRepository.GetByHash(ctx, hash)
		if calls.Add(1) == 1 {
			close(read)
			select {
			case <-resume:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return stored, err
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := svc.Refresh(ctx, pair.RefreshToken, domain.SessionMeta{}); result <- err }()
	select {
	case <-read:
	case <-ctx.Done():
		t.Fatal("refresh did not read the token")
	}
	logoutErr := svc.Logout(ctx, pair.RefreshToken)
	close(resume)
	if logoutErr != nil {
		t.Fatal(logoutErr)
	}
	if err := <-result; !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("refresh after completed logout: got %v, want invalid token", err)
	}
	var active int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NULL").Scan(&active); err != nil || active != 0 {
		t.Fatalf("active tokens after logout: %d, error %v", active, err)
	}
}

func TestRegister_RefreshFailureRollsBackCredentials(t *testing.T) {
	pool, svc, refresh, _, pub := fixture(t)
	injected := errors.New("injected refresh write failure")
	refresh.create = func(context.Context, *domain.RefreshToken) error { return injected }
	if _, err := svc.Register(t.Context(), "test@example.com", "password123", domain.SessionMeta{}); !errors.Is(err, injected) {
		t.Fatalf("register failure: %v", err)
	}
	var credentials int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM credentials").Scan(&credentials); err != nil || credentials != 0 {
		t.Fatalf("credentials survived failed registration: %d, error %v", credentials, err)
	}
	if pub.count.Load() != 0 {
		t.Fatal("failed registration published an event")
	}
	refresh.create = nil
	register(t, svc)
	if pub.count.Load() != 1 {
		t.Fatal("retry registration must publish exactly one event")
	}
}

func TestRefresh_CreateFailureRollsBackMarkUsed(t *testing.T) {
	_, svc, refresh, tokens, _ := fixture(t)
	pair := register(t, svc)
	injected := errors.New("injected rotation failure")
	refresh.create = func(context.Context, *domain.RefreshToken) error { return injected }
	if _, err := svc.Refresh(t.Context(), pair.RefreshToken, domain.SessionMeta{}); !errors.Is(err, injected) {
		t.Fatalf("rotation failure: %v", err)
	}
	stored, err := refresh.GetByHash(t.Context(), tokens.HashRefreshToken(pair.RefreshToken))
	if err != nil || stored.UsedAt != nil {
		t.Fatalf("failed rotation consumed token: %v", err)
	}
	refresh.create = nil
	if _, err := svc.Refresh(t.Context(), pair.RefreshToken, domain.SessionMeta{}); err != nil {
		t.Fatalf("retry rotation: %v", err)
	}
}

func TestRefresh_ReuseRevocationSurvivesRollback(t *testing.T) {
	pool, svc, _, _, _ := fixture(t)
	pair := register(t, svc)
	child, err := svc.Refresh(t.Context(), pair.RefreshToken, domain.SessionMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(t.Context(), pair.RefreshToken, domain.SessionMeta{}); !errors.Is(err, domain.ErrTokenReused) {
		t.Fatalf("reuse: %v", err)
	}
	var active int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NULL").Scan(&active); err != nil || active != 0 {
		t.Fatalf("revocation rolled back: %d active, error %v", active, err)
	}
	if _, err := svc.Refresh(t.Context(), child.RefreshToken, domain.SessionMeta{}); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("child after reuse: %v", err)
	}
}

func TestRefresh_ConcurrentReuseRevokesFamily(t *testing.T) {
	pool, svc, refresh, _, _ := fixture(t)
	pair := register(t, svc)
	bothRead, resume := make(chan struct{}, 2), make(chan struct{})
	var reads atomic.Int32
	refresh.get = func(ctx context.Context, hash string) (*domain.RefreshToken, error) {
		stored, err := refresh.RefreshTokenRepository.GetByHash(ctx, hash)
		if reads.Add(1) <= 2 {
			bothRead <- struct{}{}
			select {
			case <-resume:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return stored, err
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := svc.Refresh(ctx, pair.RefreshToken, domain.SessionMeta{}); results <- err }()
	}
	for range 2 {
		select {
		case <-bothRead:
		case <-ctx.Done():
			t.Fatal("both refresh calls did not read token")
		}
	}
	close(resume)
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, domain.ErrTokenReused)) || (second == nil && errors.Is(first, domain.ErrTokenReused))) {
		t.Fatalf("concurrent rotation outcomes: %v, %v", first, second)
	}
	var active int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NULL").Scan(&active); err != nil || active != 0 {
		t.Fatalf("active family after reuse: %d, error %v", active, err)
	}
}

func TestFamilyRevocation_WaitsForInflightRotation(t *testing.T) {
	for _, action := range []string{"logout", "reuse"} {
		t.Run(action, func(t *testing.T) {
			pool, svc, refresh, _, _ := fixture(t)
			original := register(t, svc)
			rotating := original
			if action == "reuse" {
				var err error
				rotating, err = svc.Refresh(t.Context(), original.RefreshToken, domain.SessionMeta{})
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			inserted, finishRotation := make(chan struct{}), make(chan struct{})
			refresh.create = func(ctx context.Context, tok *domain.RefreshToken) error {
				if err := refresh.RefreshTokenRepository.Create(ctx, tok); err != nil {
					return err
				}
				close(inserted)
				select {
				case <-finishRotation:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			rotationResult := make(chan error, 1)
			go func() { _, err := svc.Refresh(ctx, rotating.RefreshToken, domain.SessionMeta{}); rotationResult <- err }()
			select {
			case <-inserted:
			case <-ctx.Done():
				t.Fatal("rotation did not insert its child")
			}
			revocationResult := make(chan error, 1)
			go func() {
				if action == "logout" {
					revocationResult <- svc.Logout(ctx, rotating.RefreshToken)
				} else {
					_, err := svc.Refresh(ctx, original.RefreshToken, domain.SessionMeta{})
					revocationResult <- err
				}
			}()
			// Wait for actual DB lock contention, not an arbitrary delay. Revocation
			// must not finish while the rotating transaction can still insert a child.
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			waiting := false
			for !waiting {
				select {
				case err := <-revocationResult:
					close(finishRotation)
					<-rotationResult
					t.Fatalf("revocation finished before rotation committed: %v", err)
				case <-ctx.Done():
					close(finishRotation)
					t.Fatal("revocation did not wait for rotation")
				case <-ticker.C:
					var blocked int
					if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&blocked); err != nil {
						close(finishRotation)
						t.Fatal(err)
					}
					waiting = blocked > 0
				}
			}
			close(finishRotation)
			if err := <-rotationResult; err != nil {
				t.Fatalf("inflight rotation: %v", err)
			}
			err := <-revocationResult
			if (action == "logout" && err != nil) || (action == "reuse" && !errors.Is(err, domain.ErrTokenReused)) {
				t.Fatalf("revocation: %v", err)
			}
			var active int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NULL").Scan(&active); err != nil || active != 0 {
				t.Fatalf("inflight child escaped revocation: %d active, error %v", active, err)
			}
		})
	}
}

func TestRefresh_ReuseDoesNotRevokeAnotherUser(t *testing.T) {
	_, svc, _, _, _ := fixture(t)
	first := register(t, svc)
	second, err := svc.Register(t.Context(), "second@example.com", "password456", domain.SessionMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(t.Context(), first.RefreshToken, domain.SessionMeta{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(t.Context(), first.RefreshToken, domain.SessionMeta{}); !errors.Is(err, domain.ErrTokenReused) {
		t.Fatalf("reuse for first user: %v", err)
	}
	if _, err := svc.Refresh(t.Context(), second.RefreshToken, domain.SessionMeta{}); err != nil {
		t.Fatalf("another user's family was revoked: %v", err)
	}
}

func TestRefresh_RevokedTokenCannotBeMarkedUsed(t *testing.T) {
	_, svc, refresh, tokens, _ := fixture(t)
	pair := register(t, svc)
	stored, err := refresh.GetByHash(t.Context(), tokens.HashRefreshToken(pair.RefreshToken))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Logout(t.Context(), pair.RefreshToken); err != nil {
		t.Fatal(err)
	}
	marked, err := refresh.MarkUsed(t.Context(), stored.ID)
	if err != nil || marked {
		t.Fatalf("marked a revoked token: %v, error %v", marked, err)
	}
}

func TestTransactions_CancelledRequestRollsBack(t *testing.T) {
	for _, operation := range []string{"register", "refresh"} {
		t.Run(operation, func(t *testing.T) {
			pool, svc, refresh, tokens, pub := fixture(t)
			var original domain.TokenPair
			if operation == "refresh" {
				original = register(t, svc)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			refresh.create = func(ctx context.Context, tok *domain.RefreshToken) error {
				if err := refresh.RefreshTokenRepository.Create(ctx, tok); err != nil {
					return err
				}
				cancel()
				return ctx.Err()
			}
			var err error
			if operation == "register" {
				_, err = svc.Register(ctx, "test@example.com", "password123", domain.SessionMeta{})
			} else {
				_, err = svc.Refresh(ctx, original.RefreshToken, domain.SessionMeta{})
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled %s: %v", operation, err)
			}
			refresh.create = nil
			if operation == "register" {
				var count int
				if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM credentials").Scan(&count); err != nil || count != 0 {
					t.Fatalf("cancelled registration left credentials: %d, error %v", count, err)
				}
				if pub.count.Load() != 0 {
					t.Fatal("cancelled registration published an event")
				}
				register(t, svc)
			} else {
				stored, err := refresh.GetByHash(t.Context(), tokens.HashRefreshToken(original.RefreshToken))
				if err != nil || stored.UsedAt != nil {
					t.Fatalf("cancelled rotation consumed original: %v", err)
				}
				if _, err := svc.Refresh(t.Context(), original.RefreshToken, domain.SessionMeta{}); err != nil {
					t.Fatalf("retry after cancellation: %v", err)
				}
			}
		})
	}
}

func TestRefresh_CancellationAfterReuseStillRevokesFamily(t *testing.T) {
	pool, svc, refresh, _, _ := fixture(t)
	parent := register(t, svc)
	if _, err := svc.Refresh(t.Context(), parent.RefreshToken, domain.SessionMeta{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	refresh.mark = func(ctx context.Context, id uuid.UUID) (bool, error) {
		marked, err := refresh.RefreshTokenRepository.MarkUsed(ctx, id)
		if err == nil && !marked {
			cancel() // Client disconnects once reuse has been detected.
		}
		return marked, err
	}
	if _, err := svc.Refresh(ctx, parent.RefreshToken, domain.SessionMeta{}); !errors.Is(err, domain.ErrTokenReused) {
		t.Fatalf("reuse after disconnect: %v", err)
	}
	var active int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NULL").Scan(&active); err != nil || active != 0 {
		t.Fatalf("client cancellation prevented revocation: %d active, error %v", active, err)
	}
}
