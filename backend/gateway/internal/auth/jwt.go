package auth

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ErrInvalidToken — единая причина отказа для всех случаев: битый токен,
// просроченный, подписанный чужим ключом, с неожиданным алгоритмом или с
// нечитаемым sub. Наружу разница не выдаётся намеренно — она подсказала бы
// атакующему, насколько он близок.
var ErrInvalidToken = errors.New("invalid token")

type Verifier struct {
	secret []byte
}

func NewVerifier(secret string) *Verifier {
	return &Verifier{
		secret: []byte(secret),
	}
}

// UserIDFromToken проверяет подпись и срок локально, без похода в auth-service,
// и возвращает идентификатор пользователя из sub.
func (v *Verifier) UserIDFromToken(raw string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{},
		func(token *jwt.Token) (interface{}, error) {
			// Без этой проверки токен с alg=none или с подменённым на
			// асимметричный алгоритмом прошёл бы валидацию.
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return v.secret, nil
		})
	if err != nil || !token.Valid {
		return uuid.Nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok {
		return uuid.Nil, ErrInvalidToken
	}

	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, ErrInvalidToken
	}
	return id, nil
}
