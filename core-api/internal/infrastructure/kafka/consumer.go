package kafka

import (
	"context"
	"encoding/json"
	"log"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

const TopicRiskEvaluationCompleted = "risk.evaluation.completed"

// changedByRiskService identifica al Risk Service como autor en el
// historial cuando el resultado llega por Kafka.
const changedByRiskService = "risk-service"

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
	reader  *kafkago.Reader
	service *application.PaymentIntentService
}

func NewRiskResultConsumer(brokers []string, groupID string, service *application.PaymentIntentService) *RiskResultConsumer {
	return &RiskResultConsumer{
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers: brokers,
			Topic:   TopicRiskEvaluationCompleted,
			GroupID: groupID,
		}),
		service: service,
	}
}

// Run consume hasta que ctx se cancele. Un mensaje mal formado se loguea
// y se descarta — no debe bloquear el resto de la cola.
func (c *RiskResultConsumer) Run(ctx context.Context) error {
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
			changedByRiskService,
		)
		if err != nil {
			log.Printf("kafka: no se pudo aplicar el resultado de riesgo para %s: %v", payload.PaymentIntentID, err)
		}
	}
}

func (c *RiskResultConsumer) Close() error {
	return c.reader.Close()
}
