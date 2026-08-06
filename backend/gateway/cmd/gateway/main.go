package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"time"

	userv1 "ahishka/pkg/gen/ahishka/user/v1"
	"gateway/internal/auth"
	"gateway/internal/config"
	"gateway/internal/transport/web"

	"github.com/joho/godotenv"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	godotenv.Load()

	app := fx.New(
		fx.Provide(
			config.Load,
			NewLogger,
			NewVerifier,
			NewUserConn,
			NewUserClient,
			NewAuthProxy,
			web.NewUserHandler,
			web.NewRouter,
		),
		fx.WithLogger(func(logger *slog.Logger) fxevent.Logger {
			return &fxevent.SlogLogger{Logger: logger}
		}),
		fx.Invoke(Run),
	)
	app.Run()
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

func NewVerifier(cfg *config.Config) *auth.Verifier {
	return auth.NewVerifier(cfg.JWTSecret)
}

// NewUserConn держит соединение с user-service. grpc.NewClient не ходит в сеть
// при создании — соединение поднимается лениво, поэтому недоступный
// user-service не роняет старт gateway (в отличие от NATS у user-service).
func NewUserConn(lc fx.Lifecycle, cfg *config.Config, logger *slog.Logger) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(cfg.UserServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("closing user-service connection")
			return conn.Close()
		},
	})
	return conn, nil
}

func NewUserClient(conn *grpc.ClientConn) userv1.UserServiceClient {
	return userv1.NewUserServiceClient(conn)
}

func NewAuthProxy(cfg *config.Config) (*httputil.ReverseProxy, error) {
	return web.NewAuthProxy(cfg.AuthServiceURL)
}

func Run(lc fx.Lifecycle, router http.Handler, cfg *config.Config, logger *slog.Logger, shutdowner fx.Shutdowner) {
	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
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
