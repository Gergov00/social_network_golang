package web

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gateway/internal/auth"

	"github.com/google/uuid"
)

type ctxKey int

const (
	loggerKey ctxKey = iota
	userIDKey
)

// statusRecorder запоминает код ответа, чтобы access-лог знал, чем всё кончилось.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			requestID := uuid.NewString()
			w.Header().Set("X-Request-Id", requestID)

			reqLogger := logger.With("request_id", requestID)
			ctx := context.WithValue(r.Context(), loggerKey, reqLogger)

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r.WithContext(ctx))

			reqLogger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration", time.Since(start).String(),
			)
		})
	}
}

func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				LoggerFrom(r.Context()).Error("panic recovered", "panic", rec)
				WriteError(w, r, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequireAuth пропускает дальше только запросы с валидным access-токеном в
// заголовке Authorization: Bearer <token>, и кладёт id пользователя в контекст.
func RequireAuth(verifier *auth.Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			raw, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || strings.TrimSpace(raw) == "" {
				WriteError(w, r, http.StatusUnauthorized)
				return
			}

			id, err := verifier.UserIDFromToken(strings.TrimSpace(raw))
			if err != nil {
				WriteError(w, r, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFrom достаёт id пользователя, положенный RequireAuth.
func UserIDFrom(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey).(uuid.UUID)
	return id, ok
}

func LoggerFrom(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
