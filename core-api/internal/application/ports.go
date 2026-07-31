package application

import (
	"context"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

// PaymentIntentFilter filtra el listado de payment intents.
type PaymentIntentFilter struct {
	MerchantID string
	Status     domain.Status
	Page       int
	Limit      int
}

type PaymentIntentRepository interface {
	Create(ctx context.Context, pi *domain.PaymentIntent) error
	GetByID(ctx context.Context, id string) (*domain.PaymentIntent, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*domain.PaymentIntent, error)
	Update(ctx context.Context, pi *domain.PaymentIntent) error
	List(ctx context.Context, filter PaymentIntentFilter) ([]*domain.PaymentIntent, error)
	Count(ctx context.Context, filter PaymentIntentFilter) (int64, error)
}

type PaymentIntentStatusHistoryRepository interface {
	Create(ctx context.Context, h *domain.PaymentIntentStatusHistory) error
	ListByPaymentIntentID(ctx context.Context, paymentIntentID string) ([]*domain.PaymentIntentStatusHistory, error)
}

// UnitOfWork agrupa varias escrituras en una sola transacción.
type UnitOfWork interface {
	Execute(ctx context.Context, fn func(ctx context.Context) error) error
}

// IdempotencyLocker es un lock best-effort en Redis; si no está
// disponible, la restricción única de Postgres sigue siendo la garantía
// final (ver ADR-0002).
type IdempotencyLocker interface {
	Acquire(ctx context.Context, key string) (release func(), acquired bool)
}

// RiskEvaluationRequested es el evento que se publica a Kafka para que el
// Risk Service evalúe un PaymentIntent (ver ADR-0001, contrato Go/Python).
type RiskEvaluationRequested struct {
	PaymentIntentID   string
	MerchantID        string
	ExternalReference string
	AmountMinor       int64
	Currency          string
	Channel           string
	CorrelationID     string
}

// RiskRequestPublisher publica el evento de evaluación de riesgo. No
// espera respuesta: el resultado llega después por otro tópico,
// consumido por la infraestructura y aplicado vía
// PaymentIntentService.ApplyRiskResult.
type RiskRequestPublisher interface {
	Publish(ctx context.Context, event RiskEvaluationRequested) error
}
