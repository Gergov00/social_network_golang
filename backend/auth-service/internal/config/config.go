package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DBURL        string
	JWTSecret    string
	Issuer       string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	HTTPPort     string
	CookieSecure bool
	LogLevel     string
}

func Load() (*Config, error) {
	cfg := &Config{
		DBURL:        os.Getenv("DB_URL"),
		JWTSecret:    os.Getenv("JWT_SECRET"),
		Issuer:       getEnv("JWT_ISSUER", "auth-service"),
		AccessTTL:    getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
		RefreshTTL:   getEnvDuration("JWT_REFRESH_TTL", 30*24*time.Hour),
		HTTPPort:     getEnv("HTTP_PORT", ":8080"),
		CookieSecure: getEnvBool("COOKIE_SECURE", true),
		LogLevel:     getEnv("LOG_LEVEL", "info"),
	}

	if cfg.DBURL == "" {
		return nil, errors.New("DB_URL is required")
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

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	value := getEnv(key, "")
	if value == "" {
		return defaultValue
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	return duration
}

func getEnvBool(key string, defaultValue bool) bool {
	value := getEnv(key, "")
	if value == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return b
}
