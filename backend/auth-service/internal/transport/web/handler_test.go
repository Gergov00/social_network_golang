package web

import (
	"auth-service/internal/domain"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type serviceStub struct {
	calls                int
	email, password, raw string
	meta                 domain.SessionMeta
	err                  error
}

func (s *serviceStub) issue(email, password, raw string, meta domain.SessionMeta) (domain.TokenPair, error) {
	s.calls++
	s.email = email
	s.password = password
	s.raw = raw
	s.meta = meta
	return domain.TokenPair{AccessToken: "access-test", RefreshToken: "refresh-test"}, s.err
}
func (s *serviceStub) Register(_ context.Context, e, p string, m domain.SessionMeta) (domain.TokenPair, error) {
	return s.issue(e, p, "", m)
}
func (s *serviceStub) Login(_ context.Context, e, p string, m domain.SessionMeta) (domain.TokenPair, error) {
	return s.issue(e, p, "", m)
}
func (s *serviceStub) Refresh(_ context.Context, r string, m domain.SessionMeta) (domain.TokenPair, error) {
	return s.issue("", "", r, m)
}
func (s *serviceStub) Logout(_ context.Context, r string) error { s.calls++; s.raw = r; return s.err }
func requestAuth(t *testing.T, s *serviceStub, secure bool, endpoint, body string, cookie bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/auth/"+endpoint, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.10:4321"
	r.Header.Set("User-Agent", "test-agent")
	if cookie {
		r.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "old-refresh"})
	}
	w := httptest.NewRecorder()
	NewRouter(NewAuthHandler(s, 24*time.Hour, secure), slog.New(slog.NewTextHandler(io.Discard, nil))).ServeHTTP(w, r)
	return w
}
func TestAuthSuccessCookies(t *testing.T) {
	for _, secure := range []bool{false, true} {
		for _, endpoint := range []string{"register", "login", "refresh", "logout"} {
			t.Run(fmt.Sprintf("%s/secure=%t", endpoint, secure), func(t *testing.T) {
				s := &serviceStub{}
				w := requestAuth(t, s, secure, endpoint, `{"email":"  Mixed@Example.com  ","password":" password "}`, true)
				wantStatus := http.StatusOK
				if endpoint == "register" {
					wantStatus = http.StatusCreated
				}
				if endpoint == "logout" {
					wantStatus = http.StatusNoContent
				}
				if w.Code != wantStatus {
					t.Fatalf("status=%d body=%s", w.Code, w.Body)
				}
				cookies := w.Result().Cookies()
				if len(cookies) != 1 {
					t.Fatalf("cookies=%v", cookies)
				}
				c := cookies[0]
				if c.Name != refreshCookieName || c.Path != "/auth" || !c.HttpOnly || c.Secure != secure || c.SameSite != http.SameSiteStrictMode {
					t.Fatalf("cookie flags=%+v", c)
				}
				if endpoint == "logout" {
					if c.Value != "" || c.MaxAge != -1 || w.Body.Len() != 0 {
						t.Fatalf("logout cookie=%+v body=%s", c, w.Body)
					}
				} else {
					if c.Value != "refresh-test" || c.MaxAge != 86400 {
						t.Fatalf("cookie=%+v", c)
					}
					var body map[string]string
					if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
						t.Fatal(err)
					}
					if len(body) != 1 || body["access_token"] != "access-test" {
						t.Fatalf("body=%v", body)
					}
					if w.Header().Get("Content-Type") != "application/json" {
						t.Fatal("missing JSON content type")
					}
					if s.meta.IP == nil || s.meta.IP.String() != "192.0.2.10" || s.meta.UserAgent == nil || *s.meta.UserAgent != "test-agent" {
						t.Fatalf("meta=%+v", s.meta)
					}
				}
				if s.calls != 1 {
					t.Fatalf("calls=%d", s.calls)
				}
				if endpoint == "register" || endpoint == "login" {
					if s.email != "Mixed@Example.com" || s.password != " password " {
						t.Fatalf("email=%q password=%q", s.email, s.password)
					}
				} else if s.raw != "old-refresh" {
					t.Fatalf("raw=%q", s.raw)
				}
			})
		}
	}
}
func TestAuthErrors(t *testing.T) {
	for _, endpoint := range []string{"register", "login", "refresh", "logout"} {
		for _, tc := range []struct {
			name   string
			err    error
			status int
		}{
			{"email", domain.ErrEmailTaken, 409}, {"credentials", domain.ErrInvalidCredentials, 401}, {"token", domain.ErrInvalidToken, 401}, {"reuse", domain.ErrTokenReused, 401}, {"internal", errors.New("database secret connection details"), 500},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				s := &serviceStub{err: fmt.Errorf("private wrapper: %w", tc.err)}
				w := requestAuth(t, s, true, endpoint, `{"email":"test@example.com","password":"password"}`, true)
				if w.Code != tc.status {
					t.Fatalf("status=%d body=%s", w.Code, w.Body)
				}
				var body errorResponse
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Error != http.StatusText(tc.status) {
					t.Fatalf("public error=%q", body.Error)
				}
				if len(w.Result().Cookies()) != 0 {
					t.Fatal("error response changed cookie")
				}
			})
		}
	}
}
func TestAuthMissingRefreshCookie(t *testing.T) {
	for _, endpoint := range []string{"refresh", "logout"} {
		t.Run(endpoint, func(t *testing.T) {
			s := &serviceStub{}
			w := requestAuth(t, s, false, endpoint, "", false)
			if w.Code != 401 || s.calls != 0 {
				t.Fatalf("status=%d calls=%d", w.Code, s.calls)
			}
			var body errorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error != "Missing refresh token" {
				t.Fatalf("error=%q", body.Error)
			}
		})
	}
}
func TestAuthInvalidPayloadDoesNotCallService(t *testing.T) {
	for _, endpoint := range []string{"register", "login"} {
		for _, body := range []string{"", `{`, `{"email":42}`, `{"email":"bad","password":"password"}`, `{"email":"test@example.com","password":""}`} {
			t.Run(endpoint+"/"+body, func(t *testing.T) {
				s := &serviceStub{}
				w := requestAuth(t, s, false, endpoint, body, false)
				if w.Code != 400 || s.calls != 0 {
					t.Fatalf("status=%d calls=%d body=%s", w.Code, s.calls, w.Body)
				}
				if len(w.Result().Cookies()) != 0 {
					t.Fatal("invalid payload changed cookie")
				}
			})
		}
	}
}
func TestRegisterPasswordByteBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, password string
		valid          bool
	}{{"seven", strings.Repeat("a", 7), false}, {"eight", strings.Repeat("a", 8), true}, {"seventy-two", strings.Repeat("a", 72), true}, {"seventy-three", strings.Repeat("a", 73), false}, {"unicode-eight-bytes", strings.Repeat("я", 4), true}, {"unicode-seventy-two", strings.Repeat("я", 36), true}, {"unicode-seventy-four", strings.Repeat("я", 37), false}, {"spaces-preserved", "        ", true}} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(registerRequest{Email: "test@example.com", Password: tc.password})
			s := &serviceStub{}
			w := requestAuth(t, s, false, "register", string(body), false)
			want := 400
			if tc.valid {
				want = 201
			}
			if w.Code != want {
				t.Fatalf("status=%d want=%d", w.Code, want)
			}
			if tc.valid {
				if s.calls != 1 || s.password != tc.password {
					t.Fatalf("calls=%d password=%q", s.calls, s.password)
				}
			} else if s.calls != 0 {
				t.Fatal("invalid password reached service")
			}
		})
	}
}
func TestAuthRejectsDisplayNameEmail(t *testing.T) {
	for _, endpoint := range []string{"register", "login"} {
		t.Run(endpoint, func(t *testing.T) {
			s := &serviceStub{}
			w := requestAuth(t, s, false, endpoint, `{"email":"Name <test@example.com>","password":"password"}`, false)
			if w.Code != 400 || s.calls != 0 {
				t.Fatalf("status=%d calls=%d", w.Code, s.calls)
			}
		})
	}
}
func TestLoginAcceptsNonEmptyPasswordWithoutRegisterLengthRestriction(t *testing.T) {
	for _, password := range []string{"a", strings.Repeat("a", 73), " "} {
		body, _ := json.Marshal(loginRequest{Email: "test@example.com", Password: password})
		s := &serviceStub{}
		w := requestAuth(t, s, false, "login", string(body), false)
		if w.Code != 200 || s.password != password {
			t.Fatalf("status=%d password=%q", w.Code, s.password)
		}
	}
}
func TestRefreshCookieUsesConfiguredTTL(t *testing.T) {
	const ttl = 37*time.Hour + 17*time.Second
	service := &serviceStub{}
	request := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	request.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "old-refresh"})
	response := httptest.NewRecorder()
	NewRouter(NewAuthHandler(service, ttl, false), slog.New(slog.NewTextHandler(io.Discard, nil))).ServeHTTP(response, request)
	cookies := response.Result().Cookies()
	if response.Code != http.StatusOK || len(cookies) != 1 || cookies[0].MaxAge != 133217 {
		t.Fatalf("status=%d cookies=%v; want configured TTL", response.Code, cookies)
	}
}
