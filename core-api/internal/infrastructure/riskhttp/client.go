// Package riskhttp implementa el camino de emergencia: si Kafka mismo
// falla (no risk-service), core-api llama directo al endpoint síncrono
// que risk-service ya expone para pruebas y demos (ver ADR-0008).
package riskhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

type Client struct {
	baseURL string
	client  *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

type evaluateRequest struct {
	PaymentIntentID       string `json:"payment_intent_id"`
	MerchantID            string `json:"merchant_id"`
	ExternalReference     string `json:"external_reference"`
	AmountMinor           int64  `json:"amount_minor"`
	Currency              string `json:"currency"`
	Channel               string `json:"channel"`
	MerchantStatus        string `json:"merchant_status"`
	MerchantRecentIntents int    `json:"merchant_recent_intents"`
}

type evaluateResponse struct {
	PaymentIntentID string   `json:"payment_intent_id"`
	Decision        string   `json:"decision"`
	Score           int      `json:"score"`
	ReasonCodes     []string `json:"reason_codes"`
	ModelVersion    string   `json:"model_version"`
}

// Evaluate le pide la decisión directo a risk-service, síncrono, vía
// POST /api/v1/risk-evaluations — el mismo endpoint que ya usan
// scripts/demo.sh y casos-de-prueba.sh para probar las reglas sin Kafka.
func (c *Client) Evaluate(ctx context.Context, event application.RiskEvaluationRequested) (*application.RiskEvaluationResult, error) {
	payload, err := json.Marshal(evaluateRequest{
		PaymentIntentID:       event.PaymentIntentID,
		MerchantID:            event.MerchantID,
		ExternalReference:     event.ExternalReference,
		AmountMinor:           event.AmountMinor,
		Currency:              event.Currency,
		Channel:               event.Channel,
		MerchantStatus:        event.MerchantStatus,
		MerchantRecentIntents: event.MerchantRecentIntents,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/risk-evaluations", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("risk-service respondió %d", resp.StatusCode)
	}

	var out evaluateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	return &application.RiskEvaluationResult{
		Decision:     domain.RiskDecision(out.Decision),
		Score:        out.Score,
		ReasonCodes:  out.ReasonCodes,
		ModelVersion: out.ModelVersion,
	}, nil
}
