// Package redis implementa IdempotencyLocker contra Redis: un lock
// best-effort, no la garantía final. La restricción única de
// idempotency_key en Postgres es la que de verdad impide duplicados si
// este lock falla, expira antes de tiempo, o Redis no está disponible
// (ver ADR-0002).
package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
)

// lockTTL es cuánto dura el lock si nunca se libera explícitamente (ej.
// el proceso se cae a mitad de camino). Evita que un lock quede
// "trabado" para siempre.
const lockTTL = 5 * time.Second

type IdempotencyLocker struct {
	client *redis.Client
}

func NewIdempotencyLocker(client *redis.Client) *IdempotencyLocker {
	return &IdempotencyLocker{client: client}
}

var _ application.IdempotencyLocker = (*IdempotencyLocker)(nil)

// releaseScript borra el lock SOLO si el valor guardado coincide con el
// token que pusimos al tomarlo. Sin esto, un release tardío podría
// borrar el lock de OTRO proceso que lo haya tomado después de que el
// nuestro ya expiró por TTL.
var releaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
else
	return 0
end
`)

func (l *IdempotencyLocker) Acquire(ctx context.Context, key string) (func(), bool) {
	noop := func() {}

	token, err := randomToken()
	if err != nil {
		return noop, false
	}

	lockKey := "idempotency-lock:" + key
	acquired, err := l.client.SetNX(ctx, lockKey, token, lockTTL).Result()
	if err != nil || !acquired {
		return noop, false
	}

	release := func() {
		// Contexto propio: si el ctx original ya se canceló (la petición
		// HTTP ya terminó), igual queremos liberar el lock.
		releaseCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		releaseScript.Run(releaseCtx, l.client, []string{lockKey}, token)
	}
	return release, true
}

func randomToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
