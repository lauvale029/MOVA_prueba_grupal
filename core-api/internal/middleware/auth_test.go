package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/auth"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/middleware"
)

func setupApp(tokens *auth.TokenService) *fiber.App {
	app := fiber.New()
	app.Get("/protegido", middleware.RequireAuth(tokens), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"subject": middleware.Subject(c)})
	})
	return app
}

func TestRequireAuth_ValidToken_Allows(t *testing.T) {
	tokens := auth.NewTokenService("test-secret", 60)
	app := setupApp(tokens)
	token, _, err := tokens.GenerateToken("mova-service")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/protegido", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRequireAuth_MissingToken_Returns401(t *testing.T) {
	tokens := auth.NewTokenService("test-secret", 60)
	app := setupApp(tokens)

	req := httptest.NewRequest(http.MethodGet, "/protegido", nil)
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestRequireAuth_InvalidToken_Returns401(t *testing.T) {
	tokens := auth.NewTokenService("test-secret", 60)
	app := setupApp(tokens)

	req := httptest.NewRequest(http.MethodGet, "/protegido", nil)
	req.Header.Set("Authorization", "Bearer token-invalido")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
