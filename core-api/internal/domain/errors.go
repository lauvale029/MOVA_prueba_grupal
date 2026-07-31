package domain

import "errors"

var (
	ErrInvalidAmount            = errors.New("amount_minor debe ser mayor a 0")
	ErrInvalidCurrency          = errors.New("moneda no soportada")
	ErrInvalidChannel           = errors.New("canal no soportado")
	ErrMissingExternalReference = errors.New("external_reference es obligatorio")
	ErrMissingIdempotencyKey    = errors.New("idempotency_key es obligatorio")
	ErrMissingMerchantID        = errors.New("merchant_id es obligatorio")
	ErrInvalidTransition        = errors.New("la transición de estado no está permitida")
)
