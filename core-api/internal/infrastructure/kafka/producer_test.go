package kafka_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/kafka"
)

type fakeFallback struct {
	calls  int
	result *application.RiskEvaluationResult
	err    error
}

func (f *fakeFallback) Evaluate(_ context.Context, _ application.RiskEvaluationRequested) (*application.RiskEvaluationResult, error) {
	f.calls++
	return f.result, f.err
}

// TestRiskRequestPublisher_FallsBackToHTTP_WhenKafkaUnreachable no
// necesita un broker real a propósito: localhost:1 no tiene nada
// escuchando, así que simula a Kafka fallando de entrada (ver ADR-0008).
func TestRiskRequestPublisher_FallsBackToHTTP_WhenKafkaUnreachable(t *testing.T) {
	fallback := &fakeFallback{result: &application.RiskEvaluationResult{
		Decision: domain.RiskApprove, Score: 5, ReasonCodes: []string{"LOW_RISK"}, ModelVersion: "rules-v2",
	}}
	publisher := kafka.NewRiskRequestPublisher([]string{"localhost:1"}, fallback)
	defer publisher.Close()

	result, err := publisher.Publish(context.Background(), application.RiskEvaluationRequested{PaymentIntentID: "pi-1"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, domain.RiskApprove, result.Decision)
	assert.Equal(t, 1, fallback.calls)
}

func TestRiskRequestPublisher_NoFallbackConfigured_ReturnsError(t *testing.T) {
	publisher := kafka.NewRiskRequestPublisher([]string{"localhost:1"}, nil)
	defer publisher.Close()

	_, err := publisher.Publish(context.Background(), application.RiskEvaluationRequested{PaymentIntentID: "pi-1"})
	assert.Error(t, err)
}
