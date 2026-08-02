//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/postgres"
)

// seedMerchant crea un comercio real: payment_intents tiene FK contra
// payments.merchants desde la migración 0006.
func seedMerchant(t *testing.T, merchants *postgres.MerchantRepository) *domain.Merchant {
	t.Helper()
	m := newMerchant(t, "900-pi-"+uuid.New().String())
	require.NoError(t, merchants.Create(context.Background(), m))
	return m
}

func newIntent(t *testing.T, merchantID, externalRef, idemKey string) *domain.PaymentIntent {
	t.Helper()
	pi, err := domain.NewPaymentIntent(merchantID, externalRef, 15000, domain.SupportedCurrency, domain.ChannelQR, idemKey, "")
	require.NoError(t, err)
	return pi
}

// createIntent inserta el intent junto con su entrada de historial en la
// MISMA transacción: el trigger trg_intent_requires_history (migración
// 0004) exige que exista al hacer COMMIT, igual que hace
// PaymentIntentService.Create en la capa de aplicación.
func createIntent(t *testing.T, uow application.UnitOfWork, payments application.PaymentIntentRepository, history application.PaymentIntentStatusHistoryRepository, pi *domain.PaymentIntent) error {
	t.Helper()
	return uow.Execute(context.Background(), func(ctx context.Context) error {
		if err := payments.Create(ctx, pi); err != nil {
			return err
		}
		h := domain.NewPaymentIntentStatusHistory(pi.ID, "", pi.Status, "creado", "test", pi.CorrelationID)
		return history.Create(ctx, h)
	})
}

// updateIntent aplica el UPDATE junto con su entrada de historial, por la
// misma razón que createIntent.
func updateIntent(t *testing.T, uow application.UnitOfWork, payments application.PaymentIntentRepository, history application.PaymentIntentStatusHistoryRepository, pi *domain.PaymentIntent, previousStatus domain.Status) error {
	t.Helper()
	return uow.Execute(context.Background(), func(ctx context.Context) error {
		if err := payments.Update(ctx, pi); err != nil {
			return err
		}
		h := domain.NewPaymentIntentStatusHistory(pi.ID, previousStatus, pi.Status, "actualizado", "test", pi.CorrelationID)
		return history.Create(ctx, h)
	})
}

func TestPaymentIntentRepository_CreateAndGetByID(t *testing.T) {
	pool, err := postgres.NewPool(context.Background(), databaseURL(t))
	require.NoError(t, err)
	defer pool.Close()

	merchants := postgres.NewMerchantRepository(pool)
	merchant := seedMerchant(t, merchants)
	repo := postgres.NewPaymentIntentRepository(pool)
	history := postgres.NewPaymentIntentStatusHistoryRepository(pool)
	uow := postgres.NewUnitOfWork(pool)

	pi := newIntent(t, merchant.ID, "order-"+uuid.New().String(), uuid.New().String())
	require.NoError(t, createIntent(t, uow, repo, history, pi))

	got, err := repo.GetByID(context.Background(), pi.ID)
	require.NoError(t, err)
	require.Equal(t, pi.ExternalReference, got.ExternalReference)
	require.Equal(t, domain.StatusPending, got.Status)
	require.Nil(t, got.RiskDecision)

	byKey, err := repo.GetByIdempotencyKey(context.Background(), pi.IdempotencyKey)
	require.NoError(t, err)
	require.Equal(t, pi.ID, byKey.ID)
}

func TestPaymentIntentRepository_GetByID_NotFound(t *testing.T) {
	pool, err := postgres.NewPool(context.Background(), databaseURL(t))
	require.NoError(t, err)
	defer pool.Close()

	repo := postgres.NewPaymentIntentRepository(pool)
	_, err = repo.GetByID(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, application.ErrNotFound)
}

func TestPaymentIntentRepository_Create_DuplicateIdempotencyKey(t *testing.T) {
	pool, err := postgres.NewPool(context.Background(), databaseURL(t))
	require.NoError(t, err)
	defer pool.Close()

	merchants := postgres.NewMerchantRepository(pool)
	merchant := seedMerchant(t, merchants)
	repo := postgres.NewPaymentIntentRepository(pool)
	history := postgres.NewPaymentIntentStatusHistoryRepository(pool)
	uow := postgres.NewUnitOfWork(pool)

	key := uuid.New().String()
	first := newIntent(t, merchant.ID, "order-"+uuid.New().String(), key)
	require.NoError(t, createIntent(t, uow, repo, history, first))

	second := newIntent(t, merchant.ID, "order-"+uuid.New().String(), key)
	err = createIntent(t, uow, repo, history, second)
	require.ErrorIs(t, err, application.ErrConflict)
}

