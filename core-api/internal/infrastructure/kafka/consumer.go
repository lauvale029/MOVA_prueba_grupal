package kafka

import (
	"context"
	"encoding/json"
	"log"
	"math/rand"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

const TopicRiskEvaluationCompleted = "risk.evaluation.completed"

const (
	reconnectBackoffBase = time.Second
	reconnectBackoffCap  = 30 * time.Second
)

type riskCompletedMessage struct {
	PaymentIntentID string   `json:"payment_intent_id"`
	Decision        string   `json:"decision"`
	Score           int      `json:"score"`
	ReasonCodes     []string `json:"reason_codes"`
	ModelVersion    string   `json:"model_version"`
}

// RiskResultConsumer consume TopicRiskEvaluationCompleted y aplica cada
// resultado vía PaymentIntentService.ApplyRiskResult.
type RiskResultConsumer struct {
	brokers []string
	groupID string
	reader  *kafkago.Reader
	service *application.PaymentIntentService
}

func NewRiskResultConsumer(brokers []string, groupID string, service *application.PaymentIntentService) *RiskResultConsumer {
	return &RiskResultConsumer{
		brokers: brokers,
		groupID: groupID,
		reader:  newRiskResultReader(brokers, groupID),
		service: service,
	}
}

func newRiskResultReader(brokers []string, groupID string) *kafkago.Reader {
	return kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: brokers,
		Topic:   TopicRiskEvaluationCompleted,
		GroupID: groupID,
	})
}

// Run consume hasta que ctx se cancele. Si se pierde la conexión con
// Kafka, reconecta con backoff en vez de morir para siempre: antes, una
// caída transitoria del broker dejaba este consumer permanentemente sin
// aplicar resultados de riesgo por el resto de la vida del proceso — el
// caso simétrico del que protege el circuit breaker del publisher (ver
// ADR-0008), pero del lado de consumir en vez de publicar.
func (c *RiskResultConsumer) Run(ctx context.Context) error {
	for attempt := 0; ; attempt++ {
		err := c.consumeUntilError(ctx)
		if ctx.Err() != nil {
			return nil
		}
		log.Printf("kafka: consumer de riesgo perdió la conexión, reconectando: %v", err)

		_ = c.reader.Close()
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoffWithJitter(attempt, reconnectBackoffBase, reconnectBackoffCap)):
		}
		c.reader = newRiskResultReader(c.brokers, c.groupID)
	}
}

// consumeUntilError procesa mensajes hasta que ReadMessage falle (ctx
// cancelado o el broker se volvió inalcanzable) — Run decide qué hacer
// con ese error.
func (c *RiskResultConsumer) consumeUntilError(ctx context.Context) error {
	for {
		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			return err
		}

		var payload riskCompletedMessage
		if err := json.Unmarshal(msg.Value, &payload); err != nil {
			log.Printf("kafka: mensaje inválido en %s: %v", TopicRiskEvaluationCompleted, err)
			continue
		}

		_, err = c.service.ApplyRiskResult(
			ctx,
			payload.PaymentIntentID,
			domain.RiskDecision(payload.Decision),
			payload.Score,
			payload.ReasonCodes,
			payload.ModelVersion,
			application.ChangedByRiskService,
		)
		if err != nil {
			log.Printf("kafka: no se pudo aplicar el resultado de riesgo para %s: %v", payload.PaymentIntentID, err)
		}
	}
}

func (c *RiskResultConsumer) Close() error {
	return c.reader.Close()
}

// backoffWithJitter es el mismo patrón que ya usa el reconciliation-worker
// en Python (worker/infrastructure/resilience.py): jitter completo,
// random(0, min(tope, base * 2^intento)), para que reconexiones
// simultáneas no se sincronicen.
func backoffWithJitter(attempt int, base, capDuration time.Duration) time.Duration {
	if attempt > 20 { // evita que el corrimiento de bits desborde
		attempt = 20
	}
	ceiling := base * time.Duration(1<<attempt)
	if ceiling <= 0 || ceiling > capDuration {
		ceiling = capDuration
	}
	return time.Duration(rand.Int63n(int64(ceiling) + 1))
}
