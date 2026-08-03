// Package kafka implementa los puertos de mensajería contra un broker
// Kafka real (ver ADR-0001).
package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
)

const TopicRiskEvaluationRequested = "risk.evaluation.requested"

// Umbral más bajo que el del reconciliation-worker (10/60s): esto corre
// en el camino síncrono de creación de un pago, no en un ciclo de fondo,
// así que conviene fallar rápido hacia el HTTP directo (ver ADR-0008).
const (
	breakerFailureThreshold = 3
	breakerResetTimeout     = 30 * time.Second
)

type riskRequestedMessage struct {
	PaymentIntentID       string `json:"payment_intent_id"`
	MerchantID            string `json:"merchant_id"`
	ExternalReference     string `json:"external_reference"`
	AmountMinor           int64  `json:"amount_minor"`
	Currency              string `json:"currency"`
	Channel               string `json:"channel"`
	CorrelationID         string `json:"correlation_id"`
	MerchantStatus        string `json:"merchant_status"`
	MerchantRecentIntents int    `json:"merchant_recent_intents"`
}

// riskFallback es lo mínimo que necesita el camino de emergencia — lo
// cumple *riskhttp.Client sin que este paquete dependa de él (evita el
// ciclo: riskhttp ya importa application).
type riskFallback interface {
	Evaluate(ctx context.Context, event application.RiskEvaluationRequested) (*application.RiskEvaluationResult, error)
}

// RiskRequestPublisher implementa application.RiskRequestPublisher
// publicando a TopicRiskEvaluationRequested. Si Kafka mismo falla varias
// veces seguidas, el circuit breaker se abre y las siguientes llamadas
// caen directo al HTTP síncrono de risk-service, sin gastar el timeout
// de Kafka (ver ADR-0008).
type RiskRequestPublisher struct {
	writer   *kafkago.Writer
	breaker  *circuitBreaker
	fallback riskFallback
}

func NewRiskRequestPublisher(brokers []string, fallback riskFallback) *RiskRequestPublisher {
	return &RiskRequestPublisher{
		writer: &kafkago.Writer{
			Addr:                   kafkago.TCP(brokers...),
			Topic:                  TopicRiskEvaluationRequested,
			Balancer:               &kafkago.LeastBytes{},
			RequiredAcks:           kafkago.RequireOne,
			AllowAutoTopicCreation: true,
		},
		breaker:  newCircuitBreaker(breakerFailureThreshold, breakerResetTimeout),
		fallback: fallback,
	}
}

var _ application.RiskRequestPublisher = (*RiskRequestPublisher)(nil)

func (p *RiskRequestPublisher) Publish(ctx context.Context, event application.RiskEvaluationRequested) (*application.RiskEvaluationResult, error) {
	if p.breaker.allow() {
		if err := p.publishToKafka(ctx, event); err == nil {
			p.breaker.onSuccess()
			return nil, nil
		}
		p.breaker.onFailure()
	}

	// Kafka no disponible (o el breaker ya lo sabe): se resuelve ya,
	// síncrono, para no dejar el intent esperando un evento que nunca
	// va a llegar.
	if p.fallback == nil {
		return nil, errors.New("kafka no disponible y no hay respaldo HTTP configurado")
	}
	return p.fallback.Evaluate(ctx, event)
}

func (p *RiskRequestPublisher) publishToKafka(ctx context.Context, event application.RiskEvaluationRequested) error {
	payload, err := json.Marshal(riskRequestedMessage{
		PaymentIntentID:       event.PaymentIntentID,
		MerchantID:            event.MerchantID,
		ExternalReference:     event.ExternalReference,
		AmountMinor:           event.AmountMinor,
		Currency:              event.Currency,
		Channel:               event.Channel,
		CorrelationID:         event.CorrelationID,
		MerchantStatus:        event.MerchantStatus,
		MerchantRecentIntents: event.MerchantRecentIntents,
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
