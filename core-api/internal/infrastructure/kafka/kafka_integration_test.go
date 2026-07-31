//go:build integration

package kafka_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/kafka"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/memory"
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
}

type noopLocker struct{}

func (noopLocker) Acquire(_ context.Context, _ string) (func(), bool) { return func() {}, true }

type noopPublisher struct{}

func (noopPublisher) Publish(_ context.Context, _ application.RiskEvaluationRequested) error {
	return nil
}

func TestRiskResultConsumer_AppliesResultFromKafka(t *testing.T) {
	payments := memory.NewPaymentIntentRepository()
	history := memory.NewPaymentIntentStatusHistoryRepository()
	service := application.NewPaymentIntentService(payments, history, noopLocker{}, memory.UnitOfWork{}, noopPublisher{})

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
