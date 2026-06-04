package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	issuer     string
}

type AccessTokenClaims struct {
	jwt.RegisteredClaims
}

func NewManager(secret string, accessTTL, refreshTTL time.Duration, issuer string) *Manager {
	return &Manager{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		issuer:     issuer,
	}
}

func (m *Manager) NewAccessToken(userID uuid.UUID) (string, error) {
	claims := AccessTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.accessTTL)),
			Issuer:    m.issuer,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *Manager) ParseAccessToken(raw string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(raw, &AccessTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	if claims, ok := token.Claims.(*AccessTokenClaims); ok && token.Valid {
		return uuid.Parse(claims.Subject)
	}
	return uuid.Nil, fmt.Errorf("invalid token")
}

func (m *Manager) NewRefreshToken() (raw string, hash string, err error) {
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", "", err
	}
	raw = base64.URLEncoding.EncodeToString(rawBytes)
	hash = HashRefreshToken(raw)
	return
}

func HashRefreshToken(raw string) (hash string) {
	hashed := sha256.Sum256([]byte(raw))
	return base64.URLEncoding.EncodeToString(hashed[:])
}
