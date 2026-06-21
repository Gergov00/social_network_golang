package main

import (
	"auth-service/internal/config"
	"auth-service/internal/domain"
	"auth-service/internal/repository"
	"auth-service/internal/service"
	"auth-service/internal/token"
	"auth-service/internal/transport/web"
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

func main() {
	godotenv.Load()
	app := fx.New(
		fx.Provide(
			config.Load,
			NewLogger,
			fx.Annotate(repository.NewCredential, fx.As(new(domain.CredentialRepository))),
			fx.Annotate(repository.NewRefresh, fx.As(new(domain.RefreshTokenRepository))),
			fx.Annotate(repository.NewTxManager, fx.As(new(domain.TxManager))),
			fx.Annotate(NewTokenProvider, fx.As(new(domain.TokenProvider))),
			web.NewRouter,
			NewAuthHandlerProvider,
			NewDataBase,
			service.NewAuthService,
		),
		fx.WithLogger(func(logger *slog.Logger) fxevent.Logger {
			return &fxevent.SlogLogger{Logger: logger}
		}),
		fx.Invoke(Run),
	)
	app.Run()
}

func Run(lc fx.Lifecycle, router http.Handler, cfg *config.Config, logger *slog.Logger, shutdowner fx.Shutdowner) {
	srv := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: router,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("starting http server", "addr", srv.Addr)
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logger.Error("http server failed", "error", err)
					_ = shutdowner.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("stopping http server")
			return srv.Shutdown(ctx)
		},
	})
}

func NewLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return logger
}

func NewDataBase(lc fx.Lifecycle, cfg *config.Config, logger *slog.Logger) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), cfg.DBURL)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("connecting to database")
			return pool.Ping(ctx)
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("closing database connection")
			pool.Close()
			return nil
		},
	})

	return pool, nil
}

func NewAuthHandlerProvider(cfg *config.Config, service *service.AuthService) *web.AuthHandler {
	return web.NewAuthHandler(service, cfg.RefreshTTL, cfg.CookieSecure)
}

func NewTokenProvider(cfg *config.Config) *token.Manager {
	return token.NewManager(cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL, cfg.Issuer)
}
