// Package rates menyediakan pencarian tarif lintas kurir. Logika bisnis hanya
// berbicara dengan interface Provider lewat Registry (ADR-002) — menambah
// kurir sungguhan berarti menambah adapter, bukan mengubah service ini.
package rates

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/angkut/angkut/apps/api/internal/domain/courier"
)

// Deps adalah dependensi service tarif.
type Deps struct {
	Repo     courier.Repository
	Registry *courier.Registry
	Log      *slog.Logger
}

// Service menghitung opsi tarif untuk satu rute.
type Service struct {
	repo     courier.Repository
	registry *courier.Registry
	log      *slog.Logger
}

// New membuat service tarif.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: d.Repo, registry: d.Registry, log: log}
}

// Quote mengembalikan daftar opsi tarif dari semua kurir yang punya adapter
// terdaftar dan melayani rute tersebut. Kurir tanpa adapter dilewati dengan
// catatan log — bukan error, agar registry boleh belum lengkap.
func (s *Service) Quote(ctx context.Context, q courier.RateQuery) ([]courier.ServiceRate, string, string, error) {
	if err := q.Validate(); err != nil {
		return nil, "", "", err
	}

	originZone, err := s.repo.ZoneForCity(ctx, q.OriginCityID)
	if err != nil {
		return nil, "", "", err
	}
	destZone, err := s.repo.ZoneForCity(ctx, q.DestinationCityID)
	if err != nil {
		return nil, "", "", err
	}

	couriers, err := s.repo.ListActive(ctx)
	if err != nil {
		return nil, "", "", err
	}

	options := make([]courier.ServiceRate, 0)
	for _, c := range couriers {
		provider := s.registry.ProviderFor(c.Code)
		if provider == nil {
			s.log.Debug("kurir tanpa adapter dilewati", "courier", c.Code)
			continue
		}
		rates, err := provider.FetchRates(ctx, q)
		if err != nil {
			// Satu kurir gagal tidak boleh menggagalkan quote keseluruhan.
			s.log.Warn("provider kurir gagal", "courier", c.Code, "error", err)
			continue
		}
		options = append(options, rates...)
	}
	return options, originZone.Code, destZone.Code, nil
}

// ListActive mengembalikan daftar kurir aktif untuk halaman registry.
func (s *Service) ListActive(ctx context.Context) ([]courier.Courier, error) {
	return s.repo.ListActive(ctx)
}

// ServicesFor mengembalikan layanan aktif satu kurir.
func (s *Service) ServicesFor(ctx context.Context, code string) (*courier.Courier, []courier.Service, error) {
	c, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return nil, nil, err
	}
	services, err := s.repo.ListActiveServices(ctx, c.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("layanan kurir: %w", err)
	}
	return c, services, nil
}
