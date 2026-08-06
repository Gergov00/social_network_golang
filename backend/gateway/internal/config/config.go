package config

import (
	"errors"
	"os"
)

type Config struct {
	HTTPPort        string
	JWTSecret       string
	AuthServiceURL  string
	UserServiceAddr string
	LogLevel        string
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPPort:        getEnv("HTTP_PORT", "8080"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		AuthServiceURL:  getEnv("AUTH_SERVICE_URL", "http://localhost:10001"),
		UserServiceAddr: getEnv("USER_SERVICE_ADDR", "localhost:50051"),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
	}

	if cfg.JWTSecret == "" {
		return nil, errors.New("JWT_SECRET is required")
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
