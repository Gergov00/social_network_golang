package web

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gateway/internal/auth"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const middlewareSecret = "middleware-test-secret"

func middlewareToken(t *testing.T, secret string, sub string) string {
	t.Helper()

	claims := jwt.RegisteredClaims{
		Subject:   sub,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return raw
}

// probe запоминает, дошёл ли запрос до защищённого хендлера и с каким id.
type probe struct {
	called bool
	gotID  uuid.UUID
}

func (p *probe) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.called = true
		p.gotID, _ = UserIDFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireAuthPassesValidTokenAndExposesUserID(t *testing.T) {
	want := uuid.New()
	p := &probe{}
	rec := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+middlewareToken(t, middlewareSecret, want.String()))

	RequireAuth(auth.NewVerifier(middlewareSecret))(p.handler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if !p.called {
		t.Fatal("protected handler was not reached")
	}
	if p.gotID != want {
		t.Errorf("user id in context: got %v, want %v", p.gotID, want)
	}
}

func TestRequireAuthBlocksMissingHeader(t *testing.T) {
	p := &probe{}
	rec := httptest.NewRecorder()

	RequireAuth(auth.NewVerifier(middlewareSecret))(p.handler()).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rec.Code)
	}
	if p.called {
		t.Error("SECURITY: protected handler reached without a token")
	}
}

func TestRequireAuthBlocksForeignSignature(t *testing.T) {
	p := &probe{}
	rec := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+middlewareToken(t, "wrong-secret", uuid.New().String()))

	RequireAuth(auth.NewVerifier(middlewareSecret))(p.handler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rec.Code)
	}
	if p.called {
		t.Error("SECURITY: protected handler reached with a foreign-signed token")
	}
}

// TestLoggingMiddlewareSetsRequestIDHeader guards Finding 3: the request id
// minted for the access log must also reach the client as X-Request-Id, so
// a user reporting "I got a 500" can be correlated to a log line.
func TestLoggingMiddlewareSetsRequestIDHeader(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	handler := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if got := rec.Header().Get("X-Request-Id"); got == "" {
		t.Error("X-Request-Id header not set on response")
	}
}

func TestRequireAuthBlocksMalformedHeader(t *testing.T) {
	p := &probe{}
	rec := httptest.NewRecorder()

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", middlewareToken(t, middlewareSecret, uuid.New().String())) // без "Bearer "

	RequireAuth(auth.NewVerifier(middlewareSecret))(p.handler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rec.Code)
	}
	if p.called {
		t.Error("SECURITY: protected handler reached with a malformed Authorization header")
	}
}
