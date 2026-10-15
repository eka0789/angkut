// Package pricing adalah engine harga murni — tanpa DB, tanpa HTTP, tanpa
// I/O. SEMUA angka (rate, minimum, pembulatan, divisor) berasal dari Config
// yang diisi dari environment; tidak ada magic number di fungsi mana pun.
//
// Rumus inti:
//   - volumetric  = panjang × lebar × tinggi (cm) ÷ divisor layanan (cm³/kg)
//   - chargeable  = max(actual, volumetric), dibulatkan KE ATAS ke step
//   - cod_fee     = max(minimum, nilai COD × rate), dibulatkan ke step harga
//   - insurance   = max(minimum, nilai barang × rate), dibulatkan ke step harga
//   - markup      = flat + harga dasar × rate
//   - total       = dasar + markup + cod_fee + insurance, dibulatkan ke step
package pricing

import (
	"math"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// Config adalah seluruh parameter engine. Nilai diisi dari env
// (internal/config.PricingConfig) — satu sumber kebenaran, tanpa literal.
type Config struct {
	VolumetricDivisorDefault int
	WeightRoundUpStepKG      float64
	MaxChargeableWeightKG    float64
	CODFeeRateBps            int64 // 10000 = 100%
	CODFeeMin                int64
	InsuranceRateBps         int64
	InsuranceMinFee          int64
	MarkupFlat               int64
	MarkupRateBps            int64
	PriceRoundStep           int64
}

// Engine menghitung komponen harga dari parameter yang dikonfigurasi.
type Engine struct{ cfg Config }

// New membuat engine dari konfigurasi.
func New(cfg Config) *Engine { return &Engine{cfg: cfg} }

// Config mengembalikan konfigurasi engine (untuk validasi input pemanggil).
func (e *Engine) Config() Config { return e.cfg }

// WeightResult adalah hasil perhitungan berat satu paket.
type WeightResult struct {
	Actual     float64 // kg
	Volumetric float64 // kg (0 bila tanpa dimensi)
	Chargeable float64 // kg — dasar pencocokan tier tarif
	ExceedsMax bool    // chargeable melewati batas sanity
}

// Weights menghitung berat chargeable dari berat aktual dan dimensi opsional.
// Divisor 0/negatif memakai divisor default konfigurasi.
func (e *Engine) Weights(actual float64, dims *shared.Dimensions, divisor int) WeightResult {
	if divisor <= 0 {
		divisor = e.cfg.VolumetricDivisorDefault
	}
	var vol float64
	if dims != nil && dims.Length > 0 && dims.Width > 0 && dims.Height > 0 {
		vol = float64(dims.VolumetricWeight(divisor))
	}
	chargeable := math.Max(actual, vol)
	chargeable = roundUp(chargeable, e.cfg.WeightRoundUpStepKG)
	return WeightResult{
		Actual:     actual,
		Volumetric: vol,
		Chargeable: chargeable,
		ExceedsMax: chargeable > e.cfg.MaxChargeableWeightKG,
	}
}

// CODFee menghitung fee COD dari nilai yang ditagih ke penerima.
// amount <= 0 berarti tanpa COD → fee 0.
func (e *Engine) CODFee(amount int64) int64 {
	if amount <= 0 || e.cfg.CODFeeRateBps <= 0 {
		return 0
	}
	fee := e.cfg.CODFeeMin
	if computed := amount * e.cfg.CODFeeRateBps / 10000; computed > fee {
		fee = computed
	}
	return roundDown(fee, e.cfg.PriceRoundStep)
}

// InsuranceFee menghitung biaya asuransi dari nilai barang.
// declaredValue <= 0 berarti tanpa asuransi → fee 0.
func (e *Engine) InsuranceFee(declaredValue int64) int64 {
	if declaredValue <= 0 || e.cfg.InsuranceRateBps <= 0 {
		return 0
	}
	fee := e.cfg.InsuranceMinFee
	if computed := declaredValue * e.cfg.InsuranceRateBps / 10000; computed > fee {
		fee = computed
	}
	return roundDown(fee, e.cfg.PriceRoundStep)
}

// Quote adalah hasil penentuan harga satu opsi tarif.
type Quote struct {
	BasePrice    int64 // harga kurir (dari tier tarif)
	Markup       int64 // markup platform (flat + rate)
	CODFee       int64 // 0 bila non-COD
	InsuranceFee int64 // 0 bila tanpa asuransi
	Total        int64 // semua komponen, dibulatkan ke step
}

// Price menghitung total dari harga dasar kurir dan pilihan layanan.
// Kemampuan layanan (supports_cod/supports_insurance) ditegakkan di sini:
// fee hanya dihitung bila layanan memang mendukung.
func (e *Engine) Price(basePrice int64, opts QuoteOptions) Quote {
	markup := e.cfg.MarkupFlat
	if e.cfg.MarkupRateBps > 0 {
		markup += basePrice * e.cfg.MarkupRateBps / 10000
	}

	codFee := int64(0)
	if opts.IsCOD && opts.ServiceSupportsCOD {
		codFee = e.CODFee(opts.CODAmount)
	}
	insFee := int64(0)
	if opts.Insured && opts.ServiceSupportsInsurance {
		insFee = e.InsuranceFee(opts.DeclaredValue)
	}

	total := roundDown(basePrice+markup+codFee+insFee, e.cfg.PriceRoundStep)
	return Quote{
		BasePrice:    basePrice,
		Markup:       markup,
		CODFee:       codFee,
		InsuranceFee: insFee,
		Total:        total,
	}
}

// QuoteOptions adalah pilihan pemanggil untuk penentuan harga satu opsi.
type QuoteOptions struct {
	IsCOD                    bool
	CODAmount                int64
	Insured                  bool
	DeclaredValue            int64
	ServiceSupportsCOD       bool
	ServiceSupportsInsurance bool
}

// roundUp membulatkan w ke atas ke kelipatan step terkecil >= w.
func roundUp(w, step float64) float64 {
	if step <= 0 {
		return w
	}
	return math.Ceil(w/step) * step
}

// roundDown membulatkan amount ke bawah ke kelipatan step terdekat —
// pembulatan biaya konservatif (keuntungan platform tidak pernah berlebih).
func roundDown(amount, step int64) int64 {
	if step <= 0 || step == 1 {
		return amount
	}
	return amount - amount%step
}
