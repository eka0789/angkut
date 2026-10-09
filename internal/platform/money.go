package platform

import (
	"fmt"
	"strings"
)

// PriceRule adalah aturan harga platform. SEMUA angka berasal dari konfigurasi
// atau DB (aturan: jangan hardcode harga/bobot/divisor di logika bisnis).
type PriceRule struct {
	VolumetricDivisor int   // default 6000
	InsurancePercent  int   // persen dari nilai barang (mis. 2 = 0,2%)
	InsuranceMinimum  int64 // minimum biaya asuransi (rupiah)
	MarkupFlat        int64 // markup platform per shipment (rupiah)
	MarkupPercent     int   // markup platform dari harga kurir (persen)
}

// DefaultPriceRule dipakai bila DB/config belum memberi nilai. Nilai ini
// sengaja konservatif dan boleh diubah tanpa menyentuh logika.
func DefaultPriceRule() PriceRule {
	return PriceRule{
		VolumetricDivisor: 6000,
		InsurancePercent:  2,
		InsuranceMinimum:  2500,
		MarkupFlat:        0,
		MarkupPercent:     0,
	}
}

// InsuranceFee menghitung biaya asuransi dari declared value.
func (r PriceRule) InsuranceFee(declaredValue int64) int64 {
	if declaredValue <= 0 || r.InsurancePercent <= 0 {
		return 0
	}
	fee := declaredValue * int64(r.InsurancePercent) / 1000 // persen dalam per-mil
	if fee < r.InsuranceMinimum {
		fee = r.InsuranceMinimum
	}
	return fee
}

// Total menghitung harga jual dari harga dasar kurir + COD + asuransi.
func (r PriceRule) Total(basePrice, insuranceFee, codFee int64) int64 {
	// Markup dihitung dari harga dasar saja, bukan dari biaya pihak ketiga.
	markup := r.MarkupFlat + basePrice*int64(r.MarkupPercent)/100
	return basePrice + markup + insuranceFee + codFee
}

// Money formatting ringkas untuk pesan/notifikasi (bukan untuk UI — UI pakai Intl).
func FormatIDR(amount int64) string {
	neg := amount < 0
	if neg {
		amount = -amount
	}
	s := fmt.Sprintf("%d", amount)
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	out := "Rp" + strings.Join(parts, ".")
	if neg {
		return "-" + out
	}
	return out
}
