package config

import (
	"errors"
	"os"
)

type Config struct {
	DBURL    string
	GRPCPort string
	LogLevel string
}

// GRPC-порт по умолчанию 50052, а не 50051: на хосте (вне docker) рядом
// уже слушает user-service на 50051, и совпадение дефолтов означало бы
// «сервис не стартует» при первой же локальной отладке двух сервисов сразу.
func Load() (*Config, error) {
	cfg := &Config{
		DBURL:    os.Getenv("DB_URL"),
		GRPCPort: getEnv("GRPC_PORT", "50052"),
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	if cfg.DBURL == "" {
		return nil, errors.New("DB_URL is required")
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
