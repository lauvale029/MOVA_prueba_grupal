package application_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

// --- fakes ---

type inMemoryPaymentIntentRepository struct {
	mu        sync.Mutex
	byID      map[string]*domain.PaymentIntent
	byIdemKey map[string]string
}

func newInMemoryPaymentIntentRepository() *inMemoryPaymentIntentRepository {
	return &inMemoryPaymentIntentRepository{
		byID:      make(map[string]*domain.PaymentIntent),
		byIdemKey: make(map[string]string),
	}
}

func copyPI(pi *domain.PaymentIntent) *domain.PaymentIntent {
	cp := *pi
	return &cp
}

func (r *inMemoryPaymentIntentRepository) Create(_ context.Context, pi *domain.PaymentIntent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byIdemKey[pi.IdempotencyKey]; exists {
		return application.ErrConflict
	}
	r.byID[pi.ID] = copyPI(pi)
	r.byIdemKey[pi.IdempotencyKey] = pi.ID
	return nil
}

func (r *inMemoryPaymentIntentRepository) GetByID(_ context.Context, id string) (*domain.PaymentIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pi, ok := r.byID[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	return copyPI(pi), nil
}

func (r *inMemoryPaymentIntentRepository) GetByIdempotencyKey(_ context.Context, key string) (*domain.PaymentIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byIdemKey[key]
	if !ok {
		return nil, application.ErrNotFound
	}
	return copyPI(r.byID[id]), nil
}

func (r *inMemoryPaymentIntentRepository) Update(_ context.Context, pi *domain.PaymentIntent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[pi.ID]; !ok {
		return application.ErrNotFound
	}
	r.byID[pi.ID] = copyPI(pi)
	return nil
}

func (r *inMemoryPaymentIntentRepository) List(_ context.Context, _ application.PaymentIntentFilter) ([]*domain.PaymentIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]*domain.PaymentIntent, 0, len(r.byID))
	for _, pi := range r.byID {
		items = append(items, copyPI(pi))
	}
	return items, nil
}

func (r *inMemoryPaymentIntentRepository) Count(_ context.Context, _ application.PaymentIntentFilter) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.byID)), nil
}

func (r *inMemoryPaymentIntentRepository) CountRecentByMerchant(_ context.Context, merchantID string, since time.Time) (int64, error) {
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

func (r *inMemoryPaymentIntentRepository) rowCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byID)
}

type inMemoryHistoryRepository struct {
	mu      sync.Mutex
	entries []*domain.PaymentIntentStatusHistory
}

func (r *inMemoryHistoryRepository) Create(_ context.Context, h *domain.PaymentIntentStatusHistory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, h)
	return nil
}

func (r *inMemoryHistoryRepository) ListByPaymentIntentID(_ context.Context, id string) ([]*domain.PaymentIntentStatusHistory, error) {
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

// fakeMerchantRepository devuelve un comercio ACTIVE por defecto para
// cualquier id no sembrado explícitamente: a la mayoría de estos tests no
// les importa el comercio, solo el flujo de PaymentIntent.
type fakeMerchantRepository struct {
	mu   sync.Mutex
	byID map[string]*domain.Merchant
}

func newFakeMerchantRepository() *fakeMerchantRepository {
	return &fakeMerchantRepository{byID: make(map[string]*domain.Merchant)}
}

func (r *fakeMerchantRepository) Create(_ context.Context, m *domain.Merchant) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[m.ID] = m
	return nil
}

func (r *fakeMerchantRepository) GetByID(_ context.Context, id string) (*domain.Merchant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.byID[id]; ok {
		return m, nil
	}
	return &domain.Merchant{ID: id, Status: domain.MerchantStatusActive}, nil
}

type fakeUnitOfWork struct{}

