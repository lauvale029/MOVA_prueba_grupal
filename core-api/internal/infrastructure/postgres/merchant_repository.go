package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

// uniqueViolationCode es el SQLSTATE que Postgres devuelve cuando una
// restricción UNIQUE se viola (ver docs/esquema-de-datos.md §8).
const uniqueViolationCode = "23505"

type MerchantRepository struct {
	pool *pgxpool.Pool
}

func NewMerchantRepository(pool *pgxpool.Pool) *MerchantRepository {
	return &MerchantRepository{pool: pool}
}

var _ application.MerchantRepository = (*MerchantRepository)(nil)

func (r *MerchantRepository) Create(ctx context.Context, m *domain.Merchant) error {
	db := dbFromContext(ctx, r.pool)
	_, err := db.Exec(ctx, `
		INSERT INTO payments.merchants (id, name, document_number, email, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, m.ID, m.Name, m.DocumentNumber, m.Email, string(m.Status), m.CreatedAt, m.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return application.ErrConflict
		}
		return err
	}
	return nil
}

func (r *MerchantRepository) GetByID(ctx context.Context, id string) (*domain.Merchant, error) {
	db := dbFromContext(ctx, r.pool)
	row := db.QueryRow(ctx, `
		SELECT id, name, document_number, email, status, created_at, updated_at
		FROM payments.merchants WHERE id = $1
	`, id)

	var m domain.Merchant
	var status string
	err := row.Scan(&m.ID, &m.Name, &m.DocumentNumber, &m.Email, &status, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, application.ErrNotFound
		}
		return nil, err
	}
	m.Status = domain.MerchantStatus(status)
	return &m, nil
}
