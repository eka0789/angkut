package rates

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/angkut/angkut/apps/api/internal/domain/courier"
	"github.com/angkut/angkut/apps/api/internal/domain/pricing"
	"github.com/redis/go-redis/v9"
)

const cacheTTL = 5 * time.Minute

// Deps adalah dependensi service tarif.
type Deps struct {
	Repo     courier.Repository
	Registry *courier.Registry
	Engine   *pricing.Engine
	Log      *slog.Logger
	Redis    *redis.Client
}

// Service menghitung opsi tarif untuk satu rute.
type Service struct {
	repo      courier.Repository
	registry  *courier.Registry
	engine    *pricing.Engine
	log       *slog.Logger
	redis     *redis.Client
}

// New membuat service tarif.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		repo:      d.Repo,
		registry:  d.Registry,
		engine:    d.Engine,
		log:       log,
		redis:     d.Redis,
	}
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
// Gunakan caching Redis TTL 5 menit agar p95 < 300 ms.
func (s *Service) Quote(ctx context.Context, q courier.RateQuery) ([]Option, string, string, error) {
	// === CACHE CHECK ===
	cacheKey := cacheKeyForRateQuery(q)
	cached, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		// Restore cached options
		var cachedOptions []Option
		if jsonErr := json.Unmarshal([]byte(cached), &cachedOptions); jsonErr == nil {
			originZone, _ := cachedOptions[0].OriginZone, ""
			destZone, _ := cachedOptions[0].DestinationZone, ""
			return cachedOptions, originZone, destZone, nil
		}
		// Fall through to recompute if unmarshal fails
	}

	// === RECOMPUTE ===
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

	// === SCORE & LABEL ===
	label, score := scoreOptions(options)

	// === CACHE SET ===
	if s.redis != nil {
		cachedBytes, _ := json.Marshal(options)
		s.redis.Set(ctx, cacheKey, string(cachedBytes), cacheTTL)
		// Also store label+score for quick lookup
		scoreKey := cacheKey + ":label"
		s.redis.Set(ctx, scoreKey, fmt.Sprintf("%s|%v", label, score), cacheTTL)
	}

	return options, originZone.Code, destZone.Code, nil
}

// cacheKeyForRateQuery membuat kunci cache dari parameter quote.
func cacheKeyForRateQuery(q courier.RateQuery) string {
	// Hash all relevant fields for cache key
	// Note: org_id is not in RateQuery but added at handler level via tenant context
	var fields []string
	fields = append(fields, fmt.Sprintf("w%.3f", q.ActualWeightKG))
	if q.Dimensions != nil && q.Dimensions.Length > 0 {
		fields = append(fields, fmt.Sprintf("d%.1f-%.1f-%.1f", q.Dimensions.Length, q.Dimensions.Width, q.Dimensions.Height))
	}
	fields = append(fields, fmt.Sprintf("cod%.v", q.IsCOD), fmt.Sprintf("decl%.v", q.DeclaredValue))
	return "rate:" + fmt.Sprintf("%s:%s:%s", q.OriginCityID, q.DestinationCityID, fields)
}

// scoreOptions menentukan label terbaik berdasarkan skor multi-kriteria.
func scoreOptions(options []Option) (label string, score map[string]float64) {
	if len(options) == 0 {
		return "best_price", map[string]float64{"price": 0, "speed": 0, "reliability": 0, "value": 0}
	}

	// Normalisasi: indeks 0 adalah referensi (cheapest)
	ref := options[0]
	var minPrice int64 = ref.BasePrice + ref.CODFee + ref.InsuranceFee
	for _, opt := range options {
		total := opt.BasePrice + opt.CODFee + opt.InsuranceFee
		if total < minPrice {
			minPrice = total
		}
	}

	// Skor tiap opsi
	bestScores := make(map[string]float64) // best per criterion across all options
	for _, opt := range options {
		total := opt.BasePrice + opt.CODFee + opt.InsuranceFee
		// Price score: inverse of price (lower = better)
		var priceScore float64
		if total > 0 {
			priceScore = float64(minPrice) / float64(total)
		} else {
			priceScore = 1.0
		}
		// Speed score: placeholder (0.5 = netral; diisi dari courier reliability stat di TASK-025)
		speedScore := 0.5 // placeholder — diisi dari courier reliability stat
		// Reliability score: placeholder (0.7 = netral; dari courier_services reliability_score)
		reliabilityScore := 0.7 // placeholder
		// Value score: price × reliability
		valueScore := priceScore * reliabilityScore

		// Track best per criterion
		if bestScores["price"] < priceScore {
			bestScores["price"] = priceScore
		}
		if bestScores["speed"] < speedScore {
			bestScores["speed"] = speedScore
		}
		if bestScores["reliability"] < reliabilityScore {
			bestScores["reliability"] = reliabilityScore
		}
		if bestScores["value"] < valueScore {
			bestScores["value"] = valueScore
		}
	}

	// Determine best label
	// best_price = termurah
	// fastest = skor speed tertinggi (placeholder)
	// most_reliable = skor reliability tertinggi
	// best_value = price × reliability termahal

	// Sederhana: score setiap label berdasarkan total score
	label = "best_price"
	bestTotal := bestScores["price"]

	// Cek apakah fastest (placeholder: jika speed skor tinggi)
	if bestScores["speed"] > 0.6 && bestScores["speed"] > bestTotal {
		label = "fastest"
		bestTotal = bestScores["speed"]
	}

	// Cek most_reliable
	if bestScores["reliability"] > bestTotal {
		label = "most_reliable"
	}

	// Cek best_value (price_score × reliability)
	valueScore := bestScores["price"] * bestScores["reliability"]
	if valueScore > bestTotal {
		label = "best_value"
	}

	score = bestScores
	return label, score
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
		Service: sr.Service,
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

// ServicesFor mengembalikan layanan untuk kurir tertentu berdasarkan kode.
// Digunakan oleh endpoint GET /api/v1/couriers/{code}/services.
func (s *Service) ServicesFor(ctx context.Context, code string) (*courier.Courier, []courier.Service, error) {
	couriers, err := s.repo.ListActive(ctx)
	if err != nil {
		return nil, nil, err
	}

	for _, c := range couriers {
		if c.Code == code {
			services, err := s.repo.ListActiveServices(ctx, c.ID)
			if err != nil {
				return nil, nil, err
			}
			if len(services) == 0 {
				// Fallback: return empty services if none found
				services = []courier.Service{}
			}
			return &c, services, nil
		}
	}

	return nil, nil, fmt.Errorf("kurir dengan kode %s tidak ditemukan", code)
}


	// ListActive mengembalikan daftar kurir aktif untuk halaman registry.
	// Digunakan oleh endpoint GET /api/v1/couriers — List kurir yang aktif
	// dan tersedia di registry.
func (s *Service) ListActive(ctx context.Context) ([]courier.Courier, error) {
	return s.repo.ListActive(ctx)
}
