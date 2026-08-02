package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lauvale029/MOVA_prueba_grupal/core-api/internal/domain"
)

func TestNewMerchant_Valid(t *testing.T) {
	m, err := domain.NewMerchant("Tienda Demo", "900123456", "demo@tienda.com")

	require.NoError(t, err)
	assert.NotEmpty(t, m.ID)
	assert.Equal(t, domain.MerchantStatusActive, m.Status)
	assert.False(t, m.CreatedAt.IsZero())
}

func TestNewMerchant_InvalidCases(t *testing.T) {
	cases := []struct {
		name, doc, email string
		expectedErr      error
	}{
		{"", "900123456", "demo@tienda.com", domain.ErrMissingMerchantName},
		{"Tienda Demo", "", "demo@tienda.com", domain.ErrMissingDocumentNumber},
		{"Tienda Demo", "900123456", "", domain.ErrMissingEmail},
	}

	for _, tc := range cases {
		_, err := domain.NewMerchant(tc.name, tc.doc, tc.email)
		assert.ErrorIs(t, err, tc.expectedErr)
	}
}
