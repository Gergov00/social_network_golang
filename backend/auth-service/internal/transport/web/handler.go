package web

import (
	"auth-service/internal/domain"
	"encoding/json"
	"net/http"
	"time"
)

type AuthHandler struct {
	service    domain.AuthService
	refreshTTL time.Duration
	secure     bool
}

func NewAuthHandler(service domain.AuthService, refreshTTL time.Duration, secure bool) *AuthHandler {
	return &AuthHandler{
		service:    service,
		refreshTTL: refreshTTL,
		secure:     secure,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body"})
		return
	}
	err := req.validate()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	meta := domain.SessionMeta{
		IP:        clientIP(r),
		UserAgent: userAgent(r),
	}
	tokenPair, err := h.service.Register(r.Context(), req.Email, req.Password, meta)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.setRefreshCookie(w, tokenPair.RefreshToken)
	writeJSON(w, http.StatusCreated, tokenResponse{AccessToken: tokenPair.AccessToken})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body"})
		return
	}

	err := req.validate()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	meta := domain.SessionMeta{
		IP:        clientIP(r),
		UserAgent: userAgent(r),
	}
	tokenPair, err := h.service.Login(r.Context(), req.Email, req.Password, meta)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.setRefreshCookie(w, tokenPair.RefreshToken)
	writeJSON(w, http.StatusOK, tokenResponse{AccessToken: tokenPair.AccessToken})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := h.readRefreshCookie(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Missing refresh token"})
		return
	}
	err = h.service.Logout(r.Context(), refreshToken)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := h.readRefreshCookie(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Missing refresh token"})
		return
	}
	meta := domain.SessionMeta{
		IP:        clientIP(r),
		UserAgent: userAgent(r),
	}
	tokenPair, err := h.service.Refresh(r.Context(), refreshToken, meta)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.setRefreshCookie(w, tokenPair.RefreshToken)
	writeJSON(w, http.StatusOK, tokenResponse{AccessToken: tokenPair.AccessToken})
}
