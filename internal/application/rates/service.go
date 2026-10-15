package rates

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/courier"
	"github.com/angkut/angkut/apps/internal/domain/pricing"
	"github.com/angkut/angkut/apps/internal/domain/scoring"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/redis/go-redis/v9"
)

const cacheTTL = 5 * time.Minute

// quoteIDPrefix menandai versi token quote_id.
const quoteIDPrefix = "q1."

// Deps adalah dependensi service tarif.
type Deps struct {
	Repo     courier.Repository
	Registry *courier.Registry
	Engine   *pricing.Engine
	Scorer   *scoring.Engine
	Log      *slog.Logger
	// Redis opsional: bila nil, hasil dihitung ulang tanpa cache.
	Redis *redis.Client
}

// Service menghitung opsi tarif untuk satu rute, memeringkatnya, dan
// menyusun rekomendasi.
type Service struct {
	repo     courier.Repository
	registry *courier.Registry
	engine   *pricing.Engine
	scorer   *scoring.Engine
	log      *slog.Logger
	redis    *redis.Client
}

// New membuat service tarif.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		repo:     d.Repo,
		registry: d.Registry,
		engine:   d.Engine,
		scorer:   d.Scorer,
		log:      log,
		redis:    d.Redis,
	}
}

// Party adalah identitas ringkas kurir/layanan sesuai kontrak (code + name).
type Party struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Option adalah satu opsi tarif siap tampil: tarif kurir + hasil pricing +
// skor/label/rekomendasi. Bentuknya mengikuti planning/api-contract.md
// §Couriers & Rates.
type Option struct {
	Courier Party `json:"courier"`
	Service Party `json:"service"`

	OriginZone       string  `json:"origin_zone"`
	DestinationZone  string  `json:"destination_zone"`
	ChargeableWeight float64 `json:"chargeable_weight"`
	VolumetricWeight float64 `json:"volumetric_weight,omitempty"`

	// Price adalah harga akhir yang dibayar pelanggan (termasuk markup, COD,
	// asuransi); rinciannya tersedia di field breakdown.
	Price        int64 `json:"price"`
	BasePrice    int64 `json:"base_price"`
	Markup       int64 `json:"markup"`
	CODFee       int64 `json:"cod_fee"`
	InsuranceFee int64 `json:"insurance_fee"`

	EtaMinDays       int     `json:"eta_min_days"`
	EtaMaxDays       int     `json:"eta_max_days"`
	ReliabilityScore float64 `json:"reliability_score"`
	SupportsCOD      bool    `json:"supports_cod"`
	SupportsPickup   bool    `json:"supports_pickup"`

	EstimatedSuccessRate float64                 `json:"estimated_success_rate"`
	Score                float64                 `json:"score"`
	Labels               []string                `json:"labels"`
	IsRecommended        bool                    `json:"is_recommended"`
	Recommendation       *scoring.Recommendation `json:"recommendation,omitempty"`
}

// QuoteResult adalah satu penilaian tarif lengkap: identitas quote, opsi
// terurut, dan ringkasan label.
type QuoteResult struct {
	QuoteID          string          `json:"quote_id"`
	OriginZone       string          `json:"origin_zone"`
	DestinationZone  string          `json:"destination_zone"`
	ChargeableWeight float64         `json:"chargeable_weight"`
	Options          []Option        `json:"options"`
	SummaryLabels    scoring.Summary `json:"summary_labels"`
}

