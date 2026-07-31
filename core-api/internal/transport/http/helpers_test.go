package http_test

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/auth"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/memory"
	transporthttp "github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/transport/http"
)

const (
	testAuthUsername = "mova-service"
	testAuthPassword = "test-password"
)

type noopPublisher struct{}

func (noopPublisher) Publish(_ context.Context, _ application.RiskEvaluationRequested) error {
	return nil
}

type testApp struct {
	app      *fiber.App
	service  *application.PaymentIntentService
	payments *memory.PaymentIntentRepository
	token    string // JWT válido, listo para usar en Authorization: Bearer
}

func setupApp() *testApp {
	payments := memory.NewPaymentIntentRepository()
	history := memory.NewPaymentIntentStatusHistoryRepository()
	service := application.NewPaymentIntentService(payments, history, noopLocker{}, memory.UnitOfWork{}, noopPublisher{})

	tokens := auth.NewTokenService("test-secret", 60)
	token, _, _ := tokens.GenerateToken(testAuthUsername)

	paymentHandler := transporthttp.NewPaymentIntentHandler(service)
	readinessHandler := transporthttp.NewReadinessHandler("localhost:0") // no se usa en estos tests
	authHandler := transporthttp.NewAuthHandler(tokens, testAuthUsername, testAuthPassword)

	return &testApp{
		app:      transporthttp.NewRouter(paymentHandler, readinessHandler, authHandler, tokens),
		service:  service,
		payments: payments,
		token:    token,
	}
}

type noopLocker struct{}

func (noopLocker) Acquire(_ context.Context, _ string) (func(), bool) { return func() {}, true }
