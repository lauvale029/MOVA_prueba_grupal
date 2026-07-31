package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/auth"
)

type AuthHandler struct {
	tokens   *auth.TokenService
	username string
	password string
}

func NewAuthHandler(tokens *auth.TokenService, username, password string) *AuthHandler {
	return &AuthHandler{tokens: tokens, username: username, password: password}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login valida la única credencial de servicio configurada (no hay
// tabla de usuarios — ver README, sección Autenticación).
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, "INVALID_REQUEST_BODY", "el cuerpo de la petición no es un JSON válido")
	}

	if req.Username != h.username || req.Password != h.password {
		return errorResponse(c, fiber.StatusUnauthorized, "INVALID_CREDENTIALS", "usuario o contraseña incorrectos")
	}

	token, expiresAt, err := h.tokens.GenerateToken(req.Username)
	if err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "error interno")
	}

	return c.JSON(fiber.Map{
		"token":      token,
		"expires_at": expiresAt.Format(timeFormat),
	})
}
