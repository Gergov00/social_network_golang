package token_test

import (
	"auth-service/internal/token"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testSecret = "auth-token-test-secret"

func TestManager_NewAccessToken(t *testing.T) {
	t.Parallel()
	const ttl = 15 * time.Minute
	manager := token.NewManager(testSecret, ttl, 24*time.Hour, "test-issuer")
	userID := uuid.New()
	before := time.Now().Truncate(time.Second)
	raw, err := manager.NewAccessToken(userID)
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now()
	claims := &token.AccessTokenClaims{}
	parsed, err := jwt.ParseWithClaims(raw, claims, func(_ *jwt.Token) (any, error) {
		return []byte(testSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("test-issuer"), jwt.WithExpirationRequired())
	if err != nil || !parsed.Valid {
		t.Fatalf("parse generated token: %v", err)
	}
	if claims.Subject != userID.String() {
		t.Fatalf("subject = %q, want %q", claims.Subject, userID)
	}
	if claims.IssuedAt == nil || claims.ExpiresAt == nil {
		t.Fatal("issued-at and expiry must be present")
	}
	if claims.IssuedAt.Time.Before(before) || claims.IssuedAt.Time.After(after) {
		t.Fatalf("issued-at %v outside generation interval [%v, %v]", claims.IssuedAt.Time, before, after)
	}
	if claims.ExpiresAt.Time.Before(before.Add(ttl)) || claims.ExpiresAt.Time.After(after.Add(ttl)) {
		t.Fatalf("expiry %v outside configured TTL bounds", claims.ExpiresAt.Time)
	}
	got, err := manager.ParseAccessToken(raw)
	if err != nil || got != userID {
		t.Fatalf("roundtrip = %v, %v; want %v", got, err, userID)
	}
}

func TestManager_ParseAccessTokenRejectsInvalidTokens(t *testing.T) {
	t.Parallel()
	manager := token.NewManager(testSecret, time.Minute, time.Hour, "test-issuer")
	userID := uuid.New()
	future := time.Now().Add(time.Hour)
	cases := []struct {
		name      string
		raw       string
		wantError error
	}{
		{name: "malformed", raw: "not-a-jwt", wantError: jwt.ErrTokenMalformed},
		{name: "expired", raw: signedToken(t, userID.String(), time.Now().Add(-time.Hour), testSecret), wantError: jwt.ErrTokenExpired},
		{name: "wrong signature", raw: signedToken(t, userID.String(), future, "another-secret"), wantError: jwt.ErrTokenSignatureInvalid},
		{name: "invalid UUID subject", raw: signedToken(t, "not-a-uuid", future, testSecret)},
		{name: "missing subject", raw: signedToken(t, "", future, testSecret)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := manager.ParseAccessToken(tc.raw)
			if err == nil || got != uuid.Nil {
				t.Fatalf("parse = %v, %v; want nil UUID and rejection", got, err)
			}
			if tc.wantError != nil && !errors.Is(err, tc.wantError) {
				t.Fatalf("error = %v, want %v", err, tc.wantError)
			}
		})
	}
}

// This test-only method deliberately accepts the supplied key. Rejection must
// therefore come from Manager's algorithm guard, not the JWT library's key check.
type permissiveSigningMethod struct{}

func (permissiveSigningMethod) Alg() string                      { return "AUTH_TEST_NON_HMAC" }
func (permissiveSigningMethod) Sign(string, any) ([]byte, error) { return []byte("signature"), nil }
func (permissiveSigningMethod) Verify(string, []byte, any) error { return nil }

func TestManager_ParseAccessTokenRejectsNonHMACMethod(t *testing.T) {
	jwt.RegisterSigningMethod("AUTH_TEST_NON_HMAC", func() jwt.SigningMethod { return permissiveSigningMethod{} })
	claims := jwt.RegisteredClaims{Subject: uuid.NewString(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
	raw, err := jwt.NewWithClaims(permissiveSigningMethod{}, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	manager := token.NewManager(testSecret, time.Minute, time.Hour, "test-issuer")
	got, err := manager.ParseAccessToken(raw)
	if err == nil || got != uuid.Nil || !strings.Contains(err.Error(), "unexpected signing method") {
		t.Fatalf("parse = %v, %v; want explicit algorithm rejection", got, err)
	}
}

func TestManager_NewRefreshToken(t *testing.T) {
	t.Parallel()
	manager := token.NewManager(testSecret, time.Minute, time.Hour, "test-issuer")
	seen := make(map[string]bool)
	for range 32 {
		raw, hash, err := manager.NewRefreshToken()
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := base64.URLEncoding.DecodeString(raw)
		if err != nil || len(decoded) != 32 || len(raw) != 44 {
			t.Fatalf("refresh encoding: raw length %d, decoded length %d, error %v", len(raw), len(decoded), err)
		}
		if seen[raw] {
			t.Fatal("repeated refresh token")
		}
		seen[raw] = true
		expected := sha256.Sum256([]byte(raw))
		if len(hash) != 44 || hash != base64.URLEncoding.EncodeToString(expected[:]) {
			t.Fatalf("refresh hash = %q, want padded base64 SHA256 of raw token", hash)
		}
	}
}

func TestManager_HashRefreshToken(t *testing.T) {
	t.Parallel()
	manager := token.NewManager(testSecret, time.Minute, time.Hour, "test-issuer")
	for _, raw := range []string{"", "a-refresh-token", "token-with-spaces "} {
		t.Run(raw, func(t *testing.T) {
			expected := sha256.Sum256([]byte(raw))
			if got := manager.HashRefreshToken(raw); got != base64.URLEncoding.EncodeToString(expected[:]) {
				t.Fatalf("hash = %q, want SHA256 of exact input", got)
			}
		})
	}
}

func TestManager_RefreshTokenTTL(t *testing.T) {
	t.Parallel()
	const ttl = 37*time.Hour + 17*time.Second
	manager := token.NewManager(testSecret, time.Minute, ttl, "test-issuer")
	if got := manager.RefreshTokenTTL(); got != ttl {
		t.Fatalf("TTL = %v, want %v", got, ttl)
	}
}

func signedToken(t *testing.T, subject string, expiry time.Time, secret string) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(expiry),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
