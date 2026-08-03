package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

type PaymentIntentStatusHistoryRepository struct {
	pool *pgxpool.Pool
}

func NewPaymentIntentStatusHistoryRepository(pool *pgxpool.Pool) *PaymentIntentStatusHistoryRepository {
	return &PaymentIntentStatusHistoryRepository{pool: pool}
}

var _ application.PaymentIntentStatusHistoryRepository = (*PaymentIntentStatusHistoryRepository)(nil)

func (r *PaymentIntentStatusHistoryRepository) Create(ctx context.Context, h *domain.PaymentIntentStatusHistory) error {
	db := dbFromContext(ctx, r.pool)
	_, err := db.Exec(ctx, `
		INSERT INTO payments.payment_intent_status_history (
			id, payment_intent_id, previous_status, new_status, reason, changed_by, correlation_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		h.ID, h.PaymentIntentID, statusParam(h.PreviousStatus), string(h.NewStatus), h.Reason, h.ChangedBy, h.CorrelationID, h.CreatedAt,
	)
	return err
}

func (r *PaymentIntentStatusHistoryRepository) ListByPaymentIntentID(ctx context.Context, id string) ([]*domain.PaymentIntentStatusHistory, error) {
	db := dbFromContext(ctx, r.pool)
	rows, err := db.Query(ctx, `
		SELECT id, payment_intent_id, previous_status, new_status, reason, changed_by, correlation_id, created_at
		FROM payments.payment_intent_status_history
		WHERE payment_intent_id = $1
		ORDER BY created_at
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*domain.PaymentIntentStatusHistory, 0)
	for rows.Next() {
		var h domain.PaymentIntentStatusHistory
		var previousStatus *string
		var newStatus string

		if err := rows.Scan(&h.ID, &h.PaymentIntentID, &previousStatus, &newStatus, &h.Reason, &h.ChangedBy, &h.CorrelationID, &h.CreatedAt); err != nil {
			return nil, err
		}
		if previousStatus != nil {
			h.PreviousStatus = domain.Status(*previousStatus)
		}
		h.NewStatus = domain.Status(newStatus)
		out = append(out, &h)
	}
	return out, rows.Err()
}

// statusParam convierte el string vacío del dominio (sin estado previo, ver
// domain.NewPaymentIntentStatusHistory) en NULL para la columna nullable.
func statusParam(s domain.Status) *string {
	if s == "" {
		return nil
	}
	v := string(s)
	return &v
}
