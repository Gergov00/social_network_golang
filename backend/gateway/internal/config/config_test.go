package config

import "testing"

// TestLoadRequiresJWTSecret guards the one piece of security-critical
// validation in this package: JWT_SECRET has no default. A default signing
// secret would let anyone who knows it mint valid access tokens, so an
// unset JWT_SECRET must fail startup rather than silently falling back to
// something.
func TestLoadRequiresJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")

	cfg, err := Load()

	if err == nil {
		t.Fatal("Load: got nil error with JWT_SECRET unset, want an error")
	}
	if cfg != nil {
		t.Errorf("Load: got non-nil config %+v alongside an error, want nil", cfg)
	}
}

func TestLoadWithJWTSecretSetSucceeds(t *testing.T) {
	t.Setenv("JWT_SECRET", "some-secret")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if cfg.JWTSecret != "some-secret" {
		t.Errorf("JWTSecret: got %q, want %q", cfg.JWTSecret, "some-secret")
	}
}

// TestLoadAppliesDocumentedDefaults locks in the documented defaults for
// everything that has one, so a future edit changing a default value (or
// silently removing it) shows up as a failing test.
func TestLoadAppliesDocumentedDefaults(t *testing.T) {
	t.Setenv("JWT_SECRET", "some-secret")
	t.Setenv("HTTP_PORT", "")
	t.Setenv("AUTH_SERVICE_URL", "")
	t.Setenv("USER_SERVICE_ADDR", "")
	t.Setenv("LOG_LEVEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	want := Config{
		HTTPPort:        "8080",
		JWTSecret:       "some-secret",
		AuthServiceURL:  "http://localhost:10001",
		UserServiceAddr: "localhost:50051",
		LogLevel:        "info",
	}
	if *cfg != want {
		t.Errorf("Load defaults: got %+v, want %+v", *cfg, want)
	}
}
