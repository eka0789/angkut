package bulk

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// headerAliases memetakan nama kolom yang lazim (Indonesia maupun Inggris,
// dengan spasi/underscore/garis bawah) ke field baris. Ekspor lewat fungsi
// CsvHeaders untuk menulis contoh/template di FE.
var headerAliases = map[string]string{
	"nama_penerima":  "recipient_name",
	"penerima":       "recipient_name",
	"recipient_name": "recipient_name",
	"recipient":      "recipient_name",
	"telepon":        "recipient_phone",
	"no_hp":          "recipient_phone",
	"nomor_telepon":  "recipient_phone",
	"phone":          "recipient_phone",
	"hp":             "recipient_phone",
	"alamat":         "street",
	"alamat_jalan":   "street",
	"street":         "street",
	"address":        "street",
	"kodepos":        "postal_code",
	"kode_pos":       "postal_code",
	"postal_code":    "postal_code",
	"zip":            "postal_code",
	"berat":          "weight_kg",
	"berat_kg":       "weight_kg",
	"weight":         "weight_kg",
	"weight_kg":      "weight_kg",
	"panjang":        "length_cm",
	"length":         "length_cm",
	"length_cm":      "length_cm",
	"lebar":          "width_cm",
	"width":          "width_cm",
	"width_cm":       "width_cm",
	"tinggi":         "height_cm",
	"height":         "height_cm",
	"height_cm":      "height_cm",
	"cod":            "cod_amount",
	"cod_amount":     "cod_amount",
	"nominal_cod":    "cod_amount",
	"nilai_barang":   "declared_value",
	"declared_value": "declared_value",
	"declared":       "declared_value",
	"asuransi":       "insurance",
	"insurance":      "insurance",
	"catatan":        "notes",
	"note":           "notes",
	"notes":          "notes",
}

// CsvHeaders mengembalikan nama kolom kanonik yang didukung (untuk template).
func CsvHeaders() []string {
	return []string{
		"recipient_name", "recipient_phone", "street", "postal_code", "weight_kg",
		"length_cm", "width_cm", "height_cm", "cod_amount", "declared_value",
		"insurance", "notes",
	}
}

// ParseCSV membaca unggahan CSV mentah menjadi baris-baris ternormalisasi.
//
// Aturan:
//   - baris pertama adalah header; nama kolom dinormalisasi lewat alias
//     (huruf besar/kecil, spasi, underscore, garis bawah tak peduli);
//   - kolom wajib yang tidak ada = kesalahan STRUKTUR (seluruh file ditolak);
//   - kesalahan isi satu sel TIDAK membatalkan file — nilainya dikirim apa
//     adanya dan akan terlihat sebagai issue baris saat validasi
//     ("per baris independen" berlaku sejak dari parser);
//   - dibatasi maxRows agar tidak ada file raksasa diproses penuh.
func ParseCSV(r io.Reader, maxRows int) ([]Row, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // jumlah kolom bebas; kekurangan kolom jadi issue baris
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("membaca header: %w", err)
	}
	idx := map[string]int{}
	for i, h := range header {
		// Normalisasi longgar: "berat (kg)" → "berat_kg", "Kode-Pos" →
		// "kode_pos", tanda baca ekspor Excel dibuang.
		key := strings.ToLower(strings.TrimSpace(h))
		key = strings.NewReplacer("(", "", ")", "", "?", "", ".", "", ",", "").Replace(key)
		key = strings.ReplaceAll(key, " ", "_")
		key = strings.ReplaceAll(key, "-", "_")
		key = strings.ReplaceAll(key, "__", "_")
		if canon, ok := headerAliases[key]; ok {
			if _, dup := idx[canon]; !dup {
				idx[canon] = i
			}
		}
	}
	for _, required := range []string{"recipient_name", "recipient_phone", "street", "postal_code", "weight_kg"} {
		if _, ok := idx[required]; !ok {
			return nil, fmt.Errorf("kolom wajib %q tidak ada pada header", required)
		}
	}

	var rows []Row
	for lineNo := 2; ; lineNo++ {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("baris %d: %w", lineNo, err)
		}
		if maxRows > 0 && len(rows) >= maxRows {
			return nil, fmt.Errorf("jumlah baris melebihi batas %d", maxRows)
		}
		get := func(field string) string {
			i, ok := idx[field]
			if !ok || i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}
		row := Row{
			RecipientName:  get("recipient_name"),
			RecipientPhone: get("recipient_phone"),
			Street:         get("street"),
			PostalCode:     get("postal_code"),
			WeightKG:       parseFloat(strings.ReplaceAll(get("weight_kg"), ",", ".")),
			LengthCM:       parseFloatPtr(get("length_cm")),
			WidthCM:        parseFloatPtr(get("width_cm")),
			HeightCM:       parseFloatPtr(get("height_cm")),
			CODAmount:      parseInt(get("cod_amount")),
			DeclaredValue:  parseInt(get("declared_value")),
			Insurance:      parseBool(get("insurance")),
			Notes:          get("notes"),
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("file tidak memuat baris data")
	}
	return rows, nil
}

func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0 // 0 → terdeteksi "berat harus > 0" saat validasi
	}
	return v
}

func parseFloatPtr(s string) *float64 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v := parseFloat(s)
	return &v
}

func parseInt(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Hapus pemisah ribuan yang sering muncul dari ekspor Excel ("150.000"
	// atau "150,000"); nilai rupiah tidak pernah memakai desimal.
	s = strings.TrimSuffix(s, ".00")
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		return 0
	}
	return int64(f)
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "y", "ya", "yes", "true", "t", "aktif":
		return true
	}
	return false
}
