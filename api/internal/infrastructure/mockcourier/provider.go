// Package mockcourier menyediakan MockCourierProvider — adapter kurir yang
// menghitung tarif dari tabel courier_rates, bukan dari API eksternal.
// Kontraknya identik dengan adapter sungguhan (ADR-002), sehingga Phase 4
// cukup menukar implementasi di registry tanpa menyentuh logika bisnis.
package mockcourier

import (
	"context"
	"math"

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

// FetchRates menghitung tarif semua layanan aktif kurir ini untuk rute.
// Berat chargeable dihitung PER LAYANAN (divisor volumetrik bisa berbeda),
// dibulatkan ke atas ke step dari query, lalu dicocokkan ke tier tarif.
// Layanan tanpa tarif yang cocok dilewati (bukan error) — kurir nyata juga
// boleh tidak melayani rute tertentu.
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
		wr := p.weights(q, s.VolumetricDivisor)
		rates, err := p.repo.FindRates(ctx, s.ID, originZone.Code, destZone.Code)
		if err != nil {
			return nil, err
		}
		rate, ok := matchRate(rates, wr.Chargeable)
		if !ok {
			continue
		}
		out = append(out, courier.ServiceRate{
			Courier:          *c,
			Service:          s,
			Price:            rate.Price,
			OriginZone:       originZone.Code,
			DestinationZone:  destZone.Code,
			ChargeableWeight: wr.Chargeable,
			VolumetricWeight: wr.Volumetric,
		})
	}
	return out, nil
}

// weights menghitung berat aktual/volumetrik/chargeable untuk satu layanan.
func (p *Provider) weights(q courier.RateQuery, divisor int) courier.WeightResult {
	var vol float64
	if q.Dimensions != nil && q.Dimensions.Length > 0 && q.Dimensions.Width > 0 && q.Dimensions.Height > 0 {
		vol = float64(q.Dimensions.VolumetricWeight(divisor))
	}
	chargeable := q.ActualWeightKG
	if vol > chargeable {
		chargeable = vol
	}
	if q.WeightRoundUpStepKG > 0 {
		chargeable = math.Ceil(chargeable/q.WeightRoundUpStepKG) * q.WeightRoundUpStepKG
	}
	return courier.WeightResult{
		Actual:     q.ActualWeightKG,
		Volumetric: vol,
		Chargeable: chargeable,
	}
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
