package http

import "github.com/gofiber/fiber/v2"

// NewRouter arma la app Fiber. El middleware de autenticación se agrega
// en el PR de auth (ver TODO en changedBy, auth_context.go).
func NewRouter(paymentIntentHandler *PaymentIntentHandler, readinessHandler *ReadinessHandler) *fiber.App {
	app := fiber.New()

	app.Get("/readiness", readinessHandler.Ready)

	v1 := app.Group("/api/v1")
	v1.Post("/payment-intents", paymentIntentHandler.Create)
	v1.Get("/payment-intents", paymentIntentHandler.List)
	v1.Get("/payment-intents/:payment_intent_id", paymentIntentHandler.Get)
	v1.Get("/payment-intents/:payment_intent_id/history", paymentIntentHandler.History)

	return app
}
