package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

func validIntent(t *testing.T) *domain.PaymentIntent {
	t.Helper()
	pi, err := domain.NewPaymentIntent("merchant-1", "order-1", 15_000_00, "COP", domain.ChannelQR, "idem-1", "")
	require.NoError(t, err)
	return pi
}

func TestNewPaymentIntent_Valid(t *testing.T) {
	pi := validIntent(t)

	assert.Equal(t, domain.StatusPending, pi.Status)
	assert.NotEmpty(t, pi.ID)
	assert.NotEmpty(t, pi.CorrelationID)
	assert.True(t, pi.ExpiresAt.After(pi.CreatedAt))
}

func TestNewPaymentIntent_GeneratesCorrelationIDIfEmpty(t *testing.T) {
	pi := validIntent(t)
	assert.NotEmpty(t, pi.CorrelationID)
}

func TestNewPaymentIntent_KeepsGivenCorrelationID(t *testing.T) {
	pi, err := domain.NewPaymentIntent("merchant-1", "order-1", 1000, "COP", domain.ChannelQR, "idem-1", "corr-fijo")
	require.NoError(t, err)
	assert.Equal(t, "corr-fijo", pi.CorrelationID)
}

func TestNewPaymentIntent_InvalidCases(t *testing.T) {
	cases := []struct {
		name        string
		merchantID  string
		extRef      string
		amount      int64
		currency    string
		channel     domain.Channel
		idemKey     string
		expectedErr error
	}{
		{"sin merchant", "", "order-1", 1000, "COP", domain.ChannelQR, "idem-1", domain.ErrMissingMerchantID},
		{"sin external_reference", "merchant-1", "", 1000, "COP", domain.ChannelQR, "idem-1", domain.ErrMissingExternalReference},
		{"sin idempotency_key", "merchant-1", "order-1", 1000, "COP", domain.ChannelQR, "", domain.ErrMissingIdempotencyKey},
		{"monto cero", "merchant-1", "order-1", 0, "COP", domain.ChannelQR, "idem-1", domain.ErrInvalidAmount},
		{"monto negativo", "merchant-1", "order-1", -1, "COP", domain.ChannelQR, "idem-1", domain.ErrInvalidAmount},
		{"moneda no soportada", "merchant-1", "order-1", 1000, "USD", domain.ChannelQR, "idem-1", domain.ErrInvalidCurrency},
		{"canal no soportado", "merchant-1", "order-1", 1000, "COP", domain.Channel("NEQUI"), "idem-1", domain.ErrInvalidChannel},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := domain.NewPaymentIntent(tc.merchantID, tc.extRef, tc.amount, tc.currency, tc.channel, tc.idemKey, "")
			assert.ErrorIs(t, err, tc.expectedErr)
		})
	}
}

func TestCanTransitionTo(t *testing.T) {
	cases := []struct {
		from, to domain.Status
		allowed  bool
	}{
		{domain.StatusPending, domain.StatusUnderReview, true},
		{domain.StatusPending, domain.StatusApproved, false},
		{domain.StatusPending, domain.StatusRejected, false},
		{domain.StatusPending, domain.StatusCancelled, true},
		{domain.StatusPending, domain.StatusExpired, true},
		{domain.StatusUnderReview, domain.StatusApproved, true},
		{domain.StatusUnderReview, domain.StatusRejected, true},
		{domain.StatusUnderReview, domain.StatusExpired, true},
		{domain.StatusUnderReview, domain.StatusPending, false},
		{domain.StatusApproved, domain.StatusPending, false},
		{domain.StatusApproved, domain.StatusRejected, false},
		{domain.StatusRejected, domain.StatusApproved, false},
		{domain.StatusCancelled, domain.StatusApproved, false},
		{domain.StatusExpired, domain.StatusApproved, false},
	}

	for _, tc := range cases {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			assert.Equal(t, tc.allowed, tc.from.CanTransitionTo(tc.to))
		})
	}
}

func TestChangeStatus_InvalidTransitionReturnsError(t *testing.T) {
	pi := validIntent(t)
	require.NoError(t, pi.ChangeStatus(domain.StatusUnderReview))
	require.NoError(t, pi.ChangeStatus(domain.StatusApproved))

	err := pi.ChangeStatus(domain.StatusRejected)
	assert.ErrorIs(t, err, domain.ErrInvalidTransition)
	assert.Equal(t, domain.StatusApproved, pi.Status, "un intento fallido no debe mutar el estado")
}

// underReviewIntent simula el paso obligatorio por UNDER_REVIEW al
// enviarse a evaluación de riesgo (ver ADR-0001) antes de aplicar
// cualquier decisión de riesgo.
func underReviewIntent(t *testing.T) *domain.PaymentIntent {
	t.Helper()
	pi := validIntent(t)
	require.NoError(t, pi.ChangeStatus(domain.StatusUnderReview))
	return pi
}

func TestApplyRiskDecision_Approve(t *testing.T) {
	pi := underReviewIntent(t)
	err := pi.ApplyRiskDecision(domain.RiskApprove, 10, nil, "rules-v1")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusApproved, pi.Status)
	assert.Equal(t, domain.RiskApprove, *pi.RiskDecision)
	assert.Equal(t, 10, *pi.RiskScore)
	assert.Equal(t, "rules-v1", *pi.RiskModelVersion)
}

func TestApplyRiskDecision_Review_StaysUnderReview(t *testing.T) {
	pi := underReviewIntent(t)
	err := pi.ApplyRiskDecision(domain.RiskReview, 60, []string{"AMOUNT_ABOVE_THRESHOLD"}, "rules-v1")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusUnderReview, pi.Status)
	assert.Equal(t, []string{"AMOUNT_ABOVE_THRESHOLD"}, pi.RiskReasonCodes)
}

func TestApplyRiskDecision_Reject(t *testing.T) {
	pi := underReviewIntent(t)
	err := pi.ApplyRiskDecision(domain.RiskReject, 100, []string{"MERCHANT_BLOCKED"}, "rules-v1")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusRejected, pi.Status)
}

func TestApplyRiskDecision_OnAlreadyResolvedIntent_Fails(t *testing.T) {
	pi := underReviewIntent(t)
	require.NoError(t, pi.ApplyRiskDecision(domain.RiskApprove, 10, nil, "rules-v1"))

	err := pi.ApplyRiskDecision(domain.RiskReject, 100, nil, "rules-v1")
	assert.ErrorIs(t, err, domain.ErrInvalidTransition)
}
