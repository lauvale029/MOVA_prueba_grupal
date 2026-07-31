// Package auth firma y valida JWT para la única credencial de servicio
// configurada (ver README, sección Autenticación).
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("token inválido o expirado")

type TokenService struct {
	secret     []byte
	expiration time.Duration
}

func NewTokenService(secret string, expirationMinutes int) *TokenService {
	return &TokenService{
		secret:     []byte(secret),
		expiration: time.Duration(expirationMinutes) * time.Minute,
	}
}

// GenerateToken firma un JWT con subject como identidad — es lo que
// después queda registrado como changed_by en el historial.
func (s *TokenService) GenerateToken(subject string) (token string, expiresAt time.Time, err error) {
	expiresAt = time.Now().UTC().Add(s.expiration)

	claims := jwt.RegisteredClaims{
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

// ValidateToken devuelve el subject si el token es válido y no expiró.
func (s *TokenService) ValidateToken(tokenString string) (string, error) {
	claims := &jwt.RegisteredClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return "", ErrInvalidToken
	}

	return claims.Subject, nil
}
