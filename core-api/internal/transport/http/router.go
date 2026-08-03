package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/auth"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/metrics"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/middleware"
)

// NewRouter arma la app Fiber. /auth/login, /readiness y /docs son
// públicas; todo lo demás exige un JWT válido (ver middleware.RequireAuth).
func NewRouter(paymentIntentHandler *PaymentIntentHandler, merchantHandler *MerchantHandler, readinessHandler *ReadinessHandler, authHandler *AuthHandler, docsHandler *DocsHandler, tokens *auth.TokenService) *fiber.App {
	app := fiber.New()

	app.Get("/health", Health)
	app.Get("/metrics", MetricsHandler(metrics.Registry))
	app.Get("/readiness", readinessHandler.Ready)
	app.Get("/docs", docsHandler.UI)
	app.Get("/docs/openapi.yaml", docsHandler.Spec)
	app.Post("/api/v1/auth/login", authHandler.Login)

	protected := app.Group("/api/v1", middleware.RequireAuth(tokens))
	protected.Post("/merchants", merchantHandler.Create)
	protected.Get("/merchants/:merchant_id", merchantHandler.Get)

	protected.Post("/payment-intents", paymentIntentHandler.Create)
	protected.Get("/payment-intents", paymentIntentHandler.List)
	protected.Get("/payment-intents/:payment_intent_id", paymentIntentHandler.Get)
	protected.Get("/payment-intents/:payment_intent_id/history", paymentIntentHandler.History)
	protected.Patch("/payment-intents/:payment_intent_id/status", paymentIntentHandler.UpdateStatus)

	return app
}
