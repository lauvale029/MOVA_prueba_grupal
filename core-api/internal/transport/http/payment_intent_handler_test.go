package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
)

func createRequest(t *testing.T, ta *testApp, body map[string]any, idempotencyKey string) *http.Request {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment-intents", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ta.token)
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return req
}

func authedGet(ta *testApp, path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+ta.token)
	return req
}

func validBody() map[string]any {
	return map[string]any{
		"merchant_id":        "merchant-1",
		"external_reference": "order-1",
		"amount_minor":       15_000_00,
		"currency":           "COP",
		"channel":            "QR",
	}
}

func decodeJSON(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
}

func TestCreatePaymentIntent_Success(t *testing.T) {
	ta := setupApp()
	req := createRequest(t, ta, validBody(), "idem-1")

	resp, err := ta.app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]any
	decodeJSON(t, resp, &body)
	assert.Equal(t, "UNDER_REVIEW", body["status"], "se envió a evaluación de riesgo de inmediato")
	assert.NotEmpty(t, body["id"])
	assert.NotEmpty(t, body["correlation_id"])
}

func TestCreatePaymentIntent_MissingIdempotencyKey(t *testing.T) {
	ta := setupApp()
	req := createRequest(t, ta, validBody(), "")

	resp, err := ta.app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestCreatePaymentIntent_InvalidBody(t *testing.T) {
	ta := setupApp()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment-intents", bytes.NewReader([]byte("no-es-json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ta.token)
	req.Header.Set("Idempotency-Key", "idem-1")

	resp, err := ta.app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreatePaymentIntent_MissingToken_Returns401(t *testing.T) {
	ta := setupApp()
	payload, _ := json.Marshal(validBody())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment-intents", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "idem-1")

	resp, err := ta.app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestCreatePaymentIntent_Retry_ReturnsSameIntent(t *testing.T) {
	ta := setupApp()

	first, err := ta.app.Test(createRequest(t, ta, validBody(), "idem-1"), -1)
	require.NoError(t, err)
	var firstBody map[string]any
	decodeJSON(t, first, &firstBody)

	second, err := ta.app.Test(createRequest(t, ta, validBody(), "idem-1"), -1)
	require.NoError(t, err)
	var secondBody map[string]any
	decodeJSON(t, second, &secondBody)

	assert.Equal(t, firstBody["id"], secondBody["id"])

	total, err := ta.payments.Count(context.Background(), application.PaymentIntentFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
}

func TestGetPaymentIntent_Success(t *testing.T) {
	ta := setupApp()
	created, _ := ta.app.Test(createRequest(t, ta, validBody(), "idem-1"), -1)
	var createdBody map[string]any
	decodeJSON(t, created, &createdBody)
	id := createdBody["id"].(string)

	resp, err := ta.app.Test(authedGet(ta, "/api/v1/payment-intents/"+id), -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetPaymentIntent_NotFound(t *testing.T) {
	ta := setupApp()
	resp, err := ta.app.Test(authedGet(ta, "/api/v1/payment-intents/no-existe"), -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestListPaymentIntents_FiltersByMerchant(t *testing.T) {
	ta := setupApp()
	_, _ = ta.app.Test(createRequest(t, ta, validBody(), "idem-1"), -1)

	other := validBody()
	other["merchant_id"] = "merchant-2"
	other["external_reference"] = "order-2"
	_, _ = ta.app.Test(createRequest(t, ta, other, "idem-2"), -1)

	resp, err := ta.app.Test(authedGet(ta, "/api/v1/payment-intents?merchant_id=merchant-2"), -1)
	require.NoError(t, err)

	var body struct {
		Data  []map[string]any `json:"data"`
		Total int64            `json:"total"`
	}
	decodeJSON(t, resp, &body)
	require.Len(t, body.Data, 1)
	assert.Equal(t, "merchant-2", body.Data[0]["merchant_id"])
}

func TestGetPaymentIntentHistory_Success(t *testing.T) {
	ta := setupApp()
	created, _ := ta.app.Test(createRequest(t, ta, validBody(), "idem-1"), -1)
	var createdBody map[string]any
	decodeJSON(t, created, &createdBody)
	id := createdBody["id"].(string)

	resp, err := ta.app.Test(authedGet(ta, "/api/v1/payment-intents/"+id+"/history"), -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var entries []map[string]any
	decodeJSON(t, resp, &entries)
	require.Len(t, entries, 2, "creado + enviado a revisión")
	assert.Equal(t, "PENDING", entries[0]["new_status"])
	assert.Equal(t, "UNDER_REVIEW", entries[1]["new_status"])
}

func TestGetPaymentIntentHistory_NotFound(t *testing.T) {
	ta := setupApp()
	resp, err := ta.app.Test(authedGet(ta, "/api/v1/payment-intents/no-existe/history"), -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
