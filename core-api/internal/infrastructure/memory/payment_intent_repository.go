// Package memory implementa los repositorios en memoria que sostienen el
// sistema mientras la persistencia real en Postgres no está lista (ver
// README y ADR-0002). No usar en producción.
package memory

import (
	"context"
	"sync"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

type PaymentIntentRepository struct {
	mu        sync.Mutex
	byID      map[string]*domain.PaymentIntent
	byIdemKey map[string]string
}

func NewPaymentIntentRepository() *PaymentIntentRepository {
	return &PaymentIntentRepository{
		byID:      make(map[string]*domain.PaymentIntent),
		byIdemKey: make(map[string]string),
	}
}

var _ application.PaymentIntentRepository = (*PaymentIntentRepository)(nil)

func copyPI(pi *domain.PaymentIntent) *domain.PaymentIntent {
	cp := *pi
	return &cp
}

func (r *PaymentIntentRepository) Create(_ context.Context, pi *domain.PaymentIntent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byIdemKey[pi.IdempotencyKey]; exists {
		return application.ErrConflict
	}
	r.byID[pi.ID] = copyPI(pi)
	r.byIdemKey[pi.IdempotencyKey] = pi.ID
	return nil
}

func (r *PaymentIntentRepository) GetByID(_ context.Context, id string) (*domain.PaymentIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pi, ok := r.byID[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	return copyPI(pi), nil
}

func (r *PaymentIntentRepository) GetByIdempotencyKey(_ context.Context, key string) (*domain.PaymentIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byIdemKey[key]
	if !ok {
		return nil, application.ErrNotFound
	}
	return copyPI(r.byID[id]), nil
}

func (r *PaymentIntentRepository) Update(_ context.Context, pi *domain.PaymentIntent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[pi.ID]; !ok {
		return application.ErrNotFound
	}
	r.byID[pi.ID] = copyPI(pi)
	return nil
}

func (r *PaymentIntentRepository) List(_ context.Context, filter application.PaymentIntentFilter) ([]*domain.PaymentIntent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	matches := r.filtered(filter)
	start := (filter.Page - 1) * filter.Limit
	if start >= len(matches) {
		return []*domain.PaymentIntent{}, nil
	}
	end := start + filter.Limit
	if end > len(matches) {
		end = len(matches)
	}
	return matches[start:end], nil
}

func (r *PaymentIntentRepository) Count(_ context.Context, filter application.PaymentIntentFilter) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.filtered(filter))), nil
}

// filtered asume el lock ya tomado por quien llama.
func (r *PaymentIntentRepository) filtered(filter application.PaymentIntentFilter) []*domain.PaymentIntent {
	items := make([]*domain.PaymentIntent, 0, len(r.byID))
	for _, pi := range r.byID {
		if filter.MerchantID != "" && pi.MerchantID != filter.MerchantID {
			continue
		}
		if filter.Status != "" && pi.Status != filter.Status {
			continue
		}
		items = append(items, copyPI(pi))
	}
	return items
}
