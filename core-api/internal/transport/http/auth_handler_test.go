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

func loginRequest(username, password string) *http.Request {
	payload, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestLogin_ValidCredentials_ReturnsToken(t *testing.T) {
	ta := setupApp()

	resp, err := ta.app.Test(loginRequest(testAuthUsername, testAuthPassword), -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	decodeJSON(t, resp, &body)
	assert.NotEmpty(t, body["token"])
	assert.NotEmpty(t, body["expires_at"])
}

func TestLogin_InvalidCredentials_Returns401(t *testing.T) {
	ta := setupApp()

	resp, err := ta.app.Test(loginRequest(testAuthUsername, "contraseña-incorrecta"), -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestLogin_InvalidBody_Returns400(t *testing.T) {
	ta := setupApp()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte("no-es-json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ta.app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
