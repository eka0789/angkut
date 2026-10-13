package courier_test

import (
	"context"
	"errors"
	"testing"

	"github.com/angkut/angkut/apps/api/internal/domain/courier"
	"github.com/angkut/angkut/apps/api/internal/domain/shared"
)

// Aturan tier: min_weight <= w < max_weight; max NULL = tanpa batas.
func TestRateMatchesWeight(t *testing.T) {
	rates := []courier.Rate{
		{MinWeight: 0, MaxWeight: ptr(1)},
		{MinWeight: 1, MaxWeight: ptr(5)},
		{MinWeight: 5, MaxWeight: nil},
	}

	cases := []struct {
		w    float64
		want int // indeks rate yang cocok; -1 = tidak ada
	}{
		{0.5, 0},
		{1.0, 1}, // min inklusif
		{4.99, 1},
		{5.0, 2},  // max eksklusif → tier berikutnya
		{50.0, 2}, // max NULL = tanpa batas
	}
	for _, tc := range cases {
		found := -1
		for i, r := range rates {
			if r.MatchesWeight(tc.w) {
				found = i
				break
			}
		}
		if found != tc.want {
			t.Fatalf("weight %v: tier ke-%d, want %d", tc.w, found, tc.want)
		}
	}
}

func ptr(v float64) *float64 { return &v }

func TestRateQueryValidate(t *testing.T) {
	ok := courier.RateQuery{OriginCityID: "3171", DestinationCityID: "3273", WeightKG: 1.5}
	if err := ok.Validate(); err != nil {
		t.Fatalf("query valid harus lolos: %v", err)
	}
	bad := courier.RateQuery{OriginCityID: "", DestinationCityID: "3273", WeightKG: 0}
	if err := bad.Validate(); err == nil {
		t.Fatal("query tanpa asal/berat harus gagal validasi")
	}
}

// Registry: provider per kode, last-write-wins, nil-safe.
func TestRegistry(t *testing.T) {
	p1 := &stubProvider{code: "jne"}
	reg := courier.NewRegistry(p1, nil)
	if reg.ProviderFor("jne") == nil {
		t.Fatal("provider jne harus terdaftar")
	}
	if reg.ProviderFor("tidak-ada") != nil {
		t.Fatal("kode tak dikenal harus nil")
	}

	// Registry nil harus aman.
	var empty *courier.Registry
	if empty.ProviderFor("jne") != nil {
		t.Fatal("registry nil harus mengembalikan nil")
	}
}

// Alur mock provider: resolve zona → layanan aktif → rate tier yang cocok.
func TestMockFlow(t *testing.T) {
	repo := &stubRepo{}
	p := mockOf("jne", repo)

	out, err := p.FetchRates(context.Background(), courier.RateQuery{
		OriginCityID:      "3171",
		DestinationCityID: "3273",
		WeightKG:          2,
	})
	if err != nil {
		t.Fatalf("FetchRates: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("semua layanan aktif (2) untuk non-COD, dapat %d", len(out))
	}
	if out[0].Price != 12000 {
		t.Fatalf("harga tier 1-5kg JABODETABEK→JAWA = 12000, dapat %d", out[0].Price)
	}
	if out[0].OriginZone != "ZONA_JABODETABEK" || out[0].DestinationZone != "ZONA_JAWA" {
		t.Fatalf("zona salah: %s → %s", out[0].OriginZone, out[0].DestinationZone)
	}
}

func mockOf(code string, repo courier.Repository) courier.Provider {
	return &mockAdapter{code: code, repo: repo}
}

// mockAdapter meniru struktur mockcourier.Provider tanpa import melingkar.
type mockAdapter struct {
	code string
	repo courier.Repository
}

func (m *mockAdapter) Code() string { return m.code }
func (m *mockAdapter) FetchRates(ctx context.Context, q courier.RateQuery) ([]courier.ServiceRate, error) {
	c, err := m.repo.GetByCode(ctx, m.code)
	if err != nil {
		return nil, err
	}
	oz, err := m.repo.ZoneForCity(ctx, q.OriginCityID)
	if err != nil {
		return nil, err
	}
	dz, err := m.repo.ZoneForCity(ctx, q.DestinationCityID)
	if err != nil {
		return nil, err
	}
	services, err := m.repo.ListActiveServices(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	var out []courier.ServiceRate
	for _, s := range services {
		if q.IsCOD && !s.SupportsCOD {
			continue
		}
		rates, err := m.repo.FindRates(ctx, s.ID, oz.Code, dz.Code)
		if err != nil {
			return nil, err
		}
		for _, r := range rates {
			if r.MatchesWeight(q.WeightKG) {
				out = append(out, courier.ServiceRate{
					Courier: *c, Service: s, Price: r.Price,
					OriginZone: oz.Code, DestinationZone: dz.Code,
				})
				break
			}
		}
	}
	return out, nil
}

type stubProvider struct{ code string }

func (s *stubProvider) Code() string { return s.code }
func (s *stubProvider) FetchRates(ctx context.Context, q courier.RateQuery) ([]courier.ServiceRate, error) {
	return nil, nil
}

// stubRepo dataset in-memory kecil untuk alur mock.
type stubRepo struct{}

func (s *stubRepo) ListActive(ctx context.Context) ([]courier.Courier, error) {
	return []courier.Courier{{ID: "c1", Code: "jne", Name: "JNE", IsActive: true}}, nil
}
func (s *stubRepo) GetByCode(ctx context.Context, code string) (*courier.Courier, error) {
	if code != "jne" {
		return nil, courier.ErrNotFound
	}
	return &courier.Courier{ID: "c1", Code: "jne", Name: "JNE", IsActive: true}, nil
}
func (s *stubRepo) ListActiveServices(ctx context.Context, courierID string) ([]courier.Service, error) {
	return []courier.Service{
		{ID: "s1", CourierID: "c1", Code: "REG", Name: "Reguler", SupportsCOD: true, EtaMinDays: 2, EtaMaxDays: 3, VolumetricDivisor: 6000},
		{ID: "s2", CourierID: "c1", Code: "COD", Name: "COD Only", SupportsCOD: true},
	}, nil
}
func (s *stubRepo) ListActiveServicesAll(ctx context.Context) ([]courier.Service, error) {
	return nil, nil
}
func (s *stubRepo) FindRates(ctx context.Context, serviceID, originZone, destinationZone string) ([]courier.Rate, error) {
	return []courier.Rate{
		{ID: "r1", CourierServiceID: serviceID, OriginZone: originZone, DestinationZone: destinationZone, MinWeight: 0, MaxWeight: ptr(1), Price: 9000},
		{ID: "r2", CourierServiceID: serviceID, OriginZone: originZone, DestinationZone: destinationZone, MinWeight: 1, MaxWeight: ptr(5), Price: 12000},
		{ID: "r3", CourierServiceID: serviceID, OriginZone: originZone, DestinationZone: destinationZone, MinWeight: 5, MaxWeight: nil, Price: 18000},
	}, nil
}
func (s *stubRepo) ZoneForRegion(ctx context.Context, regionType, regionID string) (*courier.Zone, error) {
	return nil, shared.NotFound("ZONE_NOT_FOUND", "Zona tidak ditemukan.")
}
func (s *stubRepo) ZoneForCity(ctx context.Context, cityID string) (*courier.Zone, error) {
	if cityID == "" {
		return nil, errors.New("empty")
	}
	zone := "ZONA_JABODETABEK"
	if cityID == "3273" {
		zone = "ZONA_JAWA"
	}
	return &courier.Zone{Code: zone, Name: zone, Type: "province"}, nil
}
