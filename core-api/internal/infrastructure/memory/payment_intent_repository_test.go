package memory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/memory"
)

func newIntent(t *testing.T, idemKey string) *domain.PaymentIntent {
	t.Helper()
	pi, err := domain.NewPaymentIntent("merchant-1", "order-"+idemKey, 1000, "COP", domain.ChannelQR, idemKey, "")
	require.NoError(t, err)
	return pi
}

func TestPaymentIntentRepository_CreateAndGetByID(t *testing.T) {
	repo := memory.NewPaymentIntentRepository()
	pi := newIntent(t, "idem-1")

	require.NoError(t, repo.Create(context.Background(), pi))

	got, err := repo.GetByID(context.Background(), pi.ID)
	require.NoError(t, err)
	assert.Equal(t, pi.ID, got.ID)
}

func TestPaymentIntentRepository_GetByID_NotFound(t *testing.T) {
	repo := memory.NewPaymentIntentRepository()
	_, err := repo.GetByID(context.Background(), "no-existe")
	assert.ErrorIs(t, err, application.ErrNotFound)
}

func TestPaymentIntentRepository_Create_DuplicateIdempotencyKey(t *testing.T) {
	repo := memory.NewPaymentIntentRepository()
	first := newIntent(t, "misma-key")
	second := newIntent(t, "misma-key")

	require.NoError(t, repo.Create(context.Background(), first))
	err := repo.Create(context.Background(), second)
	assert.ErrorIs(t, err, application.ErrConflict)
}

func TestPaymentIntentRepository_GetByIdempotencyKey(t *testing.T) {
	repo := memory.NewPaymentIntentRepository()
	pi := newIntent(t, "idem-1")
	require.NoError(t, repo.Create(context.Background(), pi))

	got, err := repo.GetByIdempotencyKey(context.Background(), "idem-1")
	require.NoError(t, err)
	assert.Equal(t, pi.ID, got.ID)
}

func TestPaymentIntentRepository_Update(t *testing.T) {
	repo := memory.NewPaymentIntentRepository()
	pi := newIntent(t, "idem-1")
	require.NoError(t, repo.Create(context.Background(), pi))

	require.NoError(t, pi.ChangeStatus(domain.StatusCancelled))
	require.NoError(t, repo.Update(context.Background(), pi))

	got, _ := repo.GetByID(context.Background(), pi.ID)
	assert.Equal(t, domain.StatusCancelled, got.Status)
}

func TestPaymentIntentRepository_List_FiltersByMerchantAndStatus(t *testing.T) {
	repo := memory.NewPaymentIntentRepository()
	ctx := context.Background()

	a, _ := domain.NewPaymentIntent("merchant-A", "order-a", 1000, "COP", domain.ChannelQR, "key-a", "")
	b, _ := domain.NewPaymentIntent("merchant-B", "order-b", 1000, "COP", domain.ChannelQR, "key-b", "")
	require.NoError(t, repo.Create(ctx, a))
	require.NoError(t, repo.Create(ctx, b))

	items, err := repo.List(ctx, application.PaymentIntentFilter{MerchantID: "merchant-A", Page: 1, Limit: 10})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "merchant-A", items[0].MerchantID)
}

func TestPaymentIntentRepository_Count(t *testing.T) {
	repo := memory.NewPaymentIntentRepository()
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, newIntent(t, "idem-1")))
	require.NoError(t, repo.Create(ctx, newIntent(t, "idem-2")))

	total, err := repo.Count(ctx, application.PaymentIntentFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
}
