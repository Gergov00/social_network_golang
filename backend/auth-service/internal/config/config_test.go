package config

import "testing"

func TestLoad_DefaultHTTPPort(t *testing.T) {
	t.Setenv("DB_URL", "postgres://test/test")
	t.Setenv("JWT_SECRET", "test-only")
	t.Setenv("HTTP_PORT", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPPort != "8080" {
		t.Fatalf("HTTPPort = %q, want 8080 without a leading colon", cfg.HTTPPort)
	}
}
