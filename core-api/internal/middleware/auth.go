package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/auth"
)

const localsSubjectKey = "subject"

// RequireAuth valida el header Authorization: Bearer <token> y guarda el
// subject en c.Locals para que los handlers lo usen como changed_by.
func RequireAuth(tokens *auth.TokenService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return unauthorized(c)
		}

		subject, err := tokens.ValidateToken(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			return unauthorized(c)
		}

		c.Locals(localsSubjectKey, subject)
		return c.Next()
	}
}

// Subject devuelve el subject guardado por RequireAuth.
func Subject(c *fiber.Ctx) string {
	subject, _ := c.Locals(localsSubjectKey).(string)
	return subject
}

func unauthorized(c *fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": fiber.Map{"code": "UNAUTHORIZED", "message": "falta el token, es inválido, o expiró"},
	})
}
