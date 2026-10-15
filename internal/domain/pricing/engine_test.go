package pricing_test

import (
	"testing"

	"github.com/angkut/angkut/apps/internal/domain/pricing"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// testConfig — nilai sengaja beda dari default agar kebocoran literal
// terdeteksi: COD 1% min 2500, asuransi 0.2% min 2500, markup 2% + 500,
// bulat harga 100, step berat 0.1.
func testConfig() pricing.Config {
	return pricing.Config{
		VolumetricDivisorDefault: 6000,
		WeightRoundUpStepKG:      0.1,
		MaxChargeableWeightKG:    100,
		CODFeeRateBps:            100,
		CODFeeMin:                2500,
		InsuranceRateBps:         20,
		InsuranceMinFee:          2500,
		MarkupFlat:               500,
		MarkupRateBps:            200,
		PriceRoundStep:           100,
	}
}

func TestWeights(t *testing.T) {
	e := pricing.New(testConfig())

	// Aktual dominan.
	w := e.Weights(2.03, nil, 6000)
	if w.Chargeable != 2.1 { // dibulatkan ke atas ke step 0.1
		t.Fatalf("chargeable 2.03 → 2.1, dapat %v", w.Chargeable)
	}
	if w.Volumetric != 0 {
		t.Fatalf("tanpa dimensi volumetrik harus 0, dapat %v", w.Volumetric)
	}

	// Volumetrik dominan: 60×40×40 = 96000 ÷ 6000 = 16 kg.
	dims := &shared.Dimensions{Length: 60, Width: 40, Height: 40}
	w = e.Weights(2, dims, 6000)
	if w.Volumetric != 16 || w.Chargeable != 16 {
		t.Fatalf("volumetrik 16, dapat %v/%v", w.Volumetric, w.Chargeable)
	}
	if w.ExceedsMax {
		t.Fatal("16 kg tidak boleh dianggap melebihi batas 100")
	}

	// Divisor per layanan menang atas default.
	w = e.Weights(2, dims, 12000)
	if w.Volumetric != 8 {
		t.Fatalf("divisor 12000 → 8 kg, dapat %v", w.Volumetric)
	}

	// Melebihi batas sanity.
	w = e.Weights(150, nil, 6000)
	if !w.ExceedsMax {
		t.Fatal("150 kg harus ditandai melebihi batas")
	}
}

func TestCODFee(t *testing.T) {
	e := pricing.New(testConfig())

	if got := e.CODFee(0); got != 0 {
		t.Fatalf("tanpa COD fee harus 0, dapat %d", got)
	}
	// 300.000 × 1% = 3000 > min 2500 → 3000 (sudah kelipatan 100).
	if got := e.CODFee(300_000); got != 3000 {
		t.Fatalf("COD 300rb → 3000, dapat %d", got)
	}
	// 100.000 × 1% = 1000 < min 2500 → 2500.
	if got := e.CODFee(100_000); got != 2500 {
		t.Fatalf("COD 100rb → min 2500, dapat %d", got)
	}
	// Pembulatan ke bawah: 555.555 × 1% = 5555.55 → 5555 → bulat 5500.
	if got := e.CODFee(555_555); got != 5500 {
		t.Fatalf("COD 555555 → 5500, dapat %d", got)
	}
}

func TestInsuranceFee(t *testing.T) {
	e := pricing.New(testConfig())

	if got := e.InsuranceFee(0); got != 0 {
		t.Fatalf("tanpa nilai barang fee harus 0, dapat %d", got)
	}
	// 5.000.000 × 0.2% = 10.000.
	if got := e.InsuranceFee(5_000_000); got != 10000 {
		t.Fatalf("asuransi 5jt → 10000, dapat %d", got)
	}
	// 500.000 × 0.2% = 1000 < min 2500 → 2500.
	if got := e.InsuranceFee(500_000); got != 2500 {
		t.Fatalf("asuransi 500rb → min 2500, dapat %d", got)
	}
}

func TestPrice(t *testing.T) {
	e := pricing.New(testConfig())

	// Dasar 20.000 + markup (500 + 2%) + COD 300rb (3000) → 24.000.
	q := e.Price(20000, pricing.QuoteOptions{
		IsCOD: true, CODAmount: 300_000,
		ServiceSupportsCOD: true,
	})
	if q.BasePrice != 20000 || q.Markup != 900 || q.CODFee != 3000 || q.Total != 23900 {
		t.Fatalf("salah: %+v", q)
	}
	// Total dibulatkan ke bawah ke 100.
	if q.Total%100 != 0 {
		t.Fatalf("total harus kelipatan 100, dapat %d", q.Total)
	}

	// Layanan tidak mendukung COD → fee tidak dihitung meski diminta.
	q = e.Price(20000, pricing.QuoteOptions{
		IsCOD: true, CODAmount: 300_000,
		ServiceSupportsCOD: false,
	})
	if q.CODFee != 0 {
		t.Fatalf("layanan tanpa COD tidak boleh kena COD fee, dapat %d", q.CODFee)
	}

	// Asuransi diminta tapi layanan tidak mendukung → 0.
	q = e.Price(20000, pricing.QuoteOptions{
		Insured: true, DeclaredValue: 5_000_000,
		ServiceSupportsInsurance: false,
	})
	if q.InsuranceFee != 0 {
		t.Fatalf("layanan tanpa asuransi tidak boleh kena fee, dapat %d", q.InsuranceFee)
	}
}

func TestZeroRatesMeanNoFee(t *testing.T) {
	// Rate 0 di config = fitur mati, bukan gratis-diam-diam salah.
	e := pricing.New(pricing.Config{
		VolumetricDivisorDefault: 6000,
		WeightRoundUpStepKG:      0.1,
		MaxChargeableWeightKG:    100,
		CODFeeRateBps:            0,
		InsuranceRateBps:         0,
		PriceRoundStep:           100,
	})
	if got := e.CODFee(1_000_000); got != 0 {
		t.Fatalf("COD rate 0 → fee 0, dapat %d", got)
	}
	if got := e.InsuranceFee(1_000_000); got != 0 {
		t.Fatalf("asuransi rate 0 → fee 0, dapat %d", got)
	}
}
