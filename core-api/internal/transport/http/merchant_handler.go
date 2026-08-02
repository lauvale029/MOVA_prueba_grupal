package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

type MerchantHandler struct {
	service *application.MerchantService
}

func NewMerchantHandler(service *application.MerchantService) *MerchantHandler {
	return &MerchantHandler{service: service}
}

type createMerchantRequest struct {
	Name           string `json:"name"`
	DocumentNumber string `json:"document_number"`
	Email          string `json:"email"`
}

type merchantResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	DocumentNumber string `json:"document_number"`
	Email          string `json:"email"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func toMerchantResponse(m *domain.Merchant) merchantResponse {
	return merchantResponse{
		ID:             m.ID,
		Name:           m.Name,
		DocumentNumber: m.DocumentNumber,
		Email:          m.Email,
		Status:         string(m.Status),
		CreatedAt:      m.CreatedAt.Format(timeFormat),
		UpdatedAt:      m.UpdatedAt.Format(timeFormat),
	}
}

func (h *MerchantHandler) Create(c *fiber.Ctx) error {
	var req createMerchantRequest
	if err := c.BodyParser(&req); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, "INVALID_REQUEST_BODY", "el cuerpo de la petición no es un JSON válido")
	}

	m, err := h.service.Create(c.Context(), req.Name, req.DocumentNumber, req.Email)
	if err != nil {
		return handleError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(toMerchantResponse(m))
}

func (h *MerchantHandler) Get(c *fiber.Ctx) error {
	m, err := h.service.Get(c.Context(), c.Params("merchant_id"))
	if err != nil {
		return handleError(c, err)
	}
	return c.JSON(toMerchantResponse(m))
}
