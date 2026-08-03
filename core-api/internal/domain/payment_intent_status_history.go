package domain

import (
	"time"

	"github.com/google/uuid"
)

// PaymentIntentStatusHistory es una entrada inmutable del historial de
// cambios de estado de un PaymentIntent.
type PaymentIntentStatusHistory struct {
	ID              string
	PaymentIntentID string
	PreviousStatus  Status
	NewStatus       Status
	Reason          string
	ChangedBy       string
	CorrelationID   string
	CreatedAt       time.Time
}

func NewPaymentIntentStatusHistory(paymentIntentID string, previous, next Status, reason, changedBy, correlationID string) *PaymentIntentStatusHistory {
	return &PaymentIntentStatusHistory{
		ID:              uuid.New().String(),
		PaymentIntentID: paymentIntentID,
		PreviousStatus:  previous,
		NewStatus:       next,
		Reason:          reason,
		ChangedBy:       changedBy,
		CorrelationID:   correlationID,
		CreatedAt:       time.Now().UTC(),
	}
}
