//go:build integration

package kafka_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/kafka"
)

var brokers = []string{"localhost:9092"}

func TestEnsureTopics_IdempotentAcrossCalls(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, kafka.EnsureTopics(ctx, brokers, "test-ensure-topics"))
	require.NoError(t, kafka.EnsureTopics(ctx, brokers, "test-ensure-topics"), "una segunda llamada no debe fallar")
}

func TestRiskRequestPublisher_PublishesToTopic(t *testing.T) {
	publisher := kafka.NewRiskRequestPublisher(brokers)
	defer publisher.Close()

	// El tópico debe existir ANTES de crear el Reader: con GroupID, el
	// Reader arranca su descubrimiento de partición en background desde
	// el constructor, y si el tópico todavía no existe (auto-creación
	// dispara recién con el primer Publish) queda con metadata inválida.
	expectedID := "pi-test-" + uuid.New().String()
	event := application.RiskEvaluationRequested{
		PaymentIntentID: expectedID, MerchantID: "merchant-1",
		ExternalReference: "order-1", AmountMinor: 15000000,
		Currency: "COP", Channel: "QR", CorrelationID: "corr-1",
		MerchantStatus: "ACTIVE", MerchantRecentIntents: 3,
	}
	require.NoError(t, publisher.Publish(context.Background(), event))

	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: brokers,
		Topic:   kafka.TopicRiskEvaluationRequested,
		GroupID: "test-" + t.Name(),
	})
	defer reader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// El tópico es compartido (también lo usa la verificación manual en
	// vivo) y puede tener mensajes viejos antes del nuestro — se avanza
	// hasta encontrar el que este test publicó, en vez de asumir que es
	// el primero.
	var got map[string]any
	for {
		msg, err := reader.ReadMessage(ctx)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(msg.Value, &got))
		if got["payment_intent_id"] == expectedID {
			break
		}
	}

	assert.Equal(t, "ACTIVE", got["merchant_status"])
	assert.Equal(t, float64(3), got["merchant_recent_intents"])
}

type noopLocker struct{}

func (noopLocker) Acquire(_ context.Context, _ string) (func(), bool) { return func() {}, true }

type noopPublisher struct{}

func (noopPublisher) Publish(_ context.Context, _ application.RiskEvaluationRequested) error {
	return nil
}

// fakePaymentIntentRepository e fakeHistoryRepository son solo para probar
// el consumer contra Kafka real sin depender de Postgres (ver
// internal/infrastructure/postgres para la implementación real).
type fakePaymentIntentRepository struct {
	mu   sync.Mutex
	byID map[string]*domain.PaymentIntent
}

func (r *fakePaymentIntentRepository) Create(_ context.Context, pi *domain.PaymentIntent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID == nil {
		r.byID = make(map[string]*domain.PaymentIntent)
	}
	cp := *pi
	r.byID[pi.ID] = &cp
	return nil
}

func (r *fakePaymentIntentRepository) GetByID(_ context.Context, id string) (*domain.PaymentIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pi, ok := r.byID[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	cp := *pi
	return &cp, nil
}

func (r *fakePaymentIntentRepository) GetByIdempotencyKey(_ context.Context, key string) (*domain.PaymentIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, pi := range r.byID {
		if pi.IdempotencyKey == key {
			cp := *pi
			return &cp, nil
		}
	}
	return nil, application.ErrNotFound
}

func (r *fakePaymentIntentRepository) Update(_ context.Context, pi *domain.PaymentIntent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[pi.ID]; !ok {
		return application.ErrNotFound
	}
	cp := *pi
	r.byID[pi.ID] = &cp
	return nil
}

func (r *fakePaymentIntentRepository) List(_ context.Context, _ application.PaymentIntentFilter) ([]*domain.PaymentIntent, error) {
	return nil, nil
}

func (r *fakePaymentIntentRepository) Count(_ context.Context, _ application.PaymentIntentFilter) (int64, error) {
	return 0, nil
}

func (r *fakePaymentIntentRepository) CountRecentByMerchant(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}

type fakeMerchantRepository struct{}

func (fakeMerchantRepository) Create(_ context.Context, _ *domain.Merchant) error { return nil }

func (fakeMerchantRepository) GetByID(_ context.Context, id string) (*domain.Merchant, error) {
	return &domain.Merchant{ID: id, Status: domain.MerchantStatusActive}, nil
}

type fakeHistoryRepository struct{}

func (fakeHistoryRepository) Create(_ context.Context, _ *domain.PaymentIntentStatusHistory) error {
	return nil
}

func (fakeHistoryRepository) ListByPaymentIntentID(_ context.Context, _ string) ([]*domain.PaymentIntentStatusHistory, error) {
	return nil, nil
}

type fakeUnitOfWork struct{}

func (fakeUnitOfWork) Execute(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestRiskResultConsumer_AppliesResultFromKafka(t *testing.T) {
	payments := &fakePaymentIntentRepository{}
	history := fakeHistoryRepository{}
	merchants := fakeMerchantRepository{}
	service := application.NewPaymentIntentService(payments, history, merchants, noopLocker{}, fakeUnitOfWork{}, noopPublisher{})

	pi, err := domain.NewPaymentIntent("merchant-1", "order-1", 1000, "COP", domain.ChannelQR, "idem-1", "")
	require.NoError(t, err)
	require.NoError(t, payments.Create(context.Background(), pi))

	// Todo intent pasa por UNDER_REVIEW al enviarse a riesgo (ver
	// ADR-0001) — sin este paso, aplicar una decisión fallaría por
	// transición inválida (PENDING no va directo a APPROVED/REJECTED).
	require.NoError(t, pi.ChangeStatus(domain.StatusUnderReview))
	require.NoError(t, payments.Update(context.Background(), pi))

	require.NoError(t, kafka.EnsureTopics(context.Background(), brokers, kafka.TopicRiskEvaluationCompleted))

	consumer := kafka.NewRiskResultConsumer(brokers, "test-consumer-"+t.Name(), service)
	defer consumer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go consumer.Run(ctx)

	writer := &kafkago.Writer{Addr: kafkago.TCP(brokers...), Topic: kafka.TopicRiskEvaluationCompleted, AllowAutoTopicCreation: true}
	defer writer.Close()

	payload, _ := json.Marshal(map[string]any{
		"payment_intent_id": pi.ID, "decision": "APPROVE", "score": 10,
		"reason_codes": []string{}, "model_version": "rules-v1",
	})
	require.NoError(t, writer.WriteMessages(context.Background(), kafkago.Message{Value: payload}))

	require.Eventually(t, func() bool {
		got, err := payments.GetByID(context.Background(), pi.ID)
		return err == nil && got.Status == domain.StatusApproved
	}, 8*time.Second, 200*time.Millisecond)
}
