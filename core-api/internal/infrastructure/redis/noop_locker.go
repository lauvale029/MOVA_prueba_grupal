// Package redis implementa IdempotencyLocker contra Redis.
//
// TODO(Eduard): reemplazar NoopIdempotencyLocker por un lock real
// (SET NX + TTL). Mientras tanto el sistema sigue siendo correcto: la
// restricción única de idempotency_key en Postgres es la garantía final
// (ver ADR-0002); esto solo evita, como optimización, que dos requests
// concurrentes golpeen la base al mismo tiempo con la misma key.
package redis

import (
	"context"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
)

type NoopIdempotencyLocker struct{}

var _ application.IdempotencyLocker = NoopIdempotencyLocker{}

func (NoopIdempotencyLocker) Acquire(_ context.Context, _ string) (func(), bool) {
	return func() {}, true
}
