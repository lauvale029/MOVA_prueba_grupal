package redis

import "github.com/redis/go-redis/v9"

// NewClient crea un cliente de Redis. La conexión real ocurre de forma
// perezosa en el primer comando (igual que database/sql), así que esta
// función nunca falla por sí sola: si Redis no está disponible, el primer
// intento de usarlo simplemente devuelve un error, que
// IdempotencyLocker.Acquire trata como "no conseguí el lock" — nunca como
// un fallo duro (ver ADR-0002).
func NewClient(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr})
}
