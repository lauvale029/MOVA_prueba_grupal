package http_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	transporthttp "github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/transport/http"
)

type failingPinger struct{}

func (failingPinger) Ping(_ context.Context) error { return errors.New("boom") }

func TestReadiness_KafkaUnreachable_ReturnsServiceUnavailable(t *testing.T) {
	handler := transporthttp.NewReadinessHandler("localhost:1", okPinger{}) // puerto sin nada escuchando
	app := transporthttp.NewRouter(nil, nil, handler, nil, nil)             // no se usan en este test

	req := httptest.NewRequest(http.MethodGet, "/readiness", nil)
	resp, err := app.Test(req, -1)

	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestReadiness_PostgresUnreachable_ReturnsServiceUnavailable(t *testing.T) {
	// Kafka sí "responde": el check solo abre una conexión TCP, no habla
	// el protocolo, así que cualquier socket escuchando alcanza.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	handler := transporthttp.NewReadinessHandler(ln.Addr().String(), failingPinger{})
	app := transporthttp.NewRouter(nil, nil, handler, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/readiness", nil)
	resp, err := app.Test(req, -1)

	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
