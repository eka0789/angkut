package shared

import (
	"fmt"
	"regexp"
	"strings"
)

// Money dinyatakan dalam rupiah utuh (BIGINT di DB). Tidak ada desimal pada IDR.
type Money int64

func (m Money) String() string { return fmt.Sprintf("%d", int64(m)) }

// Weight dalam kilogram, presisi 3 desimal sesuai NUMERIC(10,3).
type Weight float64

// Dimensions dalam sentimeter (NUMERIC(10,2)).
type Dimensions struct {
	Length float64
	Width  float64
	Height float64
}

// VolumeCm3 mengembalikan volume dalam cm³.
func (d Dimensions) VolumeCm3() float64 { return d.Length * d.Width * d.Height }

// VolumetricWeight menghitung berat volumetrik (divisor default 6000).
func (d Dimensions) VolumetricWeight(divisor int) Weight {
	if divisor <= 0 {
		divisor = 6000
	}
	return Weight(d.VolumeCm3() / float64(divisor))
}

// ChargeableWeight memilih yang lebih besar antara berat aktual dan volumetrik.
// Ini aturan kurir, bukan preferensi — jangan dioverride user.
func ChargeableWeight(actual Weight, dims *Dimensions, divisor int) Weight {
	if dims == nil {
		return actual
	}
	if v := dims.VolumetricWeight(divisor); v > actual {
		return v
	}
	return actual
}

// AddressPolicy memisahkan alamat dari PII yang tidak diperlukan logika.
type AddressPolicy struct {
	ContactName string
	Phone       string
	Street      string
	DistrictID  string
	CityID      string
	ProvinceID  string
	PostalCode  string
	Latitude    *float64
	Longitude   *float64
}

var awbPattern = regexp.MustCompile(`^[A-Z0-9-]{6,32}$`)

// AWBPolicy memvalidasi nomor resi secara format saja; keabsahan milik kurir.
func AWBPolicy(awb string) error {
	if !awbPattern.MatchString(strings.ToUpper(awb)) {
		return fmt.Errorf("format AWB tidak valid")
	}
	return nil
}

// NormalizePhone menyeragamkan nomor telepon Indonesia ke bentuk 62xxxxxxxxxx
// tanpa spasi/tanda baca. Dipakai untuk kunci pencarian pelanggan.
func NormalizePhone(phone string) (string, error) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	switch {
	case strings.HasPrefix(digits, "62"):
	case strings.HasPrefix(digits, "0"):
		digits = "62" + digits[1:]
	case strings.HasPrefix(digits, "8"):
		digits = "62" + digits
	default:
		return "", fmt.Errorf("nomor telepon harus nomor Indonesia (08.., 62..)")
	}
	if len(digits) < 10 || len(digits) > 15 {
		return "", fmt.Errorf("panjang nomor telepon tidak wajar")
	}
	return digits, nil
}

// MaskName menyamarkan nama untuk tampilan publik (tracking tanpa login).
// "Budi Santoso" -> "B*** S******"
func MaskName(name string) string {
	parts := strings.Fields(name)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		r := []rune(p)
		if len(r) == 0 {
			continue
		}
		out = append(out, string(r[0])+strings.Repeat("*", len(r)-1))
	}
	return strings.Join(out, " ")
}
