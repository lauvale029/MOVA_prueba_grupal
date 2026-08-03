//go:build integration

package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/postgres"
	transporthttp "github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/transport/http"
)

func TestReadiness_KafkaAndPostgresReachable_ReturnsOK(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL no está configurada")
	}
	pool, err := postgres.NewPool(context.Background(), databaseURL)
	require.NoError(t, err)
	defer pool.Close()

	handler := transporthttp.NewReadinessHandler("localhost:9092", pool)
	app := transporthttp.NewRouter(nil, nil, handler, nil, nil, nil) // no se usan en este test

	req := httptest.NewRequest(http.MethodGet, "/readiness", nil)
	resp, err := app.Test(req, -1)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
