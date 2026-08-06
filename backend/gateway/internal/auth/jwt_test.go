package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testSecret = "test-secret"

// signToken собирает access-токен той же формы, что выпускает auth-service:
// HS256, идентификатор пользователя в sub.
func signToken(t *testing.T, secret string, sub string, expiresAt time.Time) string {
	t.Helper()

	claims := jwt.RegisteredClaims{
		Subject:   sub,
		Issuer:    "auth-service",
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	raw, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

func TestUserIDFromTokenAcceptsValidToken(t *testing.T) {
	want := uuid.New()
	raw := signToken(t, testSecret, want.String(), time.Now().Add(15*time.Minute))

	got, err := NewVerifier(testSecret).UserIDFromToken(raw)
	if err != nil {
		t.Fatalf("UserIDFromToken: %v", err)
	}
	if got != want {
		t.Errorf("id: got %v, want %v", got, want)
	}
}

func TestUserIDFromTokenRejectsExpiredToken(t *testing.T) {
	raw := signToken(t, testSecret, uuid.New().String(), time.Now().Add(-time.Minute))

	_, err := NewVerifier(testSecret).UserIDFromToken(raw)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for expired token, got %v", err)
	}
}

func TestUserIDFromTokenRejectsForeignSignature(t *testing.T) {
	raw := signToken(t, "someone-elses-secret", uuid.New().String(), time.Now().Add(15*time.Minute))

	_, err := NewVerifier(testSecret).UserIDFromToken(raw)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for foreign signature, got %v", err)
	}
}

// Токен с alg=none — классическая попытка обойти проверку подписи.
func TestUserIDFromTokenRejectsNoneAlgorithm(t *testing.T) {
	claims := jwt.RegisteredClaims{
		Subject:   uuid.New().String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none-token: %v", err)
	}

	_, err = NewVerifier(testSecret).UserIDFromToken(raw)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for alg=none, got %v", err)
	}
}

func TestUserIDFromTokenRejectsNonUUIDSubject(t *testing.T) {
	raw := signToken(t, testSecret, "not-a-uuid", time.Now().Add(15*time.Minute))

	_, err := NewVerifier(testSecret).UserIDFromToken(raw)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for non-uuid sub, got %v", err)
	}
}

func TestUserIDFromTokenRejectsGarbage(t *testing.T) {
	_, err := NewVerifier(testSecret).UserIDFromToken("не токен вовсе")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for garbage, got %v", err)
	}
}
