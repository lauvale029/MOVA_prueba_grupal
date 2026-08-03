package http

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	kafkago "github.com/segmentio/kafka-go"
)

// pgPinger es lo mínimo que necesita este handler de Postgres — lo
// cumple *pgxpool.Pool sin que este paquete dependa de pgx directamente.
type pgPinger interface {
	Ping(ctx context.Context) error
}

type ReadinessHandler struct {
	kafkaBroker string
	pg          pgPinger
}

func NewReadinessHandler(kafkaBroker string, pg pgPinger) *ReadinessHandler {
	return &ReadinessHandler{kafkaBroker: kafkaBroker, pg: pg}
}

// Ready confirma que las dependencias externas de las que el sistema no
// puede seguir sin (Kafka, Postgres) son alcanzables. Redis queda
// afuera a propósito: es un lock best-effort (ver ADR-0002), que esté
// caído no debería tumbar el healthcheck.
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

	if err := h.pg.Ping(ctx); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "not_ready",
			"reason": "postgres unreachable",
		})
	}

	return c.JSON(fiber.Map{"status": "ready"})
}
