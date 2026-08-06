package config

import (
	"errors"
	"os"
)

type Config struct {
	DBURL    string
	NATSURL  string
	GRPCPort string
	LogLevel string
}

func Load() (*Config, error) {
	cfg := &Config{
		DBURL:    os.Getenv("DB_URL"),
		NATSURL:  getEnv("NATS_URL", "nats://localhost:4222"),
		GRPCPort: getEnv("GRPC_PORT", "50051"),
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
