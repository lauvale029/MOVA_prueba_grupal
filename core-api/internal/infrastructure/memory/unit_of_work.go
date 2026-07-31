package memory

import (
	"context"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
)

// UnitOfWork sin transacción real (no hay un *sql.DB todavía). Eduard la
// reemplaza por la versión con *sql.Tx cuando llegue Postgres.
type UnitOfWork struct{}

var _ application.UnitOfWork = UnitOfWork{}

func (UnitOfWork) Execute(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