func (fakeUnitOfWork) Execute(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeLocker struct{}

func (fakeLocker) Acquire(_ context.Context, _ string) (func(), bool) {
	return func() {}, true
}

type fakeRiskPublisher struct {
	mu     sync.Mutex
	events []application.RiskEvaluationRequested
	// result, si no es nil, simula el camino HTTP directo del circuit
	// breaker (ADR-0008): Publish devuelve la decisión ya resuelta, en
	// vez de nil (que es lo que pasa cuando el evento se publicó bien a
	// Kafka y la decisión llega después, de forma asíncrona).
	result *application.RiskEvaluationResult
}

func (p *fakeRiskPublisher) Publish(_ context.Context, event application.RiskEvaluationRequested) (*application.RiskEvaluationResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
	return p.result, nil
}

func (p *fakeRiskPublisher) publishCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

func newServiceForTest() (*application.PaymentIntentService, *inMemoryPaymentIntentRepository, *inMemoryHistoryRepository, *fakeMerchantRepository, *fakeRiskPublisher) {
	payments := newInMemoryPaymentIntentRepository()
	history := &inMemoryHistoryRepository{}
	merchants := newFakeMerchantRepository()
	publisher := &fakeRiskPublisher{}
	svc := application.NewPaymentIntentService(payments, history, merchants, fakeLocker{}, fakeUnitOfWork{}, publisher)
	return svc, payments, history, merchants, publisher
}

// --- tests ---

func TestCreate_Valid(t *testing.T) {
	svc, _, history, _, publisher := newServiceForTest()

	pi, err := svc.Create(context.Background(), "merchant-1", "order-1", 15_000_00, "COP", domain.ChannelQR, "idem-1", "", "mova-service")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusUnderReview, pi.Status, "todo intent pasa por UNDER_REVIEW al enviarse a riesgo")
	assert.Equal(t, 1, publisher.publishCount())

	entries, _ := history.ListByPaymentIntentID(context.Background(), pi.ID)
	require.Len(t, entries, 2, "creado + enviado a revisión")
	assert.Equal(t, domain.StatusPending, entries[0].NewStatus)
	assert.Equal(t, domain.StatusUnderReview, entries[1].NewStatus)
}

// TestCreate_AppliesImmediateResult_WhenPublisherFallsBackToHTTP cierra
// el ADR-0008: si Kafka mismo falló, RiskRequestPublisher.Publish
// resuelve la decisión ya, en el mismo request (circuit breaker → HTTP
// directo), y Create debe aplicarla de inmediato en vez de esperar un
// evento que nunca se va a publicar.
func TestCreate_AppliesImmediateResult_WhenPublisherFallsBackToHTTP(t *testing.T) {
	svc, _, history, _, publisher := newServiceForTest()
	publisher.result = &application.RiskEvaluationResult{
		Decision: domain.RiskApprove, Score: 5, ReasonCodes: []string{"LOW_RISK"}, ModelVersion: "rules-v2",
	}

	pi, err := svc.Create(context.Background(), "merchant-1", "order-1", 15_000_00, "COP", domain.ChannelQR, "idem-1", "", "mova-service")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusApproved, pi.Status, "se resuelve ya, sin esperar un evento de Kafka que nunca se publicó")
	require.NotNil(t, pi.RiskDecision)
	assert.Equal(t, domain.RiskApprove, *pi.RiskDecision)

	entries, _ := history.ListByPaymentIntentID(context.Background(), pi.ID)
	require.Len(t, entries, 3, "creado + enviado a revisión + resultado de riesgo")
	assert.Equal(t, domain.StatusApproved, entries[2].NewStatus)
	assert.Contains(t, entries[2].Reason, "HTTP directo", "el historial debe distinguir el camino de emergencia del normal por Kafka")
}

