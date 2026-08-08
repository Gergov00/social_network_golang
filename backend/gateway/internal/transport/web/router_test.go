package web

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"
	"time"

	userv1 "ahishka/pkg/gen/ahishka/user/v1"
	"gateway/internal/auth"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

const routerTestSecret = "router-test-secret"

func routerToken(t *testing.T, sub string) string {
	t.Helper()

	claims := jwt.RegisteredClaims{
		Subject:   sub,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(routerTestSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return raw
}

// testAuthProxy builds a *httputil.ReverseProxy pointed at an arbitrary valid
// target. None of the tests in this file hit /auth/*, so the target is never
// actually dialed — NewRouter just needs a non-nil proxy to wire in.
func testAuthProxy(t *testing.T) *httputil.ReverseProxy {
	t.Helper()
	proxy, err := NewAuthProxy("http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("NewAuthProxy: %v", err)
	}
	return proxy
}

// panickingUserClient simulates an upstream failure severe enough to panic
// the handler, so RecoverMiddleware actually has something to catch. Only
// GetUser is exercised (via GET /me) — the other two methods are unused by
// this file's tests.
type panickingUserClient struct{}

func (panickingUserClient) GetUser(ctx context.Context, in *userv1.GetUserRequest, opts ...grpc.CallOption) (*userv1.GetUserResponse, error) {
	panic("boom: simulated upstream panic")
}

func (panickingUserClient) GetUserByHandle(ctx context.Context, in *userv1.GetUserByHandleRequest, opts ...grpc.CallOption) (*userv1.GetUserByHandleResponse, error) {
	return nil, nil
}

func (panickingUserClient) UpdateProfile(ctx context.Context, in *userv1.UpdateProfileRequest, opts ...grpc.CallOption) (*userv1.UpdateProfileResponse, error) {
	return nil, nil
}

func (panickingUserClient) BatchGetUsers(ctx context.Context, in *userv1.BatchGetUsersRequest, opts ...grpc.CallOption) (*userv1.BatchGetUsersResponse, error) {
	panic("upstream exploded")
}

// TestRouterRecoveredPanicIsLoggedWithCorrectStatus guards the load-bearing
// middleware composition order documented in NewRouter:
// LoggingMiddleware(logger)(RecoverMiddleware(mux)) — logging OUTSIDE,
// recover INSIDE.
//
// If the wrap were swapped, RecoverMiddleware would write the 500 on the raw
// http.ResponseWriter, outside the statusRecorder LoggingMiddleware installs.
// The client would still see 500 (that's the leaky part — a status-only
// assertion can't tell the orders apart), but the access log would report
// whatever status was written before the panic (here: default 200, since
// nothing was written yet) instead of 500. So this test asserts both: the
// HTTP response status AND that the *logged* status line says 500.
func TestRouterRecoveredPanicIsLoggedWithCorrectStatus(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	router := NewRouter(
		NewUserHandler(panickingUserClient{}),
		NewPostHandler(&fakePostClient{}, &batchUserClient{}),
		NewCommentHandler(&fakeCommentClient{}, &batchUserClient{}),
		testAuthProxy(t),
		auth.NewVerifier(routerTestSecret),
		logger,
	)

	id := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+routerToken(t, id.String()))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("response status: got %d, want 500", rec.Code)
	}

	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error != http.StatusText(http.StatusInternalServerError) {
		t.Errorf("body: got %q, want %q", body.Error, http.StatusText(http.StatusInternalServerError))
	}

	// The load-bearing assertion: the access log line (written by
	// LoggingMiddleware after next.ServeHTTP returns) must report status 500.
	// It can only do that if the statusRecorder it wrapped the handler chain
	// with saw the WriteHeader(500) call that RecoverMiddleware made — which
	// requires RecoverMiddleware to run INSIDE LoggingMiddleware, not outside.
	logOutput := logBuf.String()
	if !bytes.Contains(logBuf.Bytes(), []byte(`"status":500`)) {
		t.Errorf("access log did not report status 500 (order of middleware wrong?): %s", logOutput)
	}
}

