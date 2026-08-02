package application

import (
	"context"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

type MerchantService struct {
	merchants MerchantRepository
}

func NewMerchantService(merchants MerchantRepository) *MerchantService {
	return &MerchantService{merchants: merchants}
}

// Create valida y persiste un Merchant nuevo. Si document_number ya
// existe, el repositorio devuelve ErrConflict (restricción única).
func (s *MerchantService) Create(ctx context.Context, name, documentNumber, email string) (*domain.Merchant, error) {
	m, err := domain.NewMerchant(name, documentNumber, email)
	if err != nil {
		return nil, err
	}
	if err := s.merchants.Create(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *MerchantService) Get(ctx context.Context, id string) (*domain.Merchant, error) {
	return s.merchants.GetByID(ctx, id)
}