// TestCreate_PublishesMerchantStatusAndRecentIntents cierra el issue #14:
// el evento de riesgo debe traer el estado del comercio y cuántos intents
// recientes tiene — sin contarse a sí mismo (ver ADR-0004).
func TestCreate_PublishesMerchantStatusAndRecentIntents(t *testing.T) {
	svc, _, _, merchants, publisher := newServiceForTest()
	ctx := context.Background()

	merchant := &domain.Merchant{ID: "merchant-inactive", Status: domain.MerchantStatusInactive}
	require.NoError(t, merchants.Create(ctx, merchant))

	_, err := svc.Create(ctx, merchant.ID, "order-1", 1000, "COP", domain.ChannelQR, "idem-1", "", "mova-service")
	require.NoError(t, err)
	_, err = svc.Create(ctx, merchant.ID, "order-2", 1000, "COP", domain.ChannelQR, "idem-2", "", "mova-service")
	require.NoError(t, err)

	require.Len(t, publisher.events, 2)
	assert.Equal(t, "INACTIVE", publisher.events[0].MerchantStatus)
	assert.Equal(t, 0, publisher.events[0].MerchantRecentIntents, "el primer intent del comercio no se cuenta a sí mismo")
	assert.Equal(t, "INACTIVE", publisher.events[1].MerchantStatus)
	assert.Equal(t, 1, publisher.events[1].MerchantRecentIntents, "ya existía el primer intent al crear el segundo")
}

func TestCreate_MissingIdempotencyKey(t *testing.T) {
	svc, _, _, _, _ := newServiceForTest()

	_, err := svc.Create(context.Background(), "merchant-1", "order-1", 1000, "COP", domain.ChannelQR, "", "", "mova-service")
	assert.ErrorIs(t, err, domain.ErrMissingIdempotencyKey)
}

func TestCreate_Retry_ReturnsSameIntent_DoesNotPublishAgain(t *testing.T) {
	svc, payments, _, _, publisher := newServiceForTest()
	ctx := context.Background()

	first, err := svc.Create(ctx, "merchant-1", "order-1", 1000, "COP", domain.ChannelQR, "idem-1", "", "mova-service")
	require.NoError(t, err)

	second, err := svc.Create(ctx, "merchant-1", "order-1", 1000, "COP", domain.ChannelQR, "idem-1", "", "mova-service")
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, 1, payments.rowCount())
	assert.Equal(t, 1, publisher.publishCount())
}

func TestCreate_Concurrent_OnlyOneRowCreated(t *testing.T) {
	svc, payments, _, _, _ := newServiceForTest()
	const attempts = 20
	var wg sync.WaitGroup
	errs := make([]error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = svc.Create(context.Background(), "merchant-1", "order-1", 1000, "COP", domain.ChannelQR, "misma-key", "", "mova-service")
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, payments.rowCount())
}

func TestApplyRiskResult_Approve(t *testing.T) {
	svc, _, history, _, _ := newServiceForTest()
	ctx := context.Background()

	pi, err := svc.Create(ctx, "merchant-1", "order-1", 1000, "COP", domain.ChannelQR, "idem-1", "", "mova-service")
	require.NoError(t, err)

	resolved, err := svc.ApplyRiskResult(ctx, pi.ID, domain.RiskApprove, 10, nil, "rules-v1", "risk-service")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusApproved, resolved.Status)

	entries, _ := history.ListByPaymentIntentID(ctx, pi.ID)
	assert.Len(t, entries, 3, "creado + enviado a revisión + resultado de riesgo")
}

func TestApplyRiskResult_Review_StaysUnderReview(t *testing.T) {
	svc, _, history, _, _ := newServiceForTest()
	ctx := context.Background()

	pi, err := svc.Create(ctx, "merchant-1", "order-1", 1000, "COP", domain.ChannelQR, "idem-1", "", "mova-service")
	require.NoError(t, err)

	resolved, err := svc.ApplyRiskResult(ctx, pi.ID, domain.RiskReview, 60, []string{"AMOUNT_ABOVE_THRESHOLD"}, "rules-v1", "risk-service")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusUnderReview, resolved.Status)

	// Aunque el estado no cambia, se registra el reintento de evaluación.
	entries, _ := history.ListByPaymentIntentID(ctx, pi.ID)
	assert.Len(t, entries, 3)
}

