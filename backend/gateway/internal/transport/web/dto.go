package web

import (
	"time"

	userv1 "ahishka/pkg/gen/ahishka/user/v1"
)

// userDTO — форма пользователя наружу. Отдельный тип, а не protojson поверх
// userv1.User: REST-контракт для фронта не должен ездить следом за
// внутренним proto-контрактом.
type userDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Handle    string    `json:"handle"`
	Bio       string    `json:"bio"`
	City      string    `json:"city"`
	Work      string    `json:"work"`
	Verified  bool      `json:"verified"`
	AvatarURL *string   `json:"avatarUrl"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func toUserDTO(user *userv1.User) userDTO {
	// user == nil is a contract violation (err == nil should always come with
	// a populated User) but not impossible from a misbehaving upstream — stay
	// nil-safe throughout rather than panicking on a bad response.
	if user == nil {
		return userDTO{}
	}
	return userDTO{
		ID:        user.GetId(),
		Name:      user.GetName(),
		Handle:    user.GetHandle(),
		Bio:       user.GetBio(),
		City:      user.GetCity(),
		Work:      user.GetWork(),
		Verified:  user.GetVerified(),
		AvatarURL: user.AvatarUrl,
		CreatedAt: user.GetCreatedAt().AsTime(),
		UpdatedAt: user.GetUpdatedAt().AsTime(),
	}
}

// updateMeRequest — тело PATCH /me. Все поля указатели: отсутствующее поле
// означает «не менять» и уезжает в gRPC как nil (partial update).
//
// Поля id тут нет намеренно и его нельзя добавлять: кого правим, решает
// только JWT, а не клиент.
type updateMeRequest struct {
	Name   *string `json:"name"`
	Handle *string `json:"handle"`
	Bio    *string `json:"bio"`
	City   *string `json:"city"`
	Work   *string `json:"work"`
}
