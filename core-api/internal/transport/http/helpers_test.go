package http_test

import (
	"context"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/auth"
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

type fakePaymentIntentRepository struct {
	mu        sync.Mutex
	byID      map[string]*domain.PaymentIntent
	byIdemKey map[string]string
}

func newFakePaymentIntentRepository() *fakePaymentIntentRepository {
	return &fakePaymentIntentRepository{
		byID:      make(map[string]*domain.PaymentIntent),
		byIdemKey: make(map[string]string),
	}
}

func copyPI(pi *domain.PaymentIntent) *domain.PaymentIntent {
	cp := *pi
	return &cp
}

func (r *fakePaymentIntentRepository) Create(_ context.Context, pi *domain.PaymentIntent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byIdemKey[pi.IdempotencyKey]; exists {
		return application.ErrConflict
	}
	r.byID[pi.ID] = copyPI(pi)
	r.byIdemKey[pi.IdempotencyKey] = pi.ID
	return nil
}

func (r *fakePaymentIntentRepository) GetByID(_ context.Context, id string) (*domain.PaymentIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pi, ok := r.byID[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	return copyPI(pi), nil
}

func (r *fakePaymentIntentRepository) GetByIdempotencyKey(_ context.Context, key string) (*domain.PaymentIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byIdemKey[key]
	if !ok {
		return nil, application.ErrNotFound
	}
	return copyPI(r.byID[id]), nil
}

func (r *fakePaymentIntentRepository) Update(_ context.Context, pi *domain.PaymentIntent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[pi.ID]; !ok {
		return application.ErrNotFound
	}
	r.byID[pi.ID] = copyPI(pi)
	return nil
}

func (r *fakePaymentIntentRepository) List(_ context.Context, filter application.PaymentIntentFilter) ([]*domain.PaymentIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.filtered(filter), nil
}

func (r *fakePaymentIntentRepository) Count(_ context.Context, filter application.PaymentIntentFilter) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.filtered(filter))), nil
}

func (r *fakePaymentIntentRepository) CountRecentByMerchant(_ context.Context, merchantID string, since time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var count int64
	for _, pi := range r.byID {
		if pi.MerchantID == merchantID && !pi.CreatedAt.Before(since) {
			count++
		}
	}
	return count, nil
}

// filtered asume el lock ya tomado por quien llama.
func (r *fakePaymentIntentRepository) filtered(filter application.PaymentIntentFilter) []*domain.PaymentIntent {
	items := make([]*domain.PaymentIntent, 0, len(r.byID))
	for _, pi := range r.byID {
		if filter.MerchantID != "" && pi.MerchantID != filter.MerchantID {
			continue
		}
		if filter.Status != "" && pi.Status != filter.Status {
			continue
		}
		items = append(items, copyPI(pi))
	}
	return items
}

type fakeHistoryRepository struct {
	mu      sync.Mutex
	entries []*domain.PaymentIntentStatusHistory
}

func (r *fakeHistoryRepository) Create(_ context.Context, h *domain.PaymentIntentStatusHistory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, h)
	return nil
}

func (r *fakeHistoryRepository) ListByPaymentIntentID(_ context.Context, id string) ([]*domain.PaymentIntentStatusHistory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.PaymentIntentStatusHistory
	for _, h := range r.entries {
		if h.PaymentIntentID == id {
			out = append(out, h)
		}
	}
	return out, nil
}

// fakeUnitOfWork no abre una transacción real — basta para pruebas de HTTP
// que no tocan Postgres (ver postgres.UnitOfWork para la versión real).
type fakeUnitOfWork struct{}

func (fakeUnitOfWork) Execute(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type testApp struct {
	app       *fiber.App
	service   *application.PaymentIntentService
	payments  *fakePaymentIntentRepository
	merchants *fakeMerchantRepository
	token     string // JWT válido, listo para usar en Authorization: Bearer
}

func setupApp() *testApp {
	payments := newFakePaymentIntentRepository()
	history := &fakeHistoryRepository{}
	merchants := newFakeMerchantRepository()
	// Comercios que ya asumen los tests de payment intents (merchant_id
	// fijo en el body/query) — desde que Create valida que el comercio
	// exista, hace falta sembrarlos.
	_ = merchants.Create(context.Background(), &domain.Merchant{ID: "merchant-1", DocumentNumber: "seed-1", Status: domain.MerchantStatusActive})
	_ = merchants.Create(context.Background(), &domain.Merchant{ID: "merchant-2", DocumentNumber: "seed-2", Status: domain.MerchantStatusActive})
	service := application.NewPaymentIntentService(payments, history, merchants, noopLocker{}, fakeUnitOfWork{}, noopPublisher{})

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
