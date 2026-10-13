// Package rates menyediakan pencarian tarif lintas kurir dan pemrician final
// lewat pricing engine. Logika bisnis hanya berbicara dengan interface
// Provider lewat Registry (ADR-002) — menambah kurir sungguhan berarti
// menambah adapter, bukan mengubah service ini.
package rates

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/angkut/angkut/apps/api/internal/domain/courier"
	"github.com/angkut/angkut/apps/api/internal/domain/pricing"
)

// Deps adalah dependensi service tarif.
type Deps struct {
	Repo     courier.Repository
	Registry *courier.Registry
	Engine   *pricing.Engine
	Log      *slog.Logger
}

// Service menghitung opsi tarif untuk satu rute.
type Service struct {
	repo     courier.Repository
	registry *courier.Registry
	engine   *pricing.Engine
	log      *slog.Logger
}

// New membuat service tarif.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: d.Repo, registry: d.Registry, engine: d.Engine, log: log}
}

// Option adalah satu opsi tarif siap tampil: tarif kurir + hasil pricing.
type Option struct {
	Courier          courier.Courier `json:"courier"`
	Service          courier.Service `json:"service"`
	OriginZone       string          `json:"origin_zone"`
	DestinationZone  string          `json:"destination_zone"`
	ChargeableWeight float64         `json:"chargeable_weight"`
	VolumetricWeight float64         `json:"volumetric_weight,omitempty"`
	BasePrice        int64           `json:"base_price"`
	Markup           int64           `json:"markup"`
	CODFee           int64           `json:"cod_fee"`
	InsuranceFee     int64           `json:"insurance_fee"`
	TotalPrice       int64           `json:"total_price"`
}

// Quote mengembalikan daftar opsi tarif dari semua kurir yang punya adapter
// terdaftar dan melayani rute tersebut. Kurir tanpa adapter dilewati dengan
// catatan log — bukan error, agar registry boleh belum lengkap.
func (s *Service) Quote(ctx context.Context, q courier.RateQuery) ([]Option, string, string, error) {
	if err := q.Validate(); err != nil {
		return nil, "", "", err
	}
	if s.engine == nil {
		return nil, "", "", fmt.Errorf("pricing engine belum dikonfigurasi")
	}
	// Step pembulatan berat diteruskan ke provider agar tier dicocokkan pada
	// berat yang sama dengan yang akan ditagih.
	q.WeightRoundUpStepKG = s.engine.Config().WeightRoundUpStepKG

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

	options := make([]Option, 0)
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
		for _, sr := range rates {
			options = append(options, s.priceOption(sr, q))
		}
	}
	return options, originZone.Code, destZone.Code, nil
}

// priceOption menerapkan pricing engine pada satu hasil provider.
func (s *Service) priceOption(sr courier.ServiceRate, q courier.RateQuery) Option {
	quote := s.engine.Price(sr.Price, pricing.QuoteOptions{
		IsCOD:                    q.IsCOD,
		CODAmount:                q.CODAmount,
		Insured:                  q.Insured,
		DeclaredValue:            q.DeclaredValue,
		ServiceSupportsCOD:       sr.Service.SupportsCOD,
		ServiceSupportsInsurance: sr.Service.SupportsInsurance,
	})
	return Option{
		Courier:          sr.Courier,
		Service:          sr.Service,
		OriginZone:       sr.OriginZone,
		DestinationZone:  sr.DestinationZone,
		ChargeableWeight: sr.ChargeableWeight,
		VolumetricWeight: sr.VolumetricWeight,
		BasePrice:        quote.BasePrice,
		Markup:           quote.Markup,
		CODFee:           quote.CODFee,
		InsuranceFee:     quote.InsuranceFee,
		TotalPrice:       quote.Total,
	}
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