// Quote menghitung, memeringkat, dan memberi label pada seluruh opsi tarif
// untuk satu rute. Kurir tanpa adapter atau tanpa tarif rute dilewati dengan
// catatan log — bukan error, agar registry boleh belum lengkap.
//
// Hasil di-cache di Redis selama cacheTTL agar p95 tetap rendah; cache adalah
// optimisasi murni — bila Redis tidak tersedia, perhitungan penuh dijalankan.
func (s *Service) Quote(ctx context.Context, q courier.RateQuery) (*QuoteResult, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	if s.engine == nil || s.scorer == nil {
		return nil, fmt.Errorf("pricing/scoring engine belum dikonfigurasi")
	}

	cacheKey := cacheKeyForRateQuery(q)
	if res := s.readCache(ctx, cacheKey); res != nil {
		return res, nil
	}

	// Step pembulatan berat diteruskan ke provider agar tier dicocokkan pada
	// berat yang sama dengan yang akan ditagih.
	q.WeightRoundUpStepKG = s.engine.Config().WeightRoundUpStepKG

	originZone, err := s.repo.ZoneForCity(ctx, q.OriginCityID)
	if err != nil {
		return nil, err
	}
	destZone, err := s.repo.ZoneForCity(ctx, q.DestinationCityID)
	if err != nil {
		return nil, err
	}

	couriers, err := s.repo.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	options := make([]Option, 0)
	for _, c := range couriers {
		provider := s.registry.ProviderFor(c.Code)
		if provider == nil {
			s.log.Debug("kurir tanpa adapter dilewati", "courier", c.Code)
			continue
		}
		found, err := provider.FetchRates(ctx, q)
		if err != nil {
			// Satu kurir gagal tidak boleh menggagalkan quote keseluruhan.
			s.log.Warn("provider kurir gagal", "courier", c.Code, "error", err)
			continue
		}
		for _, sr := range found {
			options = append(options, s.priceOption(sr, q, originZone.Code, destZone.Code))
		}
	}

	// Opsi dengan harga tak diketahui dibuang SEBELUM pemeringkatan agar tidak
	// menjadi "termurah" palsu hanya karena harganya 0.
	options = dropUnpriced(options)

	summary := s.applyScoring(options)

	res := &QuoteResult{
		QuoteID:          encodeQuoteID(q),
		OriginZone:       originZone.Code,
		DestinationZone:  destZone.Code,
		ChargeableWeight: s.chargeableWeight(q),
		Options:          options,
		SummaryLabels:    summary,
	}
	s.writeCache(ctx, cacheKey, res)
	return res, nil
}

// Recommend mengembalikan opsi yang direkomendasikan untuk quote yang sudah
// pernah dihitung. quote_id bersifat self-contained (memuat parameternya),
// sehingga rekomendasi dapat dihitung ulang tanpa penyimpanan server —
// cache hanya mempercepat, bukan syarat kebenaran.
func (s *Service) Recommend(ctx context.Context, quoteID string) (*QuoteResult, *Option, error) {
	q, err := decodeQuoteID(quoteID)
	if err != nil {
		return nil, nil, err
	}
	res, err := s.Quote(ctx, q)
	if err != nil {
		return nil, nil, err
	}
	for i := range res.Options {
		if res.Options[i].IsRecommended {
			return res, &res.Options[i], nil
		}
	}
	// Tidak ada opsi (rute tak dilayani) — jujur, bukan mengarang pemenang.
	return res, nil, nil
}

// applyScoring menilai seluruh opsi lalu menempelkan skor, label, dan
// rekomendasi ke masing-masing opsi. Mengembalikan ringkasan label.
func (s *Service) applyScoring(options []Option) scoring.Summary {
	if len(options) == 0 {
		return scoring.Summary{}
	}
	inputs := make([]scoring.Input, len(options))
	for i, opt := range options {
		inputs[i] = scoring.Input{
			Key:              scoring.Key(opt.Courier.Code, opt.Service.Code),
			TotalPrice:       opt.Price,
			EtaMinDays:       opt.EtaMinDays,
			EtaMaxDays:       opt.EtaMaxDays,
			ReliabilityScore: opt.ReliabilityScore,
			SupportsCOD:      opt.SupportsCOD,
			SupportsPickup:   opt.SupportsPickup,
		}
	}
	results, summary := s.scorer.Rank(inputs)
	for i := range options {
		options[i].Score = results[i].Score
		options[i].Labels = results[i].Labels
		options[i].IsRecommended = results[i].IsRecommended
		options[i].EstimatedSuccessRate = results[i].EstimatedSuccessRate
		options[i].Recommendation = results[i].Recommendation
	}
	return summary
}

// chargeableWeight menghitung berat chargeable tingkat quote memakai divisor
// default engine — pembanding tunggal antar kurir, terlepas dari divisor
// masing-masing layanan.
func (s *Service) chargeableWeight(q courier.RateQuery) float64 {
	return s.engine.Weights(q.ActualWeightKG, q.Dimensions, 0).Chargeable
}

