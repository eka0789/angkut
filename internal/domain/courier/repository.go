package courier

import (
	"context"
)

// Repository membaca registry kurir, layanan, tarif, dan zona.
// Kurir adalah data platform (bukan tenant) — tidak ada filter organization_id.
type Repository interface {
	ListActive(ctx context.Context) ([]Courier, error)
	GetByCode(ctx context.Context, code string) (*Courier, error)
	ListActiveServices(ctx context.Context, courierID string) ([]Service, error)
	ListActiveServicesAll(ctx context.Context) ([]Service, error)
	// FindRates mengembalikan rate efektif untuk satu layanan pada pasangan
	// zona. Pemanggil menyaring tier berat via MatchesWeight.
	FindRates(ctx context.Context, serviceID, originZone, destinationZone string) ([]Rate, error)
	// ZoneForRegion mencari zona yang memuat kode wilayah (id provinsi/kota).
	// regionType menentukan kolom pencocokan ("province", "city", ...).
	ZoneForRegion(ctx context.Context, regionType, regionID string) (*Zone, error)
	// ZoneForCity mencari zona tarif untuk kota tujuan/asal. Zona berbasis
	// provinsi, sehingga implementasi menjembatani city → province → zone.
	ZoneForCity(ctx context.Context, cityID string) (*Zone, error)
}

// Provider adalah adapter ke API kurir eksternal (ADR-002). Implementasi
// sungguhan (JNE, J&T, ...) menulis di Phase 4; MockCourierProvider menghitung
// dari tabel courier_rates sehingga seluruh alur bisa diuji tanpa jaringan.
type Provider interface {
	// Code adalah kode kurir yang dilayani adapter ini.
	Code() string
	// FetchRates mengembalikan daftar tarif layanan kurir untuk satu rute.
	FetchRates(ctx context.Context, q RateQuery) ([]ServiceRate, error)
}

// Registry menyatukan provider per kode kurir. Logika bisnis meminta provider
// lewat registry, bukan memilih implementasi sendiri.
type Registry struct {
	byCode map[string]Provider
}

// NewRegistry membangun registry dari daftar provider. Provider dengan kode
// sama menimpa yang lama (last-write-wins) — dipakai saat mengganti mock
// dengan adapter sungguhan pada kode yang sama.
func NewRegistry(providers ...Provider) *Registry {
	byCode := make(map[string]Provider, len(providers))
	for _, p := range providers {
		if p != nil && p.Code() != "" {
			byCode[p.Code()] = p
		}
	}
	return &Registry{byCode: byCode}
}

// ProviderFor mengembalikan provider untuk kode kurir; nil bila tidak terdaftar.
func (r *Registry) ProviderFor(code string) Provider {
	if r == nil {
		return nil
	}
	return r.byCode[code]
}
