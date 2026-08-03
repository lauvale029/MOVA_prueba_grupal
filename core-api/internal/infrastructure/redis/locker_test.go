//go:build integration

package redis_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	redisinfra "github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/redis"
)

func redisAddr(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR no está configurada")
	}
	return addr
}

func TestIdempotencyLocker_SecondAcquireFailsUntilReleased(t *testing.T) {
	client := redisinfra.NewClient(redisAddr(t))
	defer client.Close()
	locker := redisinfra.NewIdempotencyLocker(client)

	key := "test-" + uuid.New().String()
	ctx := context.Background()

	release, acquired := locker.Acquire(ctx, key)
	require.True(t, acquired, "el primero sí debería conseguir el lock")

	_, acquiredAgain := locker.Acquire(ctx, key)
	require.False(t, acquiredAgain, "un segundo intento con la misma key debe fallar mientras el lock siga tomado")

	release()

	_, acquiredAfterRelease := locker.Acquire(ctx, key)
	require.True(t, acquiredAfterRelease, "tras liberar, la misma key vuelve a estar disponible")
}

func TestIdempotencyLocker_DifferentKeysDontCollide(t *testing.T) {
	client := redisinfra.NewClient(redisAddr(t))
	defer client.Close()
	locker := redisinfra.NewIdempotencyLocker(client)
	ctx := context.Background()

	_, acquiredA := locker.Acquire(ctx, "test-a-"+uuid.New().String())
	_, acquiredB := locker.Acquire(ctx, "test-b-"+uuid.New().String())

	require.True(t, acquiredA)
	require.True(t, acquiredB)
}
