package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

type PaymentIntentHandler struct {
	service *application.PaymentIntentService
}

func NewPaymentIntentHandler(service *application.PaymentIntentService) *PaymentIntentHandler {
	return &PaymentIntentHandler{service: service}
}

type createPaymentIntentRequest struct {
	MerchantID        string `json:"merchant_id"`
	ExternalReference string `json:"external_reference"`
	AmountMinor       int64  `json:"amount_minor"`
	Currency          string `json:"currency"`
	Channel           string `json:"channel"`
}

type paymentIntentResponse struct {
	ID                string   `json:"id"`
	MerchantID        string   `json:"merchant_id"`
	ExternalReference string   `json:"external_reference"`
	AmountMinor       int64    `json:"amount_minor"`
	Currency          string   `json:"currency"`
	Channel           string   `json:"channel"`
	Status            string   `json:"status"`
	RiskDecision      *string  `json:"risk_decision"`
	RiskScore         *int     `json:"risk_score"`
	RiskReasonCodes   []string `json:"risk_reason_codes"`
	CorrelationID     string   `json:"correlation_id"`
	ExpiresAt         string   `json:"expires_at"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

func toPaymentIntentResponse(pi *domain.PaymentIntent) paymentIntentResponse {
	resp := paymentIntentResponse{
		ID:                pi.ID,
		MerchantID:        pi.MerchantID,
		ExternalReference: pi.ExternalReference,
		AmountMinor:       pi.AmountMinor,
		Currency:          pi.Currency,
		Channel:           string(pi.Channel),
		Status:            string(pi.Status),
		RiskReasonCodes:   pi.RiskReasonCodes,
		CorrelationID:     pi.CorrelationID,
		ExpiresAt:         pi.ExpiresAt.Format(timeFormat),
		CreatedAt:         pi.CreatedAt.Format(timeFormat),
		UpdatedAt:         pi.UpdatedAt.Format(timeFormat),
	}
	if pi.RiskDecision != nil {
		v := string(*pi.RiskDecision)
		resp.RiskDecision = &v
	}
	resp.RiskScore = pi.RiskScore
	return resp
}

const timeFormat = "2006-01-02T15:04:05Z07:00"

// Create crea un PaymentIntent. Requiere el header Idempotency-Key.
func (h *PaymentIntentHandler) Create(c *fiber.Ctx) error {
	var req createPaymentIntentRequest
	if err := c.BodyParser(&req); err != nil {
		return errorResponse(c, fiber.StatusBadRequest, "INVALID_REQUEST_BODY", "el cuerpo de la petición no es un JSON válido")
	}

	idempotencyKey := c.Get("Idempotency-Key")
	correlationID := c.Get("X-Correlation-Id")

	pi, err := h.service.Create(
		c.Context(),
		req.MerchantID, req.ExternalReference,
		req.AmountMinor, req.Currency, domain.Channel(req.Channel),
		idempotencyKey, correlationID, changedBy(c),
	)
	if err != nil {
		return handleError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(toPaymentIntentResponse(pi))
}

func (h *PaymentIntentHandler) Get(c *fiber.Ctx) error {
	pi, err := h.service.Get(c.Context(), c.Params("payment_intent_id"))
	if err != nil {
		return handleError(c, err)
	}
	return c.JSON(toPaymentIntentResponse(pi))
}

func (h *PaymentIntentHandler) List(c *fiber.Ctx) error {
	filter := application.PaymentIntentFilter{
		MerchantID: c.Query("merchant_id"),
		Status:     domain.Status(c.Query("status")),
		Page:       c.QueryInt("page", application.DefaultPage),
		Limit:      c.QueryInt("limit", application.DefaultLimit),
	}

	items, total, err := h.service.List(c.Context(), filter)
	if err != nil {
		return handleError(c, err)
	}

	data := make([]paymentIntentResponse, 0, len(items))
	for _, pi := range items {
		data = append(data, toPaymentIntentResponse(pi))
	}

	return c.JSON(fiber.Map{
		"data":  data,
		"page":  filter.Page,
		"limit": filter.Limit,
		"total": total,
	})
}

type historyEntryResponse struct {
	ID              string `json:"id"`
	PaymentIntentID string `json:"payment_intent_id"`
	PreviousStatus  string `json:"previous_status"`
	NewStatus       string `json:"new_status"`
	Reason          string `json:"reason"`
	ChangedBy       string `json:"changed_by"`
	CorrelationID   string `json:"correlation_id"`
	CreatedAt       string `json:"created_at"`
}

func (h *PaymentIntentHandler) History(c *fiber.Ctx) error {
	entries, err := h.service.History(c.Context(), c.Params("payment_intent_id"))
	if err != nil {
		return handleError(c, err)
	}

	data := make([]historyEntryResponse, 0, len(entries))
	for _, e := range entries {
		data = append(data, historyEntryResponse{
			ID: e.ID, PaymentIntentID: e.PaymentIntentID,
			PreviousStatus: string(e.PreviousStatus), NewStatus: string(e.NewStatus),
			Reason: e.Reason, ChangedBy: e.ChangedBy, CorrelationID: e.CorrelationID,
			CreatedAt: e.CreatedAt.Format(timeFormat),
		})
	}
	return c.JSON(data)
}
