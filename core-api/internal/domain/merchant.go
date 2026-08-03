package domain

import (
	"time"

	"github.com/google/uuid"
)

type MerchantStatus string

const (
	MerchantStatusActive   MerchantStatus = "ACTIVE"
	MerchantStatusInactive MerchantStatus = "INACTIVE"
)

type Merchant struct {
	ID             string
	Name           string
	DocumentNumber string
	Email          string
	Status         MerchantStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewMerchant valida los datos de entrada y arma un Merchant nuevo,
// siempre ACTIVE — el enunciado no define un flujo de activación.
func NewMerchant(name, documentNumber, email string) (*Merchant, error) {
	if name == "" {
		return nil, ErrMissingMerchantName
	}
	if documentNumber == "" {
		return nil, ErrMissingDocumentNumber
	}
	if email == "" {
		return nil, ErrMissingEmail
	}

	now := time.Now().UTC()
	return &Merchant{
		ID:             uuid.New().String(),
		Name:           name,
		DocumentNumber: documentNumber,
		Email:          email,
		Status:         MerchantStatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}
