package postgres

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

type PaymentIntentRepository struct {
	pool *pgxpool.Pool
}

func NewPaymentIntentRepository(pool *pgxpool.Pool) *PaymentIntentRepository {
	return &PaymentIntentRepository{pool: pool}
}

var _ application.PaymentIntentRepository = (*PaymentIntentRepository)(nil)

func (r *PaymentIntentRepository) Create(ctx context.Context, pi *domain.PaymentIntent) error {
	db := dbFromContext(ctx, r.pool)
	_, err := db.Exec(ctx, `
		INSERT INTO payments.payment_intents (
			id, merchant_id, external_reference, idempotency_key,
			amount_minor, currency, channel, status,
			risk_decision, risk_score, risk_reason_codes, risk_model_version,
			correlation_id, expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`,
		pi.ID, pi.MerchantID, pi.ExternalReference, pi.IdempotencyKey,
		pi.AmountMinor, pi.Currency, string(pi.Channel), string(pi.Status),
		riskDecisionParam(pi.RiskDecision), pi.RiskScore, pi.RiskReasonCodes, pi.RiskModelVersion,
		pi.CorrelationID, pi.ExpiresAt, pi.CreatedAt, pi.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return application.ErrConflict
		}
		return err
	}
	return nil
}

func (r *PaymentIntentRepository) GetByID(ctx context.Context, id string) (*domain.PaymentIntent, error) {
	return r.scanOne(ctx, `WHERE id = $1`, id)
}

func (r *PaymentIntentRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.PaymentIntent, error) {
	return r.scanOne(ctx, `WHERE idempotency_key = $1`, key)
}

func (r *PaymentIntentRepository) scanOne(ctx context.Context, where string, arg any) (*domain.PaymentIntent, error) {
	db := dbFromContext(ctx, r.pool)
	row := db.QueryRow(ctx, paymentIntentSelect+where, arg)
	pi, err := scanPaymentIntent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrNotFound
		}
		return nil, err
	}
	return pi, nil
}

func (r *PaymentIntentRepository) Update(ctx context.Context, pi *domain.PaymentIntent) error {
	db := dbFromContext(ctx, r.pool)
	tag, err := db.Exec(ctx, `
		UPDATE payments.payment_intents SET
			status = $2, risk_decision = $3, risk_score = $4,
			risk_reason_codes = $5, risk_model_version = $6, updated_at = $7
		WHERE id = $1
	`,
		pi.ID, string(pi.Status), riskDecisionParam(pi.RiskDecision),
		pi.RiskScore, pi.RiskReasonCodes, pi.RiskModelVersion, pi.UpdatedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return application.ErrNotFound
	}
	return nil
}

func (r *PaymentIntentRepository) List(ctx context.Context, filter application.PaymentIntentFilter) ([]*domain.PaymentIntent, error) {
	db := dbFromContext(ctx, r.pool)
	where, args := filterClause(filter)
	args = append(args, filter.Limit, (filter.Page-1)*filter.Limit)

	rows, err := db.Query(ctx, paymentIntentSelect+where+
		` ORDER BY created_at DESC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*domain.PaymentIntent, 0)
	for rows.Next() {
		pi, err := scanPaymentIntent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, pi)
	}
	return items, rows.Err()
}

func (r *PaymentIntentRepository) Count(ctx context.Context, filter application.PaymentIntentFilter) (int64, error) {
	db := dbFromContext(ctx, r.pool)
	where, args := filterClause(filter)

	var total int64
	err := db.QueryRow(ctx, `SELECT count(*) FROM payments.payment_intents`+where, args...).Scan(&total)
	return total, err
}

// CountRecentByMerchant usa ix_intents_merchant_recent (migración 0002),
// pensado justo para esta consulta.
func (r *PaymentIntentRepository) CountRecentByMerchant(ctx context.Context, merchantID string, since time.Time) (int64, error) {
	db := dbFromContext(ctx, r.pool)
	var total int64
	err := db.QueryRow(ctx, `
		SELECT count(*) FROM payments.payment_intents
		WHERE merchant_id = $1 AND created_at >= $2
	`, merchantID, since).Scan(&total)
	return total, err
}

// filterClause arma el WHERE y sus argumentos a partir del filtro. Vacío
// siempre matchea todo (igual que el repo en memoria).
func filterClause(filter application.PaymentIntentFilter) (string, []any) {
	var conds []string
	var args []any

	if filter.MerchantID != "" {
		args = append(args, filter.MerchantID)
		conds = append(conds, `merchant_id = $`+strconv.Itoa(len(args)))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		conds = append(conds, `status = $`+strconv.Itoa(len(args)))
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

const paymentIntentSelect = `
	SELECT id, merchant_id, external_reference, idempotency_key,
	       amount_minor, currency, channel, status,
	       risk_decision, risk_score, risk_reason_codes, risk_model_version,
	       correlation_id, expires_at, created_at, updated_at
	FROM payments.payment_intents
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPaymentIntent(row rowScanner) (*domain.PaymentIntent, error) {
	var pi domain.PaymentIntent
	var channel, status string
	var riskDecision *string

	err := row.Scan(
		&pi.ID, &pi.MerchantID, &pi.ExternalReference, &pi.IdempotencyKey,
		&pi.AmountMinor, &pi.Currency, &channel, &status,
		&riskDecision, &pi.RiskScore, &pi.RiskReasonCodes, &pi.RiskModelVersion,
		&pi.CorrelationID, &pi.ExpiresAt, &pi.CreatedAt, &pi.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	pi.Channel = domain.Channel(channel)
	pi.Status = domain.Status(status)
	if riskDecision != nil {
		d := domain.RiskDecision(*riskDecision)
		pi.RiskDecision = &d
	}
	return &pi, nil
}

func riskDecisionParam(d *domain.RiskDecision) *string {
	if d == nil {
		return nil
	}
	s := string(*d)
	return &s
}
