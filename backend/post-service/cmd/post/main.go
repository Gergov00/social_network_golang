package main

import (
	"context"
	"log/slog"
	"net"
	"os"

	"post-service/internal/config"
	"post-service/internal/domain"
	"post-service/internal/repository"
	"post-service/internal/service"
	postgrpc "post-service/internal/transport/grpc"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"google.golang.org/grpc"
)

func main() {
	godotenv.Load()

	app := fx.New(
		fx.Provide(
			config.Load,
			NewLogger,
			NewDataBase,
			fx.Annotate(repository.NewPost, fx.As(new(domain.PostRepository))),
			service.NewPostService,
			postgrpc.NewPostHandler,
			postgrpc.NewServer,
		),
		fx.WithLogger(func(logger *slog.Logger) fxevent.Logger {
			return &fxevent.SlogLogger{Logger: logger}
		}),
		fx.Invoke(RunGRPC),
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

func RunGRPC(lc fx.Lifecycle, srv *grpc.Server, cfg *config.Config, logger *slog.Logger, shutdowner fx.Shutdowner) {
	addr := ":" + cfg.GRPCPort

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Listen синхронно: занятый порт должен уронить старт, а не
			// всплыть ошибкой в фоновой горутине уже после «поднялись».
			lis, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}

			logger.Info("starting grpc server", "addr", addr)
			go func() {
				if err := srv.Serve(lis); err != nil {
					logger.Error("grpc server failed", "error", err)
					_ = shutdowner.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("stopping grpc server")
			srv.GracefulStop()
			return nil
		},
	})
}
