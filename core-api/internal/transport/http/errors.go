package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func errorResponse(c *fiber.Ctx, status int, code, message string) error {
	body := errorBody{}
	body.Error.Code = code
	body.Error.Message = message
	return c.Status(status).JSON(body)
}

// handleError mapea errores de dominio/aplicación al código HTTP correcto.
func handleError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, application.ErrNotFound):
		return errorResponse(c, fiber.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, application.ErrConflict):
		return errorResponse(c, fiber.StatusConflict, "CONFLICT", err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		return errorResponse(c, fiber.StatusConflict, "INVALID_TRANSITION", err.Error())
	case errors.Is(err, domain.ErrMissingIdempotencyKey),
		errors.Is(err, domain.ErrMissingMerchantID),
		errors.Is(err, domain.ErrMissingExternalReference),
		errors.Is(err, domain.ErrInvalidAmount),
		errors.Is(err, domain.ErrInvalidCurrency),
		errors.Is(err, domain.ErrInvalidChannel):
		return errorResponse(c, fiber.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
	default:
		return errorResponse(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "error interno")
	}
}
