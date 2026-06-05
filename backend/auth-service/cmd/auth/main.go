package main

import (
	"auth-service/internal/config"
	"auth-service/internal/domain"
	"auth-service/internal/repository"
	"auth-service/internal/service"
	"auth-service/internal/token"
	"auth-service/internal/transport/web"
	"context"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"go.uber.org/fx"
)

func main() {
	godotenv.Load()
	app := fx.New(
		fx.Provide(
			config.Load,
			fx.Annotate(repository.NewCredential, fx.As(new(domain.CredentialRepository))),
			fx.Annotate(repository.NewRefresh, fx.As(new(domain.RefreshTokenRepository))),
			fx.Annotate(repository.NewTxManager, fx.As(new(domain.TxManager))),
			fx.Annotate(NewTokerProvider, fx.As(new(domain.TokenProvider))),
			web.NewRouter,
			NewAuthHandlerProvider,
			NewDataBase,
			service.NewAuthService,
		),
		fx.Invoke(Run),
	)
	app.Run()
}

func Run(lc fx.Lifecycle, router http.Handler, cfg *config.Config) {
	srv := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: router,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Println("Starting server...")
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Fatalf("listen: %s\n", err)
				}
				
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Println("Stopping server...")
			return srv.Shutdown(ctx)
		},
	})

}

func NewDataBase(lc fx.Lifecycle, cfg *config.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), cfg.DBURL)
	if err != nil {
		log.Fatalf("db pool: %v", err)
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Println("Data base to conection...")
			return pool.Ping(ctx)
		},
		OnStop: func(ctx context.Context) error {
			log.Println("Closing data base connection...")
			pool.Close()
			return nil
		},
	})

	return pool, nil
}

func NewAuthHandlerProvider(cfg *config.Config, service *service.AuthService) *web.AuthHandler {
	return web.NewAuthHandler(service, cfg.RefreshTTL, cfg.CookieSecure)
}

func NewTokerProvider(cfg *config.Config) *token.Manager {
	return token.NewManager(cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL, cfg.Issuer)
}
