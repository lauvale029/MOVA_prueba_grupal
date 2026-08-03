//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/postgres"
)

func TestPaymentIntentStatusHistoryRepository_CreateAndList(t *testing.T) {
	pool, err := postgres.NewPool(context.Background(), databaseURL(t))
	require.NoError(t, err)
	defer pool.Close()

	merchants := postgres.NewMerchantRepository(pool)
	merchant := seedMerchant(t, merchants)
	payments := postgres.NewPaymentIntentRepository(pool)
	history := postgres.NewPaymentIntentStatusHistoryRepository(pool)
	uow := postgres.NewUnitOfWork(pool)

	pi := newIntent(t, merchant.ID, "order-"+uuid.New().String(), uuid.New().String())
	require.NoError(t, createIntent(t, uow, payments, history, pi))

	previousStatus := pi.Status
	require.NoError(t, pi.ChangeStatus(domain.StatusUnderReview))
	require.NoError(t, updateIntent(t, uow, payments, history, pi, previousStatus))

	entries, err := history.ListByPaymentIntentID(context.Background(), pi.ID)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	require.Equal(t, domain.Status(""), entries[0].PreviousStatus)
	require.Equal(t, domain.StatusPending, entries[0].NewStatus)

	require.Equal(t, domain.StatusPending, entries[1].PreviousStatus)
	require.Equal(t, domain.StatusUnderReview, entries[1].NewStatus)
}
