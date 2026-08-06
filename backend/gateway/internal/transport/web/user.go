package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	userv1 "ahishka/pkg/gen/ahishka/user/v1"
)

// upstreamTimeout ограничивает время ожидания ответа от user-service по
// gRPC. Должен быть заметно меньше WriteTimeout HTTP-сервера (см. main.go),
// чтобы клиент успел получить корректный 504 (WriteGRPCError маппит
// codes.DeadlineExceeded в http.StatusGatewayTimeout), а не обрыв соединения.
const upstreamTimeout = 5 * time.Second

// maxBodyBytes — верхняя граница размера тела PATCH /me. DTO крошечный
// (несколько строковых полей), 64 KiB — с большим запасом.
const maxBodyBytes = 64 * 1024

type UserHandler struct {
	client userv1.UserServiceClient
}

func NewUserHandler(client userv1.UserServiceClient) *UserHandler {
	return &UserHandler{
		client: client,
	}
}

func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	id, ok := UserIDFrom(r.Context())
	if !ok {
		WriteError(w, r, http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), upstreamTimeout)
	defer cancel()

	resp, err := h.client.GetUser(ctx, &userv1.GetUserRequest{Id: id.String()})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, toUserDTO(resp.GetUser()))
}

func (h *UserHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	id, ok := UserIDFrom(r.Context())
	if !ok {
		WriteError(w, r, http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var body updateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, r, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), upstreamTimeout)
	defer cancel()

	// Id — строго из токена. Тело запроса на то, чей это профиль, не влияет:
	// updateMeRequest поля id не имеет, а если клиент его прислал, json его
	// просто отбросит.
	resp, err := h.client.UpdateProfile(ctx, &userv1.UpdateProfileRequest{
		Id:     id.String(),
		Name:   body.Name,
		Handle: body.Handle,
		Bio:    body.Bio,
		City:   body.City,
		Work:   body.Work,
	})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, toUserDTO(resp.GetUser()))
}

func (h *UserHandler) UserByHandle(w http.ResponseWriter, r *http.Request) {
	handle := strings.TrimSpace(r.PathValue("handle"))
	if handle == "" {
		WriteError(w, r, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), upstreamTimeout)
	defer cancel()

	resp, err := h.client.GetUserByHandle(ctx, &userv1.GetUserByHandleRequest{Handle: handle})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, toUserDTO(resp.GetUser()))
}
