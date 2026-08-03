package http_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	transporthttp "github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/transport/http"
)

func TestDocsUI_ReturnsHTML(t *testing.T) {
	handler := transporthttp.NewDocsHandler("no-importa-para-este-test.yaml")
	app := transporthttp.NewRouter(nil, nil, nil, nil, handler, nil)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/docs", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "SwaggerUIBundle")
}

func TestDocsSpec_ServesTheYAMLFile(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "spec-*.yaml")
	require.NoError(t, err)
	_, err = tmp.WriteString("openapi: 3.0.3\n")
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	handler := transporthttp.NewDocsHandler(tmp.Name())
	app := transporthttp.NewRouter(nil, nil, nil, nil, handler, nil)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "openapi: 3.0.3\n", string(body))
}

func TestDocsSpec_MissingFile_ReturnsInternalError(t *testing.T) {
	handler := transporthttp.NewDocsHandler("no-existe.yaml")
	app := transporthttp.NewRouter(nil, nil, nil, nil, handler, nil)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil), -1)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}
