package handler_test

import (
	"context"
	"testing"

	"github.com/angkut/angkut/apps/api/internal/domain/customer"
	"github.com/angkut/angkut/apps/api/internal/domain/shared"
)

// Smoke test: interface terpenuhi tanpa DB.
func TestCustomerHandlerCompiles(t *testing.T) {
	var _ customerRepo = (*mockCustomerRepo)(nil)
}

type customerRepo interface {
	Create(ctx context.Context, c *customer.Customer) error
	GetByID(ctx context.Context, orgID, id string) (*customer.Customer, error)
	GetByPhone(ctx context.Context, orgID, phone string) (*customer.Customer, error)
	Update(ctx context.Context, c *customer.Customer) error
	Delete(ctx context.Context, orgID, id string) error
	List(ctx context.Context, orgID string, segment customer.Segment, query string, limit, offset int) ([]customer.Customer, int, error)
}

type mockCustomerRepo struct{}

func (m *mockCustomerRepo) Create(ctx context.Context, c *customer.Customer) error { return nil }
func (m *mockCustomerRepo) GetByID(ctx context.Context, orgID, id string) (*customer.Customer, error) {
	return nil, customer.ErrNotFound
}
func (m *mockCustomerRepo) GetByPhone(ctx context.Context, orgID, phone string) (*customer.Customer, error) {
	return nil, shared.NotFound("CUSTOMER_NOT_FOUND", "Pelanggan tidak ditemukan.")
}
func (m *mockCustomerRepo) Update(ctx context.Context, c *customer.Customer) error { return nil }
func (m *mockCustomerRepo) Delete(ctx context.Context, orgID, id string) error     { return nil }
func (m *mockCustomerRepo) List(ctx context.Context, orgID string, segment customer.Segment, query string, limit, offset int) ([]customer.Customer, int, error) {
	return nil, 0, nil
}
