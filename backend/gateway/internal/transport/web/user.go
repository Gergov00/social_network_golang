package web

import (
	"encoding/json"
	"net/http"
	"strings"

	userv1 "ahishka/pkg/gen/ahishka/user/v1"
)

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

	resp, err := h.client.GetUser(r.Context(), &userv1.GetUserRequest{Id: id.String()})
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

	var body updateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, r, http.StatusBadRequest)
		return
	}

	// Id — строго из токена. Тело запроса на то, чей это профиль, не влияет:
	// updateMeRequest поля id не имеет, а если клиент его прислал, json его
	// просто отбросит.
	resp, err := h.client.UpdateProfile(r.Context(), &userv1.UpdateProfileRequest{
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

	resp, err := h.client.GetUserByHandle(r.Context(), &userv1.GetUserByHandleRequest{Handle: handle})
	if err != nil {
		WriteGRPCError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, toUserDTO(resp.GetUser()))
}