func TestHealthRouteReturnsOK(t *testing.T) {
	router := NewRouter(
		NewUserHandler(panickingUserClient{}),
		NewPostHandler(&fakePostClient{}, &batchUserClient{}),
		NewCommentHandler(&fakeCommentClient{}, &batchUserClient{}),
		testAuthProxy(t),
		auth.NewVerifier(routerTestSecret),
		slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != `{"status":"ok"}`+"\n" {
		t.Errorf("body: got %q, want %q", got, `{"status":"ok"}`+"\n")
	}
}

// TestProtectedRoutesWithoutAuthorizationAreUnauthorized is a cheap
// route-table check: every route that is supposed to sit behind RequireAuth
// must actually be wired that way. RequireAuth is applied per-route in
// router.go (not as a group wrap), so a future edit dropping the
// requireAuth(...) wrap from any one of these lines would compile and pass
// every other test while silently exposing that route unauthenticated. This
// table covers all three protected routes individually so no single instance
// of that regression slips through.
func TestProtectedRoutesWithoutAuthorizationAreUnauthorized(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"GET /me", http.MethodGet, "/me"},
		{"PATCH /me", http.MethodPatch, "/me"},
		{"GET /users/{handle}", http.MethodGet, "/users/someone"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(
				NewUserHandler(panickingUserClient{}),
				NewPostHandler(&fakePostClient{}, &batchUserClient{}),
				NewCommentHandler(&fakeCommentClient{}, &batchUserClient{}),
				testAuthProxy(t),
				auth.NewVerifier(routerTestSecret),
				slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
			)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status: got %d, want 401", rec.Code)
			}
		})
	}
}

// TestPostRoutesRequireAuth покрывает ВСЕ пять маршрутов постов, а не один
// «представительный»: requireAuth навешивается на каждый маршрут отдельно,
// поэтому пропавшая обёртка на одной строке компилируется и проходит все
// остальные тесты, молча открывая этот маршрут наружу.
//
// Вторая половина каждого подтеста (с валидным токеном) стережёт сам факт
// регистрации маршрута: 404 значит, что строки в роутере нет.
func TestPostRoutesRequireAuth(t *testing.T) {
	id := uuid.New().String()
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/feed"},
		{http.MethodPost, "/posts"},
		{http.MethodGet, "/posts/" + id},
		{http.MethodPost, "/posts/" + id + "/like"},
		{http.MethodDelete, "/posts/" + id + "/like"},
		{http.MethodGet, "/me/posts"},
		{http.MethodGet, "/users/anya/posts"},
		{http.MethodGet, "/posts/" + id + "/comments"},
		{http.MethodPost, "/posts/" + id + "/comments"},
		{http.MethodPost, "/comments/" + id + "/like"},
		{http.MethodDelete, "/comments/" + id + "/like"},
	}

	router := NewRouter(
		NewUserHandler(&fakeUserClient{}),
		NewPostHandler(&fakePostClient{}, &batchUserClient{}),
		NewCommentHandler(&fakeCommentClient{}, &batchUserClient{}),
		testAuthProxy(t),
		auth.NewVerifier(routerTestSecret),
		slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
	)

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			// без токена
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(route.method, route.path, nil))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("без токена status = %d, want 401", rec.Code)
			}

			// с токеном маршрут обязан существовать (не 401 и не 404)
			rec = httptest.NewRecorder()
			req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"text":"пост"}`))
			req.Header.Set("Authorization", "Bearer "+routerToken(t, uuid.New().String()))
			router.ServeHTTP(rec, req)
			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusNotFound {
				t.Errorf("с токеном status = %d — маршрут не зарегистрирован", rec.Code)
			}
		})
	}
}
