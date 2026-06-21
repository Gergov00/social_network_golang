package web

import (
	"auth-service/internal/domain"
	"errors"
	"net/http"
)

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var status int
	switch {
	case errors.Is(err, domain.ErrEmailTaken):
		status = http.StatusConflict // 409
	case errors.Is(err, domain.ErrInvalidCredentials),
		errors.Is(err, domain.ErrInvalidToken),
		errors.Is(err, domain.ErrTokenReused):
		status = http.StatusUnauthorized // 401
	default:
		status = http.StatusInternalServerError // 500
		loggerFrom(r.Context()).Error("internal server error", "error", err)
	}
	writeJSON(w, status, errorResponse{Error: http.StatusText(status)})
}
