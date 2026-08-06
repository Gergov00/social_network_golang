package config

import "testing"

func TestLoadRequiresDBURL(t *testing.T) {
	t.Setenv("DB_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() без DB_URL должен вернуть ошибку, вернул nil")
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("DB_URL", "postgres://post:post@localhost:5437/post_db")
	t.Setenv("GRPC_PORT", "")
	t.Setenv("LOG_LEVEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.GRPCPort != "50052" {
		t.Errorf("GRPCPort = %q, ожидался дефолт 50052", cfg.GRPCPort)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, ожидался дефолт info", cfg.LogLevel)
	}
}

func TestLoadReadsEnv(t *testing.T) {
	t.Setenv("DB_URL", "postgres://x")
	t.Setenv("GRPC_PORT", "60000")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.DBURL != "postgres://x" || cfg.GRPCPort != "60000" || cfg.LogLevel != "debug" {
		t.Errorf("Load() = %+v, значения из env не подхватились", cfg)
	}
}
