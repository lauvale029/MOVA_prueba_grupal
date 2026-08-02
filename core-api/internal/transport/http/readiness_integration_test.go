//go:build integration

package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	transporthttp "github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/transport/http"
)

func TestReadiness_KafkaReachable_ReturnsOK(t *testing.T) {
	handler := transporthttp.NewReadinessHandler("localhost:9092")
	app := transporthttp.NewRouter(nil, nil, handler, nil, nil) // no se usan en este test

	req := httptest.NewRequest(http.MethodGet, "/readiness", nil)
	resp, err := app.Test(req, -1)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
