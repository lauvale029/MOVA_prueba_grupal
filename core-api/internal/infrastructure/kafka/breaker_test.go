package kafka

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	b := newCircuitBreaker(3, time.Minute)

	require.True(t, b.allow())
	b.onFailure()
	require.True(t, b.allow())
	b.onFailure()
	assert.True(t, b.allow(), "todavía no llega al umbral")

	b.onFailure() // tercer fallo seguido -> abre
	assert.False(t, b.allow(), "al llegar al umbral debe abrirse")
}

func TestCircuitBreaker_HalfOpenAfterResetTimeout(t *testing.T) {
	now := time.Now()
	b := newCircuitBreaker(1, 10*time.Second)
	b.now = func() time.Time { return now }

	b.onFailure() // abre con un solo fallo
	require.False(t, b.allow())

	now = now.Add(11 * time.Second)
	assert.True(t, b.allow(), "pasado el reset timeout debe dejar pasar una sonda")
}

func TestCircuitBreaker_HalfOpenProbeFails_ReopensImmediately(t *testing.T) {
	now := time.Now()
	b := newCircuitBreaker(1, 10*time.Second)
	b.now = func() time.Time { return now }

	b.onFailure() // abre con un solo fallo (umbral 1)
	require.False(t, b.allow())

	now = now.Add(11 * time.Second)
	require.True(t, b.allow()) // semi-abierto, deja pasar la sonda

	b.onFailure() // la sonda también falla
	assert.False(t, b.allow(), "una sonda fallida reabre sin esperar el umbral de nuevo")
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	b := newCircuitBreaker(3, time.Minute)

	b.onFailure()
	b.onFailure()
	b.onSuccess()
	b.onFailure()
	b.onFailure()
	assert.True(t, b.allow(), "el éxito reinició el conteo: dos fallos más no alcanzan el umbral")
}
