package kafka

import (
	"sync"
	"time"
)

type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

// circuitBreaker es el mismo diseño que ya usa el reconciliation-worker
// en Python (worker/infrastructure/resilience.py): uno por dependencia,
// nunca global. Su valor no es dejar de llamar, es fallar rápido hacia
// el estado seguro — con el breaker abierto, un intento inútil contra
// Kafka no gasta su timeout completo (ver ADR-0008).
type circuitBreaker struct {
	mu               sync.Mutex
	failureThreshold int
	resetTimeout     time.Duration
	now              func() time.Time
	failures         int
	openedAt         time.Time
	state            breakerState
}

func newCircuitBreaker(failureThreshold int, resetTimeout time.Duration) *circuitBreaker {
	return &circuitBreaker{
		failureThreshold: failureThreshold,
		resetTimeout:     resetTimeout,
		now:              time.Now,
		state:            breakerClosed,
	}
}

// allow dice si vale la pena intentar Kafka. Si el circuito lleva
// abierto más que resetTimeout, deja pasar una sonda (semi-abierto) en
// vez de seguir rechazando para siempre.
func (b *circuitBreaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == breakerOpen && b.now().Sub(b.openedAt) >= b.resetTimeout {
		b.state = breakerHalfOpen
	}
	return b.state != breakerOpen
}

func (b *circuitBreaker) onSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.state = breakerClosed
}

func (b *circuitBreaker) onFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == breakerHalfOpen {
		// La sonda falló: se vuelve a abrir sin esperar al umbral.
		b.trip()
		return
	}

	b.failures++
	if b.failures >= b.failureThreshold {
		b.trip()
	}
}

// trip asume el lock ya tomado por quien llama.
func (b *circuitBreaker) trip() {
	b.state = breakerOpen
	b.openedAt = b.now()
	b.failures = 0
}
