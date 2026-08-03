package http

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler sirve /metrics en el formato que espera Prometheus.
func MetricsHandler(registry *prometheus.Registry) fiber.Handler {
	return adaptor.HTTPHandler(promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
}
