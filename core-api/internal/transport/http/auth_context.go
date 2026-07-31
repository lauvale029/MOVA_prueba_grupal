package http

import "github.com/gofiber/fiber/v2"

// changedBy identifica quién origina el cambio, para el historial.
//
// TODO(auth): hoy devuelve un valor fijo porque el middleware JWT todavía
// no existe (llega en el PR de autenticación). Cuando exista, este
// helper pasa a leer el subject del token desde c.Locals — los handlers
// no cambian.
func changedBy(_ *fiber.Ctx) string {
	return "core-api"
}
