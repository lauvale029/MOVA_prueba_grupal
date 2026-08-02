package application_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/application"
	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

type inMemoryMerchantRepository struct {
	mu       sync.Mutex
	byID     map[string]*domain.Merchant
	byDocNum map[string]string
}

func newInMemoryMerchantRepository() *inMemoryMerchantRepository {
	return &inMemoryMerchantRepository{
		byID:     make(map[string]*domain.Merchant),
		byDocNum: make(map[string]string),
	}
}

func (r *inMemoryMerchantRepository) Create(_ context.Context, m *domain.Merchant) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byDocNum[m.DocumentNumber]; exists {
		return application.ErrConflict
	}
	cp := *m
	r.byID[m.ID] = &cp
	r.byDocNum[m.DocumentNumber] = m.ID
	return nil
}

func (r *inMemoryMerchantRepository) GetByID(_ context.Context, id string) (*domain.Merchant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.byID[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	cp := *m
	return &cp, nil
}

func TestMerchantService_Create_Valid(t *testing.T) {
	svc := application.NewMerchantService(newInMemoryMerchantRepository())

	m, err := svc.Create(context.Background(), "Tienda Demo", "900123456", "demo@tienda.com")

	require.NoError(t, err)
	assert.Equal(t, domain.MerchantStatusActive, m.Status)
}

func TestMerchantService_Create_InvalidData(t *testing.T) {
	svc := application.NewMerchantService(newInMemoryMerchantRepository())

	_, err := svc.Create(context.Background(), "", "900123456", "demo@tienda.com")
	assert.ErrorIs(t, err, domain.ErrMissingMerchantName)
}

func TestMerchantService_Create_DuplicateDocumentNumber(t *testing.T) {
	svc := application.NewMerchantService(newInMemoryMerchantRepository())
	ctx := context.Background()

	_, err := svc.Create(ctx, "Tienda Demo", "900123456", "demo@tienda.com")
	require.NoError(t, err)

	_, err = svc.Create(ctx, "Otra Tienda", "900123456", "otra@tienda.com")
	assert.ErrorIs(t, err, application.ErrConflict)
}

func TestMerchantService_Get_NotFound(t *testing.T) {
	svc := application.NewMerchantService(newInMemoryMerchantRepository())
	_, err := svc.Get(context.Background(), "no-existe")
	assert.ErrorIs(t, err, application.ErrNotFound)
}

func TestMerchantService_Get_Success(t *testing.T) {
	svc := application.NewMerchantService(newInMemoryMerchantRepository())
	created, err := svc.Create(context.Background(), "Tienda Demo", "900123456", "demo@tienda.com")
	require.NoError(t, err)

	got, err := svc.Get(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
}
