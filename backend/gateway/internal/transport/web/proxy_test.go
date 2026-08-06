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
	if cookies[0].Value != "opaque-token-value" || !cookies[0].HttpOnly {
		t.Errorf("cookie was altered in transit: %+v", cookies[0])
	}
}

func TestNewAuthProxyRejectsBadTarget(t *testing.T) {
	if _, err := NewAuthProxy("://not a url"); err == nil {
		t.Error("expected an error for a malformed target url, got nil")
	}
}
