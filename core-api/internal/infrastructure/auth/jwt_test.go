package auth_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/auth"
)

func TestGenerateAndValidateToken_RoundTrip(t *testing.T) {
	svc := auth.NewTokenService("test-secret", 60)

	token, expiresAt, err := svc.GenerateToken("mova-service")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.True(t, expiresAt.After(time.Now().UTC()))

	subject, err := svc.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "mova-service", subject)
}

func TestValidateToken_WrongSecret_Fails(t *testing.T) {
	svc := auth.NewTokenService("secret-1", 60)
	other := auth.NewTokenService("secret-2", 60)

	token, _, err := svc.GenerateToken("mova-service")
	require.NoError(t, err)

	_, err = other.ValidateToken(token)
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestValidateToken_Expired_Fails(t *testing.T) {
	svc := auth.NewTokenService("test-secret", 0) // expira de inmediato

	token, _, err := svc.GenerateToken("mova-service")
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	_, err = svc.ValidateToken(token)
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestValidateToken_Garbage_Fails(t *testing.T) {
	svc := auth.NewTokenService("test-secret", 60)

	_, err := svc.ValidateToken("no-es-un-jwt")
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}
