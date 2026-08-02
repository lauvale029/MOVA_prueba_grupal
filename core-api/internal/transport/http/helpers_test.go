package http_test

import (
	"context"
	"sync"

	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
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

type fakeMerchantRepository struct {
	mu       sync.Mutex
	byID     map[string]*domain.Merchant
	byDocNum map[string]string
}

func newFakeMerchantRepository() *fakeMerchantRepository {
	return &fakeMerchantRepository{
		byID:     make(map[string]*domain.Merchant),
		byDocNum: make(map[string]string),
	}
}

func (r *fakeMerchantRepository) Create(_ context.Context, m *domain.Merchant) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byDocNum[m.DocumentNumber]; exists {
		return application.ErrConflict
	}
	cp := *m
	r.byID[m.ID] = &cp
	r.byDocNum[m.DocumentNumber] = m.ID
	return nil
}

func (r *fakeMerchantRepository) GetByID(_ context.Context, id string) (*domain.Merchant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.byID[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	cp := *m
	return &cp, nil
}

type testApp struct {
	app       *fiber.App
	service   *application.PaymentIntentService
	payments  *memory.PaymentIntentRepository
	merchants *fakeMerchantRepository
	token     string // JWT válido, listo para usar en Authorization: Bearer
}

func setupApp() *testApp {
	payments := memory.NewPaymentIntentRepository()
	history := memory.NewPaymentIntentStatusHistoryRepository()
	service := application.NewPaymentIntentService(payments, history, noopLocker{}, memory.UnitOfWork{}, noopPublisher{})

	merchants := newFakeMerchantRepository()
	merchantService := application.NewMerchantService(merchants)

	tokens := auth.NewTokenService("test-secret", 60)
	token, _, _ := tokens.GenerateToken(testAuthUsername)

	paymentHandler := transporthttp.NewPaymentIntentHandler(service)
	merchantHandler := transporthttp.NewMerchantHandler(merchantService)
	readinessHandler := transporthttp.NewReadinessHandler("localhost:0") // no se usa en estos tests
	authHandler := transporthttp.NewAuthHandler(tokens, testAuthUsername, testAuthPassword)

	return &testApp{
		app:       transporthttp.NewRouter(paymentHandler, merchantHandler, readinessHandler, authHandler, tokens),
		service:   service,
		payments:  payments,
		merchants: merchants,
		token:     token,
	}
}

type noopLocker struct{}

func (noopLocker) Acquire(_ context.Context, _ string) (func(), bool) { return func() {}, true }
