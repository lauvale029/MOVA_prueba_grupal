package http

import (
	"os"

	"github.com/gofiber/fiber/v2"
)

// DocsHandler sirve el contrato OpenAPI y una página de Swagger UI que lo
// consume. Lee el archivo en cada request en vez de empotrarlo en el
// binario: docs/openapi/core-api-v1.yaml vive fuera del contexto de build
// de este módulo (raíz del repo), así que solo hay una copia real —
// montada como volumen de solo lectura en docker-compose.yml.
type DocsHandler struct {
	specPath string
}

func NewDocsHandler(specPath string) *DocsHandler {
	return &DocsHandler{specPath: specPath}
}

func (h *DocsHandler) Spec(c *fiber.Ctx) error {
	data, err := os.ReadFile(h.specPath)
	if err != nil {
		return errorResponse(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "no se pudo leer el contrato OpenAPI")
	}
	c.Set(fiber.HeaderContentType, "application/yaml")
	return c.Send(data)
}

func (h *DocsHandler) UI(c *fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return c.SendString(swaggerUIPage)
}

const swaggerUIPage = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>MOVA Core API — Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: "/docs/openapi.yaml",
        dom_id: "#swagger-ui",
      });
    };
  </script>
</body>
</html>
`
