// Package kafka implementa los puertos de mensajería contra un broker
// Kafka real (ver ADR-0001).
package kafka

import (
	"context"
	"encoding/json"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
)

const TopicRiskEvaluationRequested = "risk.evaluation.requested"

type riskRequestedMessage struct {
	PaymentIntentID   string `json:"payment_intent_id"`
	MerchantID        string `json:"merchant_id"`
	ExternalReference string `json:"external_reference"`
	AmountMinor       int64  `json:"amount_minor"`
	Currency          string `json:"currency"`
	Channel           string `json:"channel"`
	CorrelationID     string `json:"correlation_id"`
}

// RiskRequestPublisher implementa application.RiskRequestPublisher
// publicando a TopicRiskEvaluationRequested.
type RiskRequestPublisher struct {
	writer *kafkago.Writer
}

func NewRiskRequestPublisher(brokers []string) *RiskRequestPublisher {
	return &RiskRequestPublisher{
		writer: &kafkago.Writer{
			Addr:                   kafkago.TCP(brokers...),
			Topic:                  TopicRiskEvaluationRequested,
			Balancer:               &kafkago.LeastBytes{},
			RequiredAcks:           kafkago.RequireOne,
			AllowAutoTopicCreation: true,
		},
	}
}

var _ application.RiskRequestPublisher = (*RiskRequestPublisher)(nil)

func (p *RiskRequestPublisher) Publish(ctx context.Context, event application.RiskEvaluationRequested) error {
	payload, err := json.Marshal(riskRequestedMessage{
		PaymentIntentID:   event.PaymentIntentID,
		MerchantID:        event.MerchantID,
		ExternalReference: event.ExternalReference,
		AmountMinor:       event.AmountMinor,
		Currency:          event.Currency,
		Channel:           event.Channel,
		CorrelationID:     event.CorrelationID,
	})
	if err != nil {
		return err
	}

	// Key = payment_intent_id: mensajes del mismo intent van a la misma
	// partición, así se procesan en orden si alguna vez hay más de uno.
	return p.writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(event.PaymentIntentID),
		Value: payload,
	})
}

func (p *RiskRequestPublisher) Close() error {
	return p.writer.Close()
}
