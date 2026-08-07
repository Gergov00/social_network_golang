package web

import (
	"log/slog"
	"net/http"
	"net/http/httputil"

	"gateway/internal/auth"
)

func NewRouter(
	userHandler *UserHandler,
	postHandler *PostHandler,
	authProxy *httputil.ReverseProxy,
	verifier *auth.Verifier,
	logger *slog.Logger,
) http.Handler {
	mux := http.NewServeMux()

	// /auth/* уходит в auth-service нетронутым — свои токены он проверяет сам.
	mux.Handle("POST /auth/register", authProxy)
	mux.Handle("POST /auth/login", authProxy)
	mux.Handle("POST /auth/logout", authProxy)
	mux.Handle("POST /auth/refresh", authProxy)

	requireAuth := RequireAuth(verifier)

	mux.Handle("GET /me", requireAuth(http.HandlerFunc(userHandler.Me)))
	mux.Handle("PATCH /me", requireAuth(http.HandlerFunc(userHandler.UpdateMe)))
	mux.Handle("GET /users/{handle}", requireAuth(http.HandlerFunc(userHandler.UserByHandle)))

	mux.Handle("GET /feed", requireAuth(http.HandlerFunc(postHandler.Feed)))
	mux.Handle("POST /posts", requireAuth(http.HandlerFunc(postHandler.CreatePost)))
	mux.Handle("GET /posts/{id}", requireAuth(http.HandlerFunc(postHandler.Post)))
	mux.Handle("POST /posts/{id}/like", requireAuth(http.HandlerFunc(postHandler.Like)))
	mux.Handle("DELETE /posts/{id}/like", requireAuth(http.HandlerFunc(postHandler.Unlike)))

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return LoggingMiddleware(logger)(RecoverMiddleware(mux))
}
