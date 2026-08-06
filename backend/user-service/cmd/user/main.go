package main

import (
	"context"
	"log/slog"
	"net"
	"os"

	"ahishka/pkg/events"
	"user-service/internal/config"
	"user-service/internal/consumer"
	"user-service/internal/domain"
	"user-service/internal/repository"
	"user-service/internal/service"
	usergrpc "user-service/internal/transport/grpc"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
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
			NewNATS,
			NewJetStream,
			fx.Annotate(repository.NewUser, fx.As(new(domain.UserRepository))),
			service.NewUserService,
			usergrpc.NewUserHandler,
			usergrpc.NewServer,
			consumer.NewUserRegisteredConsumer,
		),
		fx.WithLogger(func(logger *slog.Logger) fxevent.Logger {
			return &fxevent.SlogLogger{Logger: logger}
		}),
		fx.Invoke(RunGRPC, RunConsumer),
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

func NewNATS(lc fx.Lifecycle, cfg *config.Config, logger *slog.Logger) (*nats.Conn, error) {
	nc, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("draining nats connection")
			return nc.Drain()
		},
	})
	return nc, nil
}

func NewJetStream(nc *nats.Conn) (jetstream.JetStream, error) {
	return jetstream.New(nc)
}

func RunGRPC(lc fx.Lifecycle, srv *grpc.Server, cfg *config.Config, logger *slog.Logger, shutdowner fx.Shutdowner) {
	addr := ":" + cfg.GRPCPort

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Listen синхронно: занятый порт должен уронить старт, а не всплыть
			// как ошибка в фоновой горутине уже после "приложение поднялось".
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

func RunConsumer(lc fx.Lifecycle, cons *consumer.UserRegisteredConsumer, js jetstream.JetStream, logger *slog.Logger) {
	var consumeCtx jetstream.ConsumeContext

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Стрим должен существовать до привязки консьюмера:
			// CreateOrUpdateConsumer к несуществующему стриму — ошибка.
			if err := events.EnsureEventsStream(ctx, js); err != nil {
				return err
			}

			logger.Info("starting user.registered consumer")
			started, err := cons.Start(ctx, js)
			if err != nil {
				return err
			}
			consumeCtx = started
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if consumeCtx != nil {
				logger.Info("stopping user.registered consumer")
				consumeCtx.Stop()
			}
			return nil
		},
	})
}
