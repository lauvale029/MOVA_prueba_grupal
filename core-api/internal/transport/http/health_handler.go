package http

import "github.com/gofiber/fiber/v2"

// Health confirma que el proceso vive. No toca ninguna dependencia a
// propósito — para eso está /readiness.
func Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}
