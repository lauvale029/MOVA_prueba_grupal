//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/infrastructure/postgres"
)

func databaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL no está configurada")
	}
	return url
}

func newMerchant(t *testing.T, documentNumber string) *domain.Merchant {
	t.Helper()
	m, err := domain.NewMerchant("Tienda Test", documentNumber, "test@tienda.com")
	require.NoError(t, err)
	return m
}

func TestMerchantRepository_CreateAndGetByID(t *testing.T) {
	pool, err := postgres.NewPool(context.Background(), databaseURL(t))
	require.NoError(t, err)
	defer pool.Close()

	repo := postgres.NewMerchantRepository(pool)
	m := newMerchant(t, "900-create-"+uuid.New().String())

	require.NoError(t, repo.Create(context.Background(), m))

	got, err := repo.GetByID(context.Background(), m.ID)
	require.NoError(t, err)
	require.Equal(t, m.DocumentNumber, got.DocumentNumber)
	require.Equal(t, domain.MerchantStatusActive, got.Status)
}

func TestMerchantRepository_GetByID_NotFound(t *testing.T) {
	pool, err := postgres.NewPool(context.Background(), databaseURL(t))
	require.NoError(t, err)
	defer pool.Close()

	repo := postgres.NewMerchantRepository(pool)
	_, err = repo.GetByID(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, application.ErrNotFound)
}

func TestMerchantRepository_Create_DuplicateDocumentNumber(t *testing.T) {
	pool, err := postgres.NewPool(context.Background(), databaseURL(t))
	require.NoError(t, err)
	defer pool.Close()

	repo := postgres.NewMerchantRepository(pool)
	doc := "900-dup-" + uuid.New().String()

	first := newMerchant(t, doc)
	require.NoError(t, repo.Create(context.Background(), first))

	second := newMerchant(t, doc)
	err = repo.Create(context.Background(), second)
	require.ErrorIs(t, err, application.ErrConflict)
}
