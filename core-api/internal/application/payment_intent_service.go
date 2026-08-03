package application

import (
	"context"
	"errors"
	"time"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

const idempotencyRetryDelay = 50 * time.Millisecond

// velocityWindow es la ventana de "reciente" para merchant_recent_intents
// (ver ADR-0004) — mismo valor por defecto que usa risk-service
// (velocity_window_seconds), para que el número signifique lo mismo de
// los dos lados el día que lo empiece a consumir.
const velocityWindow = 60 * time.Second

const (
	DefaultPage  = 1
	DefaultLimit = 100
	MaxLimit     = 100
)

const (
	reasonCreated       = "payment intent creado"
	reasonSentToRisk    = "enviado a evaluación de riesgo"
	reasonRiskEvaluated = "resultado de evaluación de riesgo aplicado"
)

type PaymentIntentService struct {
	payments  PaymentIntentRepository
	history   PaymentIntentStatusHistoryRepository
	merchants MerchantRepository
	locker    IdempotencyLocker
	uow       UnitOfWork
	riskEvent RiskRequestPublisher
}

func NewPaymentIntentService(
	payments PaymentIntentRepository,
	history PaymentIntentStatusHistoryRepository,
	merchants MerchantRepository,
	locker IdempotencyLocker,
	uow UnitOfWork,
	riskEvent RiskRequestPublisher,
) *PaymentIntentService {
	return &PaymentIntentService{
		payments:  payments,
		history:   history,
		merchants: merchants,
		locker:    locker,
		uow:       uow,
		riskEvent: riskEvent,
	}
}

// Create valida y persiste un PaymentIntent en PENDING, lo mueve a
// UNDER_REVIEW de forma atómica (con su entrada de historial — ver
// ADR-0001), y recién ahí publica el evento de riesgo (async, sin
// esperar respuesta). Un reintento con la misma idempotency_key devuelve
// el intent existente sin crear otro ni volver a publicar el evento.
func (s *PaymentIntentService) Create(
	ctx context.Context,
	merchantID, externalReference string,
	amountMinor int64,
	currency string,
	channel domain.Channel,
	idempotencyKey, correlationID, changedBy string,
) (*domain.PaymentIntent, error) {
	if idempotencyKey == "" {
		return nil, domain.ErrMissingIdempotencyKey
	}

	if existing, err := s.payments.GetByIdempotencyKey(ctx, idempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	release, acquired := s.locker.Acquire(ctx, idempotencyKey)
	defer release()

	if !acquired {
		time.Sleep(idempotencyRetryDelay)
		if existing, err := s.payments.GetByIdempotencyKey(ctx, idempotencyKey); err == nil {
			return existing, nil
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}

	pi, err := domain.NewPaymentIntent(merchantID, externalReference, amountMinor, currency, channel, idempotencyKey, correlationID)
	if err != nil {
		return nil, err
	}

	merchant, err := s.merchants.GetByID(ctx, merchantID)
	if err != nil {
		return nil, err
	}

	// Se cuenta ANTES de persistir el intent actual: si no, el intent se
	// contaría a sí mismo y el umbral de velocidad quedaría corrido en uno
	// (ver ADR-0004).
	recentIntents, err := s.payments.CountRecentByMerchant(ctx, merchantID, time.Now().UTC().Add(-velocityWindow))
	if err != nil {
		return nil, err
	}

	err = s.uow.Execute(ctx, func(txCtx context.Context) error {
		if err := s.payments.Create(txCtx, pi); err != nil {
			return err
		}
		h := domain.NewPaymentIntentStatusHistory(pi.ID, "", pi.Status, reasonCreated, changedBy, pi.CorrelationID)
		return s.history.Create(txCtx, h)
	})
	if err != nil {
		if errors.Is(err, ErrConflict) {
			existing, getErr := s.payments.GetByIdempotencyKey(ctx, idempotencyKey)
			if getErr != nil {
				if errors.Is(getErr, ErrNotFound) {
					return nil, ErrConflict
				}
				return nil, getErr
			}
			return existing, nil
		}
		return nil, err
	}

	// Mover a UNDER_REVIEW ANTES de publicar (no al revés): si el proceso
	// se cayera entre publicar y guardar el estado, quedaría un evento en
	// Kafka sin que el intent refleje que ya se envió (ver ADR-0001).
	if err := s.markUnderReview(ctx, pi, changedBy); err != nil {
		return nil, err
	}

	// Publicar es best-effort desde la perspectiva del request HTTP: si
	// Kafka falla acá, el intent ya quedó en UNDER_REVIEW; el circuit
	// breaker (pendiente, ver README) se encargará de la resiliencia.
	_ = s.riskEvent.Publish(ctx, RiskEvaluationRequested{
		PaymentIntentID:       pi.ID,
		MerchantID:            pi.MerchantID,
		ExternalReference:     pi.ExternalReference,
		AmountMinor:           pi.AmountMinor,
		Currency:              pi.Currency,
		Channel:               string(pi.Channel),
		CorrelationID:         pi.CorrelationID,
		MerchantStatus:        string(merchant.Status),
		MerchantRecentIntents: int(recentIntents),
	})

	return pi, nil
}

// markUnderReview mueve PENDING->UNDER_REVIEW de forma atómica con su
// entrada de historial, justo antes de publicar el evento de riesgo.
func (s *PaymentIntentService) markUnderReview(ctx context.Context, pi *domain.PaymentIntent, changedBy string) error {
	previousStatus := pi.Status
	if err := pi.ChangeStatus(domain.StatusUnderReview); err != nil {
		return err
	}

	return s.uow.Execute(ctx, func(txCtx context.Context) error {
		if err := s.payments.Update(txCtx, pi); err != nil {
			return err
		}
		h := domain.NewPaymentIntentStatusHistory(pi.ID, previousStatus, pi.Status, reasonSentToRisk, changedBy, pi.CorrelationID)
		return s.history.Create(txCtx, h)
	})
}

// ApplyRiskResult aplica el veredicto de riesgo llegado por Kafka. Lo
// invoca el consumer de infraestructura, no un endpoint HTTP.
func (s *PaymentIntentService) ApplyRiskResult(
	ctx context.Context,
	paymentIntentID string,
	decision domain.RiskDecision,
	score int,
	reasonCodes []string,
	modelVersion, changedBy string,
) (*domain.PaymentIntent, error) {
	pi, err := s.payments.GetByID(ctx, paymentIntentID)
	if err != nil {
		return nil, err
	}

	previousStatus := pi.Status
	if err := pi.ApplyRiskDecision(decision, score, reasonCodes, modelVersion); err != nil {
		return nil, err
	}

	err = s.uow.Execute(ctx, func(txCtx context.Context) error {
		if err := s.payments.Update(txCtx, pi); err != nil {
			return err
		}
		h := domain.NewPaymentIntentStatusHistory(pi.ID, previousStatus, pi.Status, reasonRiskEvaluated, changedBy, pi.CorrelationID)
		return s.history.Create(txCtx, h)
	})
	if err != nil {
		return nil, err
	}

	return pi, nil
}

// UpdateStatus aplica una transición manual (hoy solo la usa el
// reconciliation-worker para expirar intents vencidos). Si el intent ya
// está en el estado pedido, es ErrConflict (409, el worker lo cuenta como
// éxito); una transición que la tabla del dominio no permite es
// domain.ErrInvalidTransition (422) — se distingue ANTES de llamar a
// ChangeStatus, que por sí sola no separa los dos casos.
func (s *PaymentIntentService) UpdateStatus(ctx context.Context, paymentIntentID string, next domain.Status, reason, changedBy string) (*domain.PaymentIntent, error) {
	pi, err := s.payments.GetByID(ctx, paymentIntentID)
	if err != nil {
		return nil, err
	}

	if pi.Status == next {
		return nil, ErrConflict
	}

	previousStatus := pi.Status
	if err := pi.ChangeStatus(next); err != nil {
		return nil, err
	}

	err = s.uow.Execute(ctx, func(txCtx context.Context) error {
		if err := s.payments.Update(txCtx, pi); err != nil {
			return err
		}
		h := domain.NewPaymentIntentStatusHistory(pi.ID, previousStatus, pi.Status, reason, changedBy, pi.CorrelationID)
		return s.history.Create(txCtx, h)
	})
	if err != nil {
		return nil, err
	}

	return pi, nil
}

func (s *PaymentIntentService) Get(ctx context.Context, id string) (*domain.PaymentIntent, error) {
	return s.payments.GetByID(ctx, id)
}

func (s *PaymentIntentService) List(ctx context.Context, filter PaymentIntentFilter) ([]*domain.PaymentIntent, int64, error) {
	if filter.Page < 1 {
		filter.Page = DefaultPage
	}
	if filter.Limit <= 0 {
		filter.Limit = DefaultLimit
	}
	if filter.Limit > MaxLimit {
		filter.Limit = MaxLimit
	}

	items, err := s.payments.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.payments.Count(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *PaymentIntentService) History(ctx context.Context, paymentIntentID string) ([]*domain.PaymentIntentStatusHistory, error) {
	if _, err := s.payments.GetByID(ctx, paymentIntentID); err != nil {
		return nil, err
	}
	return s.history.ListByPaymentIntentID(ctx, paymentIntentID)
}
