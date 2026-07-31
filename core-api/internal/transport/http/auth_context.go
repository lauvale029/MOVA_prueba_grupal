package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/middleware"
)

// changedBy identifica quién origina el cambio, para el historial —
// el subject del JWT autenticado (ver middleware.RequireAuth).
func changedBy(c *fiber.Ctx) string {
	return middleware.Subject(c)
}
