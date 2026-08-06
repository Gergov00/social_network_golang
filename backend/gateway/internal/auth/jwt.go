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

// keyFunc отдаёт ключ для проверки подписи jwt.ParseWithClaims. Проверка
// алгоритма — defense-in-depth: сейчас у нас единственный статический
// HMAC-секрет, и сама библиотека уже отказывает alg=none/RSA-ключам, потому
// что типы ключей не совпадают (Verify для none требует особую внутреннюю
// константу, для RSA — *rsa.PublicKey, а мы всегда возвращаем []byte). Но
// как только keyFunc научится резолвить ключ по алгоритму (например, при
// переходе на RS256+JWKS из роадмапа), эта случайная защита библиотеки
// исчезнет — и вот тогда явная проверка ниже станет единственным барьером.
// Оставляем её сейчас, чтобы не потерять при будущем рефакторинге.
func (v *Verifier) keyFunc(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	}
	return v.secret, nil
}

// UserIDFromToken проверяет подпись и срок локально, без похода в auth-service,
// и возвращает идентификатор пользователя из sub.
func (v *Verifier) UserIDFromToken(raw string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{}, v.keyFunc)
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
