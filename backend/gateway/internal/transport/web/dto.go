package web

import (
	"time"

	postv1 "ahishka/pkg/gen/ahishka/post/v1"
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

// postDTO — форма поста наружу. Author подставляет gateway из user-service:
// post-service хранит только author_id (database-per-service), склейка
// двух источников в один экран — это и есть работа BFF.
//
// Author == nil — законное состояние: профиль автора мог не найтись или
// user-service мог не ответить. Фронт обязан это пережить (нарисовать
// заглушку), а не считать ошибкой.
type postDTO struct {
	ID        string    `json:"id"`
	AuthorID  string    `json:"authorId"`
	Author    *userDTO  `json:"author"`
	Text      string    `json:"text"`
	Likes     int32     `json:"likes"`
	Liked     bool      `json:"liked"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// feedResponse — страница ленты. NextCursor пустой, если страниц больше нет.
type feedResponse struct {
	Posts      []postDTO `json:"posts"`
	NextCursor string    `json:"nextCursor"`
}

func toPostDTO(post *postv1.Post, authors map[string]userDTO) postDTO {
	if post == nil {
		return postDTO{}
	}

	dto := postDTO{
		ID:        post.GetId(),
		AuthorID:  post.GetAuthorId(),
		Text:      post.GetText(),
		Likes:     post.GetLikesCount(),
		Liked:     post.GetLiked(),
		CreatedAt: post.GetCreatedAt().AsTime(),
		UpdatedAt: post.GetUpdatedAt().AsTime(),
	}
	if author, ok := authors[post.GetAuthorId()]; ok {
		dto.Author = &author
	}
	return dto
}

// createPostRequest — тело POST /posts. Поля автора тут нет и его нельзя
// добавлять: кто автор, решает только JWT (см. withCaller).
type createPostRequest struct {
	Text string `json:"text"`
}
