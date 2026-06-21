package web

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
)

type ctxKey int

const loggerKey ctxKey = iota

func loggerFrom(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqLogger := logger.With(
				"request_id", uuid.NewString(),
				"method", r.Method,
				"path", r.URL.Path,
			)
			ctx := context.WithValue(r.Context(), loggerKey, reqLogger)

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()

			next.ServeHTTP(rec, r.WithContext(ctx))

			reqLogger.Info("request handled",
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				loggerFrom(r.Context()).Error("panic recovered",
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				writeJSON(w, http.StatusInternalServerError,
					errorResponse{Error: http.StatusText(http.StatusInternalServerError)})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
