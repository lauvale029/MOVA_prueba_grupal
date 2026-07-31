package http

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	kafkago "github.com/segmentio/kafka-go"
)

type ReadinessHandler struct {
	kafkaBroker string
}

func NewReadinessHandler(kafkaBroker string) *ReadinessHandler {
	return &ReadinessHandler{kafkaBroker: kafkaBroker}
}

// Ready confirma que las dependencias externas EN USO hoy (Kafka) son
// alcanzables. Postgres/Redis se suman acá cuando dejen de ser
// implementaciones en memoria
func (h *ReadinessHandler) Ready(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
	defer cancel()

	conn, err := kafkago.DialContext(ctx, "tcp", h.kafkaBroker)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "not_ready",
			"reason": "kafka unreachable",
		})
	}
	defer conn.Close()

	return c.JSON(fiber.Map{"status": "ready"})
}
