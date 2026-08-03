package domain

import (
	"time"

	"github.com/google/uuid"
)

type Channel string

const (
	ChannelQR                 Channel = "QR"
	ChannelPaymentLink        Channel = "PAYMENT_LINK"
	ChannelDataphoneSimulated Channel = "DATAPHONE_SIMULATED"
)

func (c Channel) IsValid() bool {
	switch c {
	case ChannelQR, ChannelPaymentLink, ChannelDataphoneSimulated:
		return true
	}
	return false
}

type Status string

const (
	StatusPending     Status = "PENDING"
	StatusUnderReview Status = "UNDER_REVIEW"
	StatusApproved    Status = "APPROVED"
	StatusRejected    Status = "REJECTED"
	StatusCancelled   Status = "CANCELLED"
	StatusExpired     Status = "EXPIRED"
)

// allowedTransitions define a qué estados puede pasar cada estado.
// Todo pago pasa por UNDER_REVIEW al enviarse a evaluación de riesgo —
// PENDING->APPROVED/REJECTED directo no existe (ver ADR-0001). Si el
// Risk Service se demora o no responde, UNDER_REVIEW es el estado
// seguro en el que se queda (ver ADR-0003).
var allowedTransitions = map[Status][]Status{
	StatusPending:     {StatusUnderReview, StatusCancelled, StatusExpired},
	StatusUnderReview: {StatusApproved, StatusRejected, StatusExpired},
}

func (s Status) CanTransitionTo(next Status) bool {
	for _, allowed := range allowedTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

type RiskDecision string

const (
	RiskApprove RiskDecision = "APPROVE"
	RiskReview  RiskDecision = "REVIEW"
	RiskReject  RiskDecision = "REJECT"
)

// PaymentIntent es la entidad central: un intento de pago desde su
// creación hasta su resolución (aprobado, rechazado, cancelado o vencido).
type PaymentIntent struct {
	ID                string
	MerchantID        string
	ExternalReference string
	IdempotencyKey    string
	AmountMinor       int64
	Currency          string
	Channel           Channel
	Status            Status
	RiskDecision      *RiskDecision
	RiskScore         *int
	RiskReasonCodes   []string
	RiskModelVersion  *string
	CorrelationID     string
	ExpiresAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

const SupportedCurrency = "COP"

const defaultExpirationWindow = 30 * time.Minute

// NewPaymentIntent valida los datos de entrada y arma un PaymentIntent
// nuevo en PENDING. correlationID puede venir vacío: si no, se genera acá.
func NewPaymentIntent(
	merchantID, externalReference string,
	amountMinor int64,
	currency string,
	channel Channel,
	idempotencyKey string,
	correlationID string,
) (*PaymentIntent, error) {
	if merchantID == "" {
		return nil, ErrMissingMerchantID
	}
	if externalReference == "" {
		return nil, ErrMissingExternalReference
	}
	if idempotencyKey == "" {
		return nil, ErrMissingIdempotencyKey
	}
	if amountMinor <= 0 {
		return nil, ErrInvalidAmount
	}
	if currency != SupportedCurrency {
		return nil, ErrInvalidCurrency
	}
	if !channel.IsValid() {
		return nil, ErrInvalidChannel
	}
	if correlationID == "" {
		correlationID = uuid.New().String()
	}

	now := time.Now().UTC()
	return &PaymentIntent{
		ID:                uuid.New().String(),
		MerchantID:        merchantID,
		ExternalReference: externalReference,
		IdempotencyKey:    idempotencyKey,
		AmountMinor:       amountMinor,
		Currency:          currency,
		Channel:           channel,
		Status:            StatusPending,
		CorrelationID:     correlationID,
		ExpiresAt:         now.Add(defaultExpirationWindow),
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// ChangeStatus aplica una transición si es válida; si no, devuelve
// ErrInvalidTransition sin modificar el intent.
func (p *PaymentIntent) ChangeStatus(next Status) error {
	if !p.Status.CanTransitionTo(next) {
		return ErrInvalidTransition
	}
	p.Status = next
	p.UpdatedAt = time.Now().UTC()
	return nil
}

// ApplyRiskDecision guarda el veredicto de riesgo y mueve el estado según
// corresponda (APPROVE->APPROVED, REVIEW->UNDER_REVIEW, REJECT->REJECTED).
func (p *PaymentIntent) ApplyRiskDecision(decision RiskDecision, score int, reasonCodes []string, modelVersion string) error {
	next := p.Status
	switch decision {
	case RiskApprove:
		next = StatusApproved
	case RiskReject:
		next = StatusRejected
	case RiskReview:
		next = StatusUnderReview
	}

	// REVIEW sobre un intent que ya está en UNDER_REVIEW no es una
	// transición real (sigue igual) — pero sí actualiza el score/razones,
	// por ejemplo si el riesgo se reevalúa.
	if next != p.Status {
		if err := p.ChangeStatus(next); err != nil {
			return err
		}
	}

	p.RiskDecision = &decision
	p.RiskScore = &score
	p.RiskReasonCodes = reasonCodes
	p.RiskModelVersion = &modelVersion
	return nil
}
