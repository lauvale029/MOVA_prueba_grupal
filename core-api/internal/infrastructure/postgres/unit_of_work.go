package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
)

// UnitOfWork abre una transacción real y la deja disponible en el
// context para que los repositorios la usen sin saber que existe (ver
// application.UnitOfWork).
type UnitOfWork struct {
	pool *pgxpool.Pool
}

func NewUnitOfWork(pool *pgxpool.Pool) *UnitOfWork {
	return &UnitOfWork{pool: pool}
}

var _ application.UnitOfWork = (*UnitOfWork)(nil)

func (u *UnitOfWork) Execute(ctx context.Context, fn func(context.Context) error) error {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return err
	}

	if err := fn(withTx(ctx, tx)); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}
