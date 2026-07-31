package memory

import (
	"context"
	"sync"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

type PaymentIntentStatusHistoryRepository struct {
	mu      sync.Mutex
	entries []*domain.PaymentIntentStatusHistory
}

func NewPaymentIntentStatusHistoryRepository() *PaymentIntentStatusHistoryRepository {
	return &PaymentIntentStatusHistoryRepository{}
}

var _ application.PaymentIntentStatusHistoryRepository = (*PaymentIntentStatusHistoryRepository)(nil)

func (r *PaymentIntentStatusHistoryRepository) Create(_ context.Context, h *domain.PaymentIntentStatusHistory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, h)
	return nil
}

func (r *PaymentIntentStatusHistoryRepository) ListByPaymentIntentID(_ context.Context, id string) ([]*domain.PaymentIntentStatusHistory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.PaymentIntentStatusHistory
	for _, h := range r.entries {
		if h.PaymentIntentID == id {
			out = append(out, h)
		}
	}
	return out, nil
}
