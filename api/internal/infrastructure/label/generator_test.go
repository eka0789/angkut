package label

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/angkut/angkut/apps/api/internal/domain/shipment"
)

func sampleData() Data {
	return Data{
		Shipment: &shipment.Shipment{
			ID:               "01a13a29-e358-77c9-b34c-f5254dc65298",
			OrganizationID:   "org-1",
			CourierCode:      "jne",
			ServiceCode:      "REG",
			AWB:              "JNE260101000042",
			Status:           shipment.StatusPendingPickup,
			ActualWeight:     1.5,
			VolumetricWeight: 1.2,
			ChargeableWeight: 1.5,
			ShippingPrice:    21_000,
			CostPrice:        18_500,
			IsCOD:            true,
			CODAmount:        350_000,
			CODFee:           3_500,
			DeclaredValue:    350_000,
			Notes:            "Fragile",
			Items: []shipment.Item{
				{NameSnapshot: "Sepatu Running", Qty: 2},
				{NameSnapshot: "Kaos", Qty: 1},
			},
		},
		Store:     "Toko Utama",
		Origin:    Party{Name: "Gudang Jakarta", Phone: "021123456", Address: "Jl. Industri 1", City: "Jakarta"},
		Dest:      Party{Name: "Budi Santoso", Phone: "081234599001", Address: "Jl. Merdeka 10", City: "Bandung"},
		PrintedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	}
}

func TestGenerateProducesValidPDF(t *testing.T) {
	pdf, err := New(DefaultConfig()).Generate(sampleData())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(pdf) < 500 {
		t.Fatalf("PDF terlalu kecil (%d byte) — kemungkinan kosong", len(pdf))
	}
	// Setiap PDF diawali magic "%PDF-".
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Errorf("bukan PDF valid, awalan: %q", pdf[:min(8, len(pdf))])
	}
	if !bytes.Contains(pdf, []byte("%%EOF")) {
		t.Error("PDF tanpa penanda EOF — dokumen terpotong")
	}
}

func TestGenerateThermalFormat(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Format = "thermal"
	pdf, err := New(cfg).Generate(sampleData())
	if err != nil {
		t.Fatalf("Generate thermal: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Error("output thermal bukan PDF valid")
	}
	// Ukuran thermal = 100×150mm ≈ 283×425pt. MediaBox harus mencerminkan itu.
	if !bytes.Contains(pdf, []byte("/MediaBox")) {
		t.Error("PDF tanpa MediaBox")
	}
}

func TestGenerateRequiresShipment(t *testing.T) {
	if _, err := New(DefaultConfig()).Generate(Data{}); err == nil {
		t.Fatal("data tanpa shipment harus menghasilkan error")
	}
}

func TestGenerateSurvivesMinimalData(t *testing.T) {
	// Tanpa store, tanpa penerima, tanpa item — label tetap harus terbentuk
	// (mungkin jelek, tapi tidak boleh gagal atau panic).
	minimal := Data{
		Shipment: &shipment.Shipment{
			ID: "s1", CourierCode: "jne", ServiceCode: "REG", AWB: "JNE1",
			ActualWeight: 1, ChargeableWeight: 1, ShippingPrice: 10000,
		},
	}
	if _, err := New(DefaultConfig()).Generate(minimal); err != nil {
		t.Fatalf("label dengan data minimal gagal: %v", err)
	}
}

func TestQRPayloadRoundTrip(t *testing.T) {
	// Payload yang dicetak harus sama dengan yang dikembalikan API, dan dapat
	// diurai balik menjadi shipment_id + awb oleh pemindai.
	payload := shipment.QRPayload("ship-42", "JNE260101000042")
	parts := strings.Split(payload, "|")
	if len(parts) != 3 {
		t.Fatalf("payload = %q, ingin 3 bagian", payload)
	}
	if parts[0] != "ANGKUT" || parts[1] != "ship-42" || parts[2] != "JNE260101000042" {
		t.Errorf("payload terurai salah: %v", parts)
	}
}

func TestBarcodeAndQRImagesGenerated(t *testing.T) {
	// Barcode Code128 dari AWB alfanumerik harus bisa dibuat.
	img, err := code128PNG("JNE260101000042", 70, 16)
	if err != nil {
		t.Fatalf("code128PNG: %v", err)
	}
	if !bytes.HasPrefix(img, []byte("\x89PNG")) {
		t.Error("barcode bukan PNG")
	}
	if len(img) < 200 {
		t.Errorf("barcode terlalu kecil: %d byte", len(img))
	}

	qrImg, err := qrPNG("ANGKUT|s1|JNE1", 32)
	if err != nil {
		t.Fatalf("qrPNG: %v", err)
	}
	if !bytes.HasPrefix(qrImg, []byte("\x89PNG")) {
		t.Error("QR bukan PNG")
	}
}

func TestJoinLimitedTruncates(t *testing.T) {
	got := joinLimited([]string{"a", "b", "c", "d"}, 3)
	if !strings.Contains(got, "+1 lainnya") {
		t.Errorf("joinLimited = %q, ingin menyebut '+1 lainnya'", got)
	}
	got = joinLimited([]string{"a", "b"}, 3)
	if got != "a, b" {
		t.Errorf("joinLimited = %q, ingin 'a, b'", got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