// TestPaymentIntentRepository_Create_DuplicateExternalReference cierra el
// issue #13: la pareja (merchant_id, external_reference) debe ser única.
func TestPaymentIntentRepository_Create_DuplicateExternalReference(t *testing.T) {
	pool, err := postgres.NewPool(context.Background(), databaseURL(t))
	require.NoError(t, err)
	defer pool.Close()

	merchants := postgres.NewMerchantRepository(pool)
	merchant := seedMerchant(t, merchants)
	repo := postgres.NewPaymentIntentRepository(pool)
	history := postgres.NewPaymentIntentStatusHistoryRepository(pool)
	uow := postgres.NewUnitOfWork(pool)

	ref := "order-" + uuid.New().String()
	first := newIntent(t, merchant.ID, ref, uuid.New().String())
	require.NoError(t, createIntent(t, uow, repo, history, first))

	second := newIntent(t, merchant.ID, ref, uuid.New().String())
	err = createIntent(t, uow, repo, history, second)
	require.ErrorIs(t, err, application.ErrConflict)
}

func TestPaymentIntentRepository_Update(t *testing.T) {
	pool, err := postgres.NewPool(context.Background(), databaseURL(t))
	require.NoError(t, err)
	defer pool.Close()

	merchants := postgres.NewMerchantRepository(pool)
	merchant := seedMerchant(t, merchants)
	repo := postgres.NewPaymentIntentRepository(pool)
	history := postgres.NewPaymentIntentStatusHistoryRepository(pool)
	uow := postgres.NewUnitOfWork(pool)

	pi := newIntent(t, merchant.ID, "order-"+uuid.New().String(), uuid.New().String())
	require.NoError(t, createIntent(t, uow, repo, history, pi))

	// Igual que el flujo real (ver PaymentIntentService.markUnderReview):
	// PENDING->APPROVED directo no existe, hay que pasar por UNDER_REVIEW
	// y persistir cada transición por separado.
	require.NoError(t, pi.ChangeStatus(domain.StatusUnderReview))
	require.NoError(t, updateIntent(t, uow, repo, history, pi, domain.StatusPending))

	previousStatus := pi.Status
	require.NoError(t, pi.ApplyRiskDecision(domain.RiskApprove, 12, []string{"LOW_RISK"}, "rules-v1"))
	require.NoError(t, updateIntent(t, uow, repo, history, pi, previousStatus))

	got, err := repo.GetByID(context.Background(), pi.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusApproved, got.Status)
	require.NotNil(t, got.RiskDecision)
	require.Equal(t, domain.RiskApprove, *got.RiskDecision)
	require.Equal(t, 12, *got.RiskScore)
	require.Equal(t, []string{"LOW_RISK"}, got.RiskReasonCodes)
}

func TestPaymentIntentRepository_ListAndCount_FiltersByMerchant(t *testing.T) {
	pool, err := postgres.NewPool(context.Background(), databaseURL(t))
	require.NoError(t, err)
	defer pool.Close()

	merchants := postgres.NewMerchantRepository(pool)
	merchantA := seedMerchant(t, merchants)
	merchantB := seedMerchant(t, merchants)
	repo := postgres.NewPaymentIntentRepository(pool)
	history := postgres.NewPaymentIntentStatusHistoryRepository(pool)
	uow := postgres.NewUnitOfWork(pool)

	require.NoError(t, createIntent(t, uow, repo, history, newIntent(t, merchantA.ID, "order-"+uuid.New().String(), uuid.New().String())))
	require.NoError(t, createIntent(t, uow, repo, history, newIntent(t, merchantA.ID, "order-"+uuid.New().String(), uuid.New().String())))
	require.NoError(t, createIntent(t, uow, repo, history, newIntent(t, merchantB.ID, "order-"+uuid.New().String(), uuid.New().String())))

	filter := application.PaymentIntentFilter{MerchantID: merchantA.ID, Page: 1, Limit: 10}
	items, err := repo.List(context.Background(), filter)
	require.NoError(t, err)
	require.Len(t, items, 2)

	total, err := repo.Count(context.Background(), filter)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
}
