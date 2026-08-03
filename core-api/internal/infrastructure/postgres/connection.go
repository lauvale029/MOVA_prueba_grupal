// Package postgres implementa los repositorios reales contra el esquema
// `payments` (ver docs/esquema-de-datos.md). core-api conecta como
// mova_app — sin DELETE ni DDL (ver ADR-0007).
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("creando el pool de postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conectando a postgres: %w", err)
	}
	return pool, nil
}
