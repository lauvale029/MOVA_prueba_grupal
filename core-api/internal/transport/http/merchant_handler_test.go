package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createMerchantRequest(t *testing.T, ta *testApp, body map[string]any) *http.Request {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/merchants", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ta.token)
	return req
}

func validMerchantBody() map[string]any {
	return map[string]any{
		"name":            "Tienda Ejemplo",
		"document_number": "900123456-7",
		"email":           "contacto@tienda-ejemplo.com",
	}
}

func TestCreateMerchant_Success(t *testing.T) {
	ta := setupApp()
	req := createMerchantRequest(t, ta, validMerchantBody())

	resp, err := ta.app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]any
	decodeJSON(t, resp, &body)
	assert.Equal(t, "ACTIVE", body["status"])
	assert.Equal(t, "Tienda Ejemplo", body["name"])
	assert.NotEmpty(t, body["id"])
}

func TestCreateMerchant_InvalidBody(t *testing.T) {
	ta := setupApp()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/merchants", bytes.NewReader([]byte("no-es-json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ta.token)

	resp, err := ta.app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateMerchant_MissingFields(t *testing.T) {
	ta := setupApp()
	req := createMerchantRequest(t, ta, map[string]any{"name": "", "document_number": "", "email": ""})

	resp, err := ta.app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestCreateMerchant_DuplicateDocumentNumber(t *testing.T) {
	ta := setupApp()
	first := createMerchantRequest(t, ta, validMerchantBody())
	resp, err := ta.app.Test(first, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	second := createMerchantRequest(t, ta, validMerchantBody())
	resp, err = ta.app.Test(second, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestGetMerchant_Success(t *testing.T) {
	ta := setupApp()
	createResp, err := ta.app.Test(createMerchantRequest(t, ta, validMerchantBody()), -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var created map[string]any
	decodeJSON(t, createResp, &created)

	resp, err := ta.app.Test(authedGet(ta, "/api/v1/merchants/"+created["id"].(string)))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	decodeJSON(t, resp, &body)
	assert.Equal(t, created["id"], body["id"])
}

func TestGetMerchant_NotFound(t *testing.T) {
	ta := setupApp()
	resp, err := ta.app.Test(authedGet(ta, "/api/v1/merchants/no-existe"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
