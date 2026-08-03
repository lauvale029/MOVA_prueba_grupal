package riskhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/riskhttp"
)

func TestClient_Evaluate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/risk-evaluations", r.URL.Path)

		var got map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		assert.Equal(t, "pi-1", got["payment_intent_id"])
		assert.Equal(t, "ACTIVE", got["merchant_status"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"payment_intent_id": "pi-1", "decision": "APPROVE", "score": 5,
			"reason_codes": []string{"LOW_RISK"}, "model_version": "rules-v2",
		})
	}))
	defer server.Close()

	client := riskhttp.NewClient(server.URL)
	result, err := client.Evaluate(context.Background(), application.RiskEvaluationRequested{
		PaymentIntentID: "pi-1", MerchantStatus: "ACTIVE",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.RiskApprove, result.Decision)
	assert.Equal(t, 5, result.Score)
	assert.Equal(t, []string{"LOW_RISK"}, result.ReasonCodes)
	assert.Equal(t, "rules-v2", result.ModelVersion)
}

func TestClient_Evaluate_NonOKStatus_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	client := riskhttp.NewClient(server.URL)
	_, err := client.Evaluate(context.Background(), application.RiskEvaluationRequested{PaymentIntentID: "pi-1"})
	assert.Error(t, err)
}

func TestClient_Evaluate_Unreachable_ReturnsError(t *testing.T) {
	client := riskhttp.NewClient("http://localhost:1")
	_, err := client.Evaluate(context.Background(), application.RiskEvaluationRequested{PaymentIntentID: "pi-1"})
	assert.Error(t, err)
}