// dropUnpriced membuang opsi tanpa harga positif.
func dropUnpriced(options []Option) []Option {
	out := options[:0]
	for _, opt := range options {
		if opt.Price > 0 {
			out = append(out, opt)
		}
	}
	return out
}

// priceOption menerapkan pricing engine pada satu hasil provider.
func (s *Service) priceOption(sr courier.ServiceRate, q courier.RateQuery, originZone, destZone string) Option {
	quote := s.engine.Price(sr.Price, pricing.QuoteOptions{
		IsCOD:                    q.IsCOD,
		CODAmount:                q.CODAmount,
		Insured:                  q.Insured,
		DeclaredValue:            q.DeclaredValue,
		ServiceSupportsCOD:       sr.Service.SupportsCOD,
		ServiceSupportsInsurance: sr.Service.SupportsInsurance,
	})
	// Zona asal dari provider bila ada (mock mengisinya); jika tidak, pakai
	// zona hasil resolusi service — jangan pernah mengirim string kosong.
	optOrigin := sr.OriginZone
	if optOrigin == "" {
		optOrigin = originZone
	}
	optDest := sr.DestinationZone
	if optDest == "" {
		optDest = destZone
	}
	return Option{
		Courier:          Party{Code: sr.Courier.Code, Name: sr.Courier.Name},
		Service:          Party{Code: sr.Service.Code, Name: sr.Service.Name},
		OriginZone:       optOrigin,
		DestinationZone:  optDest,
		ChargeableWeight: sr.ChargeableWeight,
		VolumetricWeight: sr.VolumetricWeight,
		Price:            quote.Total,
		BasePrice:        quote.BasePrice,
		Markup:           quote.Markup,
		CODFee:           quote.CODFee,
		InsuranceFee:     quote.InsuranceFee,
		EtaMinDays:       sr.Service.EtaMinDays,
		EtaMaxDays:       sr.Service.EtaMaxDays,
		ReliabilityScore: sr.Service.ReliabilityScore,
		SupportsCOD:      sr.Service.SupportsCOD,
		SupportsPickup:   sr.Service.SupportsPickup,
	}
}

// ServicesFor mengembalikan layanan untuk kurir tertentu berdasarkan kode.
// Digunakan oleh endpoint GET /api/v1/couriers/{code}/services.
func (s *Service) ServicesFor(ctx context.Context, code string) (*courier.Courier, []courier.Service, error) {
	couriers, err := s.repo.ListActive(ctx)
	if err != nil {
		return nil, nil, err
	}
	for i := range couriers {
		if !strings.EqualFold(couriers[i].Code, code) {
			continue
		}
		services, err := s.repo.ListActiveServices(ctx, couriers[i].ID)
		if err != nil {
			return nil, nil, err
		}
		if services == nil {
			services = []courier.Service{}
		}
		return &couriers[i], services, nil
	}
	return nil, nil, shared.NotFound("courier_not_found",
		fmt.Sprintf("Kurir dengan kode %s tidak ditemukan.", code))
}

// ListActive mengembalikan daftar kurir aktif untuk halaman registry.
func (s *Service) ListActive(ctx context.Context) ([]courier.Courier, error) {
	return s.repo.ListActive(ctx)
}

// ─── cache ───────────────────────────────────────────────────────────────

// readCache mengembalikan hasil tersimpan, atau nil bila tidak ada/tak valid.
func (s *Service) readCache(ctx context.Context, key string) *QuoteResult {
	if s.redis == nil {
		return nil
	}
	raw, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		// Cache miss (redis.Nil) dan gangguan Redis diperlakukan sama:
		// hitung ulang. Cache tidak boleh menjadi titik gagal.
		return nil
	}
	var res QuoteResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		s.log.Warn("cache tarif rusak — menghitung ulang", "key", key, "error", err)
		return nil
	}
	return &res
}

// writeCache menyimpan hasil. Kegagalan tulis hanya dicatat — bukan error.
func (s *Service) writeCache(ctx context.Context, key string, res *QuoteResult) {
	if s.redis == nil {
		return
	}
	payload, err := json.Marshal(res)
	if err != nil {
		s.log.Warn("gagal menyandikan cache tarif", "error", err)
		return
	}
	if err := s.redis.Set(ctx, key, payload, cacheTTL).Err(); err != nil {
		s.log.Warn("gagal menulis cache tarif", "key", key, "error", err)
	}
}

