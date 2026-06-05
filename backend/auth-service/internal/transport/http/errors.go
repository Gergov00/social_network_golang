package http

import (
	"auth-service/internal/domain"
	"errors"
	"log"
	"net/http"
)

func writeError(w http.ResponseWriter, err error) {
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
		log.Printf("Internal server error: %v", err)
	}
	writeJSON(w, status, errorResponse{Error: http.StatusText(status)})
}
