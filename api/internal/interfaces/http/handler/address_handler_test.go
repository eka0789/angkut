package handler_test

import (
	"context"
	"testing"

	"github.com/angkut/angkut/apps/api/internal/domain/address"
	"github.com/angkut/angkut/apps/api/internal/domain/region"
	"github.com/angkut/angkut/apps/api/internal/domain/shared"
)

// Smoke test: interface terpenuhi tanpa DB.
func TestAddressHandlerCompiles(t *testing.T) {
	var _ addressRepo = (*mockAddressRepo)(nil)
	var _ regionRepo = (*mockRegionRepo)(nil)
}

type addressRepo interface {
	Create(ctx context.Context, a *address.Address) error
	GetByID(ctx context.Context, orgID, id string) (*address.Address, error)
	Update(ctx context.Context, a *address.Address) error
	Delete(ctx context.Context, orgID, id string) error
	List(ctx context.Context, orgID string, status address.Status) ([]address.Address, error)
	Search(ctx context.Context, orgID, query string, limit int) ([]address.Address, error)
	UnsetDefaultsExcept(ctx context.Context, orgID, exceptID string) error
}

type regionRepo interface {
	GetProvinceByID(ctx context.Context, id string) (*region.Province, error)
	GetCityByID(ctx context.Context, id string) (*region.City, error)
	GetDistrictByID(ctx context.Context, id string) (*region.District, error)
	GetVillageByID(ctx context.Context, id string) (*region.Village, error)
}

type mockAddressRepo struct{}

func (m *mockAddressRepo) Create(ctx context.Context, a *address.Address) error { return nil }
func (m *mockAddressRepo) GetByID(ctx context.Context, orgID, id string) (*address.Address, error) {
	return nil, shared.NotFound("ADDRESS_NOT_FOUND", "Alamat tidak ditemukan.")
}
func (m *mockAddressRepo) Update(ctx context.Context, a *address.Address) error { return nil }
func (m *mockAddressRepo) Delete(ctx context.Context, orgID, id string) error   { return nil }
func (m *mockAddressRepo) List(ctx context.Context, orgID string, status address.Status) ([]address.Address, error) {
	return nil, nil
}
func (m *mockAddressRepo) Search(ctx context.Context, orgID, query string, limit int) ([]address.Address, error) {
	return nil, nil
}
func (m *mockAddressRepo) UnsetDefaultsExcept(ctx context.Context, orgID, exceptID string) error {
	return nil
}

type mockRegionRepo struct{}

func (m *mockRegionRepo) GetProvinceByID(ctx context.Context, id string) (*region.Province, error) {
	return nil, shared.NotFound("PROVINCE_NOT_FOUND", "Provinsi tidak ditemukan.")
}
func (m *mockRegionRepo) GetCityByID(ctx context.Context, id string) (*region.City, error) {
	return nil, shared.NotFound("CITY_NOT_FOUND", "Kota tidak ditemukan.")
}
func (m *mockRegionRepo) GetDistrictByID(ctx context.Context, id string) (*region.District, error) {
	return nil, shared.NotFound("DISTRICT_NOT_FOUND", "Kecamatan tidak ditemukan.")
}
func (m *mockRegionRepo) GetVillageByID(ctx context.Context, id string) (*region.Village, error) {
	return nil, shared.NotFound("VILLAGE_NOT_FOUND", "Desa/kelurahan tidak ditemukan.")
}
