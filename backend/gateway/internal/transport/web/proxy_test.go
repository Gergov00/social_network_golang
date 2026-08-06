package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Прокси должен донести до auth-service тело и путь без изменений, а обратно
// вернуть статус, тело и Set-Cookie нетронутыми — на cookie держится вся
// сессия, и любая её порча ломает refresh.
func TestAuthProxyPassesRequestAndCookieThrough(t *testing.T) {
	var gotPath, gotBody, gotMethod string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)

		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    "opaque-token-value",
			Path:     "/auth",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"access_token":"header.payload.signature"}`))
	}))
	defer upstream.Close()

	proxy, err := NewAuthProxy(upstream.URL)
	if err != nil {
		t.Fatalf("NewAuthProxy: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/register",
		strings.NewReader(`{"email":"a@example.com","password":"password123"}`))
	proxy.ServeHTTP(rec, req)

	if gotMethod != http.MethodPost {
		t.Errorf("upstream method: got %q, want POST", gotMethod)
	}
	if gotPath != "/auth/register" {
		t.Errorf("upstream path: got %q, want /auth/register", gotPath)
	}
	if !strings.Contains(gotBody, "a@example.com") {
		t.Errorf("upstream body: got %q, want it to carry the email", gotBody)
	}

	if rec.Code != http.StatusCreated {
		t.Errorf("status: got %d, want 201", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "access_token") {
		t.Errorf("body: got %q, want it to carry access_token", rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "refresh_token" {
		t.Fatalf("Set-Cookie did not survive proxying: %v", cookies)
	}
	got := cookies[0]
	if got.Value != "opaque-token-value" || !got.HttpOnly {
		t.Errorf("cookie was altered in transit: %+v", got)
	}
	// These three attributes are the whole reason the cookie rides through a
	// bare reverse proxy instead of a hand-rolled handler: Path scopes it to
	// /auth, Secure/SameSite lock down transport and cross-site sending. If
	// any of them got dropped or rewritten, the refresh flow would silently
	// misbehave in the browser without any of the assertions above catching it.
	if got.Path != "/auth" {
		t.Errorf("cookie Path was altered in transit: got %q, want /auth", got.Path)
	}
	if !got.Secure {
		t.Error("cookie Secure attribute was dropped in transit")
	}
	if got.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite was altered in transit: got %v, want Strict", got.SameSite)
	}
}

// NewAuthProxy has two independent ways to reject a bad target, and it's easy
// to write a test that only ever exercises the first:
//   - url.Parse itself returns an error (e.g. "://not a url" has no valid
//     scheme separator at all).
//   - url.Parse succeeds but leaves Scheme/Host empty (e.g. "auth-service" — a
//     bare host with no "http://" prefix, exactly what a misconfigured
//     AUTH_SERVICE_URL would look like), which only the explicit guard clause
//     inside NewAuthProxy catches.
//
// Table-driven so both paths are visibly distinct and the second case can't
// quietly stop testing anything if the first one is "fixed" to also satisfy it.
func TestNewAuthProxyRejectsBadTarget(t *testing.T) {
	cases := []struct {
		name   string
		target string
	}{
		{"malformed url: rejected by url.Parse itself", "://not a url"},
		{"missing scheme/host: rejected by the guard clause", "auth-service"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewAuthProxy(tc.target); err == nil {
				t.Errorf("expected an error for target %q, got nil", tc.target)
			}
		})
	}
}
