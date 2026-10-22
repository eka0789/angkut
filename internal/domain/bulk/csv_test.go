package bulk

import (
	"strings"
	"testing"
)

const csvGood = `nama penerima,telepon,alamat,kecamatan?,kode pos,berat (kg),COD
Budi Santoso,081234567890,"Jl. Melati No. 1, RT 01",Semarang,50241,1.5,50000
Siti Aminah,+62 811-2233-4455,"Jl. Kenanga No. 9",Bandung,40111,2,0
`

func TestParseCSVHeaderAliasIndonesia(t *testing.T) {
	rows, err := ParseCSV(strings.NewReader(csvGood), 100)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, ingin 2", len(rows))
	}
	r := rows[0]
	if r.RecipientName != "Budi Santoso" || r.RecipientPhone != "081234567890" {
		t.Errorf("baris 1 salah: %+v", r)
	}
	if r.PostalCode != "50241" || r.WeightKG != 1.5 || r.CODAmount != 50000 {
		t.Errorf("angka baris 1 salah: %+v", r)
	}
	if rows[1].CODAmount != 0 {
		t.Errorf("COD baris 2 = %d, ingin 0", rows[1].CODAmount)
	}
}

func TestParseCSVKolomWajibHilangDitolak(t *testing.T) {
	_, err := ParseCSV(strings.NewReader("nama,telepon\nBudi,0812\n"), 100)
	if err == nil || !strings.Contains(err.Error(), "kolom wajib") {
		t.Errorf("err = %v, ingin penolakan kolom wajib", err)
	}
}

func TestParseCSVIsiRusakTidakMembatalkanFile(t *testing.T) {
	// Baris rusak (berat bukan angka, kode pos aneh) TETAP diparse — nilai 0
	// diteruskan dan akan muncul sebagai issue pada tahap validasi baris.
	csv := "recipient_name,phone,street,postal_code,weight_kg\n" +
		"Budi,081234567890,Jl. A,50241,abc\n" +
		"Siti,081234567891,Jl. B,40111,2\n"
	rows, err := ParseCSV(strings.NewReader(csv), 100)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, ingin 2", len(rows))
	}
	if rows[0].WeightKG != 0 {
		t.Errorf("berat rusak = %v, ingin 0 (menunggu validasi)", rows[0].WeightKG)
	}
	if issues := ValidateRow(rows[0], Options{}); len(issues) == 0 {
		t.Error("baris berat rusak harus bermasalah saat validasi")
	}
	if issues := ValidateRow(rows[1], Options{}); len(issues) != 0 {
		t.Errorf("baris sehat harus valid; issue = %v", issues)
	}
}

func TestParseCSVBatasBaris(t *testing.T) {
	var b strings.Builder
	b.WriteString("recipient_name,phone,street,postal_code,weight_kg\n")
	for i := 0; i < 5; i++ {
		b.WriteString("Budi,081234567890,Jl. A,50241,1\n")
	}
	if _, err := ParseCSV(strings.NewReader(b.String()), 3); err == nil {
		t.Error("melebihi batas baris harus ditolak")
	}
}

func TestParseCSVTanpaBarisDataDitolak(t *testing.T) {
	_, err := ParseCSV(strings.NewReader("recipient_name,phone,street,postal_code,weight_kg\n"), 10)
	if err == nil {
		t.Error("file header-saja harus ditolak")
	}
}
