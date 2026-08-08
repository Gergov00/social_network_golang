package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"time"

	postv1 "ahishka/pkg/gen/ahishka/post/v1"
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
			NewUserClient,
			NewPostClient,
			NewCommentClient,
			NewAuthProxy,
			web.NewUserHandler,
			web.NewPostHandler,
			web.NewCommentHandler,
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

// dial поднимает соединение с сервисом данных. Это НЕ fx-провайдер:
// *grpc.ClientConn у нас теперь два (user и post), а два провайдера одного
// типа fx собрать не может. Соединение живёт внутри конструктора клиента.
//
// grpc.NewClient не ходит в сеть при создании — соединение поднимается
// лениво, поэтому недоступный сервис не роняет старт gateway (в отличие от
// NATS у user-service).
func dial(lc fx.Lifecycle, addr, name string, logger *slog.Logger) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("closing service connection", "service", name)
			return conn.Close()
		},
	})
	return conn, nil
}

func NewUserClient(lc fx.Lifecycle, cfg *config.Config, logger *slog.Logger) (userv1.UserServiceClient, error) {
	conn, err := dial(lc, cfg.UserServiceAddr, "user-service", logger)
	if err != nil {
		return nil, err
	}
	return userv1.NewUserServiceClient(conn), nil
}

func NewPostClient(lc fx.Lifecycle, cfg *config.Config, logger *slog.Logger) (postv1.PostServiceClient, error) {
	conn, err := dial(lc, cfg.PostServiceAddr, "post-service", logger)
	if err != nil {
		return nil, err
	}
	return postv1.NewPostServiceClient(conn), nil
}

// CommentService живёт в том же post-service, но соединение поднимаем своё:
// два fx-провайдера одного типа *grpc.ClientConn собрать нельзя (already
// provided), поэтому каждый клиент дозванивается сам — тот же паттерн, что
// у NewUserClient/NewPostClient.
func NewCommentClient(lc fx.Lifecycle, cfg *config.Config, logger *slog.Logger) (postv1.CommentServiceClient, error) {
	conn, err := dial(lc, cfg.PostServiceAddr, "post-service (comments)", logger)
	if err != nil {
		return nil, err
	}
	return postv1.NewCommentServiceClient(conn), nil
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
