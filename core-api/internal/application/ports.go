package application

import (
	"context"
	"time"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

// PaymentIntentFilter filtra el listado de payment intents.
type PaymentIntentFilter struct {
	MerchantID string
	Status     domain.Status
	Page       int
	Limit      int
}

type MerchantRepository interface {
	Create(ctx context.Context, m *domain.Merchant) error
	GetByID(ctx context.Context, id string) (*domain.Merchant, error)
}

type PaymentIntentRepository interface {
	Create(ctx context.Context, pi *domain.PaymentIntent) error
	GetByID(ctx context.Context, id string) (*domain.PaymentIntent, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*domain.PaymentIntent, error)
	Update(ctx context.Context, pi *domain.PaymentIntent) error
	List(ctx context.Context, filter PaymentIntentFilter) ([]*domain.PaymentIntent, error)
	Count(ctx context.Context, filter PaymentIntentFilter) (int64, error)
	// CountRecentByMerchant cuenta los intents de ese comercio creados desde
	// "since". Alimenta merchant_recent_intents (ver ADR-0004) — se consulta
	// ANTES de crear el intent actual, para que no se cuente a sí mismo.
	CountRecentByMerchant(ctx context.Context, merchantID string, since time.Time) (int64, error)
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
// MerchantStatus y MerchantRecentIntents son opcionales (ver ADR-0004):
// core-api es dueño de ambos datos; risk-service los usa si vienen.
type RiskEvaluationRequested struct {
	PaymentIntentID       string
	MerchantID            string
	ExternalReference     string
	AmountMinor           int64
	Currency              string
	Channel               string
	CorrelationID         string
	MerchantStatus        string
	MerchantRecentIntents int
}

// RiskEvaluationResult es la decisión que llega YA, en el mismo request,
// cuando Publish tuvo que caer al camino HTTP directo porque Kafka mismo
// falló (ver ADR-0008) — nil cuando el evento se publicó a Kafka con
// éxito, ya que ahí la decisión llega después de forma asíncrona,
// consumida por la infraestructura y aplicada vía
// PaymentIntentService.ApplyRiskResult.
type RiskEvaluationResult struct {
	Decision     domain.RiskDecision
	Score        int
	ReasonCodes  []string
	ModelVersion string
}

// RiskRequestPublisher publica el evento de evaluación de riesgo.
type RiskRequestPublisher interface {
	Publish(ctx context.Context, event RiskEvaluationRequested) (*RiskEvaluationResult, error)
}

// ChangedByRiskService identifica al Risk Service como autor en el
// historial, sin importar si la decisión llegó por Kafka o por el
// camino HTTP directo (ver ADR-0008).
const ChangedByRiskService = "risk-service"