func TestApplyRiskResult_UnknownIntent(t *testing.T) {
	svc, _, _, _, _ := newServiceForTest()
	_, err := svc.ApplyRiskResult(context.Background(), "no-existe", domain.RiskApprove, 10, nil, "rules-v1", "risk-service")
	assert.ErrorIs(t, err, application.ErrNotFound)
}

// TestUpdateStatus_PendingToExpired_Success cierra el issue #11: es la
// transición que usa el reconciliation-worker para vencer intents.
func TestUpdateStatus_PendingToExpired_Success(t *testing.T) {
	svc, payments, history, _, _ := newServiceForTest()
	ctx := context.Background()

	// Sembrado directo en el repo (no vía svc.Create) para quedar en
	// PENDING: Create mueve a UNDER_REVIEW de inmediato.
	pi, err := domain.NewPaymentIntent("merchant-1", "order-1", 1000, "COP", domain.ChannelQR, "idem-1", "")
	require.NoError(t, err)
	require.NoError(t, payments.Create(ctx, pi))

	updated, err := svc.UpdateStatus(ctx, pi.ID, domain.StatusExpired, "vencido sin resolverse", "reconciliation-worker")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusExpired, updated.Status)

	entries, _ := history.ListByPaymentIntentID(ctx, pi.ID)
	require.Len(t, entries, 1)
	assert.Equal(t, domain.StatusPending, entries[0].PreviousStatus)
	assert.Equal(t, domain.StatusExpired, entries[0].NewStatus)
	assert.Equal(t, "reconciliation-worker", entries[0].ChangedBy)
}

func TestUpdateStatus_AlreadyAtTarget_ReturnsConflict(t *testing.T) {
	svc, _, _, _, _ := newServiceForTest()
	ctx := context.Background()

	pi, err := svc.Create(ctx, "merchant-1", "order-1", 1000, "COP", domain.ChannelQR, "idem-1", "", "mova-service")
	require.NoError(t, err) // ya queda en UNDER_REVIEW

	_, err = svc.UpdateStatus(ctx, pi.ID, domain.StatusUnderReview, "reintento", "reconciliation-worker")
	assert.ErrorIs(t, err, application.ErrConflict)
}

func TestUpdateStatus_InvalidTransition_ReturnsUnprocessable(t *testing.T) {
	svc, _, _, _, _ := newServiceForTest()
	ctx := context.Background()

	pi, err := svc.Create(ctx, "merchant-1", "order-1", 1000, "COP", domain.ChannelQR, "idem-1", "", "mova-service")
	require.NoError(t, err)
	_, err = svc.ApplyRiskResult(ctx, pi.ID, domain.RiskApprove, 10, nil, "rules-v1", "risk-service")
	require.NoError(t, err) // APPROVED es terminal

	_, err = svc.UpdateStatus(ctx, pi.ID, domain.StatusExpired, "vencido", "reconciliation-worker")
	assert.ErrorIs(t, err, domain.ErrInvalidTransition)
}

func TestUpdateStatus_UnknownIntent(t *testing.T) {
	svc, _, _, _, _ := newServiceForTest()
	_, err := svc.UpdateStatus(context.Background(), "no-existe", domain.StatusExpired, "vencido", "reconciliation-worker")
	assert.ErrorIs(t, err, application.ErrNotFound)
}

func TestGet_NotFound(t *testing.T) {
	svc, _, _, _, _ := newServiceForTest()
	_, err := svc.Get(context.Background(), "no-existe")
	assert.ErrorIs(t, err, application.ErrNotFound)
}

func TestHistory_NotFound(t *testing.T) {
	svc, _, _, _, _ := newServiceForTest()
	_, err := svc.History(context.Background(), "no-existe")
	assert.ErrorIs(t, err, application.ErrNotFound)
}
