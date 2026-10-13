// Package mockcourier menyediakan MockCourierProvider — adapter kurir yang
// menghitung tarif dari tabel courier_rates, bukan dari API eksternal.
// Kontraknya identik dengan adapter sungguhan (ADR-002), sehingga Phase 4
// cukup menukar implementasi di registry tanpa menyentuh logika bisnis.
package mockcourier

import (
	"context"

	"github.com/angkut/angkut/apps/api/internal/domain/courier"
)

// Provider adalah adapter mock yang membaca tarif dari registry database.
type Provider struct {
	code string
	repo courier.Repository
}

// New membuat provider mock untuk kode kurir tertentu.
func New(code string, repo courier.Repository) *Provider {
	return &Provider{code: code, repo: repo}
}

// Code mengembalikan kode kurir yang dilayani.
func (p *Provider) Code() string { return p.code }

// FetchRates menghitung tarif semua layanan aktif kurir ini untuk rute pada
// berat tertentu. Layanan tanpa tarif yang cocok dilewati (bukan error) —
// kurir nyata juga boleh tidak melayani rute tertentu.
func (p *Provider) FetchRates(ctx context.Context, q courier.RateQuery) ([]courier.ServiceRate, error) {
	c, err := p.repo.GetByCode(ctx, p.code)
	if err != nil {
		return nil, err
	}
	originZone, err := p.repo.ZoneForCity(ctx, q.OriginCityID)
	if err != nil {
		return nil, err
	}
	destZone, err := p.repo.ZoneForCity(ctx, q.DestinationCityID)
	if err != nil {
		return nil, err
	}

	services, err := p.repo.ListActiveServices(ctx, c.ID)
	if err != nil {
		return nil, err
	}

	out := make([]courier.ServiceRate, 0)
	for _, s := range services {
		// Layanan non-COD dilewati bila pengiriman COD.
		if q.IsCOD && !s.SupportsCOD {
			continue
		}
		rates, err := p.repo.FindRates(ctx, s.ID, originZone.Code, destZone.Code)
		if err != nil {
			return nil, err
		}
		rate, ok := matchRate(rates, q.WeightKG)
		if !ok {
			continue
		}
		out = append(out, courier.ServiceRate{
			Courier:         *c,
			Service:         s,
			Price:           rate.Price,
			OriginZone:      originZone.Code,
			DestinationZone: destZone.Code,
		})
	}
	return out, nil
}

// matchRate memilih rate pertama yang mencakup berat (tier sudah urut naik).
func matchRate(rates []courier.Rate, weight float64) (courier.Rate, bool) {
	for _, r := range rates {
		if r.MatchesWeight(weight) {
			return r, true
		}
	}
	return courier.Rate{}, false
}