// cacheKeyForRateQuery membuat kunci cache dari parameter quote.
//
// Catatan: parameter org tidak ikut karena tarif bersifat platform-level
// (courier_rates tidak tenant-scoped) dan bobot scoring bersifat global.
// Bila kelak tarif per-tenant ada, org wajib masuk ke kunci ini.
func cacheKeyForRateQuery(q courier.RateQuery) string {
	parts := []string{
		q.OriginCityID,
		q.DestinationCityID,
		fmt.Sprintf("w%.3f", q.ActualWeightKG),
	}
	if q.Dimensions != nil {
		parts = append(parts, fmt.Sprintf("d%.1f-%.1f-%.1f",
			q.Dimensions.Length, q.Dimensions.Width, q.Dimensions.Height))
	}
	parts = append(parts,
		fmt.Sprintf("cod%t-%d", q.IsCOD, q.CODAmount),
		fmt.Sprintf("ins%t-%d", q.Insured, q.DeclaredValue),
	)
	return "rate:" + strings.Join(parts, ":")
}

// ─── quote_id ────────────────────────────────────────────────────────────

// quoteParams adalah bentuk ringkas query di dalam token quote_id.
type quoteParams struct {
	Origin      string  `json:"o"`
	Destination string  `json:"d"`
	Weight      float64 `json:"w"`
	Length      float64 `json:"l,omitempty"`
	Width       float64 `json:"wi,omitempty"`
	Height      float64 `json:"h,omitempty"`
	IsCOD       bool    `json:"c,omitempty"`
	CODAmount   int64   `json:"ca,omitempty"`
	Insured     bool    `json:"i,omitempty"`
	Declared    int64   `json:"dv,omitempty"`
}

// encodeQuoteID menyandikan parameter quote menjadi token yang dapat
// dikembalikan klien apa adanya. Bersifat read-only & tidak rahasia — klien
// tetap dapat memperoleh data yang sama lewat GET /couriers/rates.
func encodeQuoteID(q courier.RateQuery) string {
	p := quoteParams{
		Origin:      q.OriginCityID,
		Destination: q.DestinationCityID,
		Weight:      q.ActualWeightKG,
		IsCOD:       q.IsCOD,
		CODAmount:   q.CODAmount,
		Insured:     q.Insured,
		Declared:    q.DeclaredValue,
	}
	if q.Dimensions != nil {
		p.Length, p.Width, p.Height = q.Dimensions.Length, q.Dimensions.Width, q.Dimensions.Height
	}
	raw, err := json.Marshal(p)
	if err != nil {
		// Tidak mungkin gagal untuk struct sederhana; jaga tetap aman.
		return quoteIDPrefix
	}
	return quoteIDPrefix + base64.RawURLEncoding.EncodeToString(raw)
}

// decodeQuoteID memulihkan parameter dari token, menolak token asing.
func decodeQuoteID(id string) (courier.RateQuery, error) {
	if !strings.HasPrefix(id, quoteIDPrefix) {
		return courier.RateQuery{}, shared.Validation(map[string]string{
			"quote_id": "Format quote_id tidak dikenali.",
		})
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, quoteIDPrefix))
	if err != nil {
		return courier.RateQuery{}, shared.Validation(map[string]string{
			"quote_id": "quote_id rusak atau tidak lengkap.",
		})
	}
	var p quoteParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return courier.RateQuery{}, shared.Validation(map[string]string{
			"quote_id": "quote_id rusak atau tidak lengkap.",
		})
	}
	q := courier.RateQuery{
		OriginCityID:      p.Origin,
		DestinationCityID: p.Destination,
		ActualWeightKG:    p.Weight,
		IsCOD:             p.IsCOD,
		CODAmount:         p.CODAmount,
		Insured:           p.Insured,
		DeclaredValue:     p.Declared,
	}
	if p.Length > 0 && p.Width > 0 && p.Height > 0 {
		q.Dimensions = &shared.Dimensions{Length: p.Length, Width: p.Width, Height: p.Height}
	}
	if err := q.Validate(); err != nil {
		return courier.RateQuery{}, err
	}
	return q, nil
}
