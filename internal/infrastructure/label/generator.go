// Package label membuat dokumen label pengiriman (PDF) beserta payload QR dan
// barcode. Murni presentasi: membaca data shipment yang sudah tersedia, tidak
// menyentuh DB atau jaringan. Ukuran & tata letak berasal dari Config.
package label

import (
	"bytes"
	"fmt"
	"image/png"
	"strings"
	"time"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/qr"
	"github.com/go-pdf/fpdf"

	"github.com/angkut/angkut/apps/internal/domain/shipment"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Config mengatur tata letak label. Semua angka dalam milimeter.
type Config struct {
	// Format label: "a4" (satu label per halaman) atau "thermal"
	// (label 100×150 mm untuk printer termal).
	Format string

	// Margin & ukuran blok.
	MarginMM   float64
	PaddingMM  float64
	TitleSize  float64
	BodySize   float64
	MonoSize   float64
	QRSizeMM   float64
	BarcodeHMM float64
	BarcodeWMM float64

	// ThermalWMM/ThermalHMM adalah ukuran halaman mode thermal.
	ThermalWMM float64
	ThermalHMM float64
}

// DefaultConfig mengembalikan tata letak bawaan yang wajar untuk A4 & thermal.
func DefaultConfig() Config {
	return Config{
		Format:     "a4",
		MarginMM:   12,
		PaddingMM:  4,
		TitleSize:  16,
		BodySize:   10,
		MonoSize:   11,
		QRSizeMM:   32,
		BarcodeHMM: 16,
		BarcodeWMM: 70,
		ThermalWMM: 100,
		ThermalHMM: 150,
	}
}

// Party adalah identitas pengirim/penerima pada label (bukan entitas domain —
// label hanya butuh teks yang tercetak).
type Party struct {
	Name    string
	Phone   string
	Address string
	City    string
}

// Data adalah seluruh isi satu label. Semua field wajib sudah di-resolve
// pemanggil (label tidak melakukan lookup).
type Data struct {
	Shipment  *shipment.Shipment
	Store     string
	Origin    Party
	Dest      Party
	PrintedAt time.Time
}

// Generator menghasilkan PDF label.
type Generator struct{ cfg Config }

// New membuat generator label.
func New(cfg Config) *Generator { return &Generator{cfg: cfg} }

// Generate menghasilkan PDF satu label. Mengembalikan error hanya bila
// prasyarat data tidak terpenuhi — bukan untuk masalah I/O (tak ada I/O di sini).
func (g *Generator) Generate(d Data) ([]byte, error) {
	if d.Shipment == nil {
		return nil, fmt.Errorf("data shipment wajib diisi")
	}
	s := d.Shipment

	pdf := fpdf.NewCustom(&fpdf.InitType{
		OrientationStr: "P",
		UnitStr:        "mm",
		Size:           fpdf.SizeType{Wd: g.pageWidth(), Ht: g.pageHeight()},
	})
	pdf.SetMargins(g.cfg.MarginMM, g.cfg.MarginMM, g.cfg.MarginMM)
	pdf.SetAutoPageBreak(false, g.cfg.MarginMM)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", g.cfg.BodySize)

	g.header(pdf, d, s)
	g.parties(pdf, d)
	g.details(pdf, s)
	g.codes(pdf, s)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("menulis PDF: %w", err)
	}
	return buf.Bytes(), nil
}

// pageWidth/pageHeight memilih ukuran halaman sesuai format.
func (g *Generator) pageWidth() float64 {
	if g.cfg.Format == "thermal" {
		return g.cfg.ThermalWMM
	}
	return 210 // A4
}

func (g *Generator) pageHeight() float64 {
	if g.cfg.Format == "thermal" {
		return g.cfg.ThermalHMM
	}
	return 297 // A4
}

// contentWidth adalah lebar area cetak.
func (g *Generator) contentWidth() float64 {
	return g.pageWidth() - 2*g.cfg.MarginMM
}

// header mencetak judul, kurir/layanan, resi, dan waktu cetak.
func (g *Generator) header(pdf *fpdf.Fpdf, d Data, s *shipment.Shipment) {
	pdf.SetFont("Helvetica", "B", g.cfg.TitleSize)
	pdf.CellFormat(g.contentWidth(), 10, "LABEL PENGIRIMAN", "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", g.cfg.BodySize)
	if d.Store != "" {
		pdf.CellFormat(g.contentWidth(), 5, "Toko: "+d.Store, "", 1, "L", false, 0, "")
	}
	pdf.CellFormat(g.contentWidth(), 5,
		fmt.Sprintf("Kurir: %s  |  Layanan: %s", s.CourierCode, s.ServiceCode),
		"", 1, "L", false, 0, "")

	// Resi dicetak besar: petugas memindai/membacanya dari jauh.
	pdf.SetFont("Courier", "B", g.cfg.MonoSize+4)
	pdf.CellFormat(g.contentWidth(), 9, "RESI: "+s.AWB, "", 1, "L", false, 0, "")

	if !d.PrintedAt.IsZero() {
		pdf.SetFont("Helvetica", "", g.cfg.BodySize-2)
		pdf.CellFormat(g.contentWidth(), 5,
			"Dicetak: "+d.PrintedAt.Format("02 Jan 2006 15:04"), "", 1, "L", false, 0, "")
	}
	pdf.Ln(g.cfg.PaddingMM)
}

// parties mencetak blok pengirim & penerima berdampingan.
func (g *Generator) parties(pdf *fpdf.Fpdf, d Data) {
	colW := (g.contentWidth() - g.cfg.PaddingMM) / 2
	lineH := 4.5

	pdf.SetFont("Helvetica", "B", g.cfg.BodySize)
	pdf.CellFormat(colW, lineH, "PENGIRIM", "", 0, "L", false, 0, "")
	pdf.CellFormat(colW, lineH, "PENERIMA", "", 1, "L", false, 0, "")

	// Cetak berdampingan: kiri pengirim, kanan penerima, baris demi baris.
	pdf.SetFont("Helvetica", "", g.cfg.BodySize)
	origLines := partyLines(d.Origin)
	destLines := partyLines(d.Dest)
	rows := len(origLines)
	if len(destLines) > rows {
		rows = len(destLines)
	}
	for i := 0; i < rows; i++ {
		left, right := "", ""
		if i < len(origLines) {
			left = origLines[i]
		}
		if i < len(destLines) {
			right = destLines[i]
		}
		pdf.CellFormat(colW, lineH, left, "", 0, "L", false, 0, "")
		pdf.CellFormat(colW, lineH, right, "", 1, "L", false, 0, "")
	}
	pdf.Ln(g.cfg.PaddingMM / 2)
}

// partyLines menyusun baris teks satu pihak.
func partyLines(p Party) []string {
	lines := make([]string, 0, 4)
	if p.Name != "" {
		lines = append(lines, p.Name)
	}
	if p.Phone != "" {
		lines = append(lines, "Telp: "+p.Phone)
	}
	if p.Address != "" {
		lines = append(lines, p.Address)
	}
	if p.City != "" {
		lines = append(lines, p.City)
	}
	return lines
}

// details mencetak rincian berat, harga, COD, dan isi paket.
func (g *Generator) details(pdf *fpdf.Fpdf, s *shipment.Shipment) {
	lineH := 5.0
	pdf.SetFont("Helvetica", "B", g.cfg.BodySize)
	pdf.CellFormat(g.contentWidth(), lineH, "RINCIAN", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", g.cfg.BodySize)

	row := func(k, v string) {
		pdf.CellFormat(45, lineH, k, "", 0, "L", false, 0, "")
		pdf.CellFormat(g.contentWidth()-45, lineH, v, "", 1, "L", false, 0, "")
	}
	row("Berat aktual", fmt.Sprintf("%.2f kg", s.ActualWeight))
	if s.VolumetricWeight > 0 {
		row("Berat volumetrik", fmt.Sprintf("%.2f kg", s.VolumetricWeight))
	}
	row("Berat chargeable", fmt.Sprintf("%.2f kg", s.ChargeableWeight))
	row("Nilai kirim", formatIDR(s.ShippingPrice))
	if s.IsCOD {
		row("COD", "YA — "+formatIDR(s.CODAmount))
		if s.CODFee > 0 {
			row("Biaya COD", formatIDR(s.CODFee))
		}
	} else {
		row("COD", "Tidak")
	}
	if s.DeclaredValue > 0 {
		row("Nilai barang", formatIDR(s.DeclaredValue))
	}
	if len(s.Items) > 0 {
		names := make([]string, 0, len(s.Items))
		for _, it := range s.Items {
			names = append(names, fmt.Sprintf("%s x%d", it.NameSnapshot, it.Qty))
		}
		row("Isi", joinLimited(names, 3))
	}
	if s.Notes != "" {
		row("Catatan", s.Notes)
	}
	pdf.Ln(g.cfg.PaddingMM)
}

// codes mencetak barcode 1D (Code128) dan QR code di bagian bawah label.
func (g *Generator) codes(pdf *fpdf.Fpdf, s *shipment.Shipment) {
	y := pdf.GetY()

	// Barcode Code128 dari AWB.
	if img, err := code128PNG(shipment.BarcodeValue(s.AWB), g.cfg.BarcodeWMM, g.cfg.BarcodeHMM); err == nil {
		pdf.RegisterImageOptionsReader("barcode", fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(img))
		pdf.ImageOptions("barcode", g.cfg.MarginMM, y, g.cfg.BarcodeWMM, g.cfg.BarcodeHMM,
			false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	}

	// QR memuat payload yang sama dengan yang dikembalikan API.
	qx := g.pageWidth() - g.cfg.MarginMM - g.cfg.QRSizeMM
	qrPayload := shipment.QRPayload(s.ID, s.AWB)
	if img, err := qrPNG(qrPayload, g.cfg.QRSizeMM); err == nil {
		pdf.RegisterImageOptionsReader("qr", fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(img))
		pdf.ImageOptions("qr", qx, y, g.cfg.QRSizeMM, g.cfg.QRSizeMM,
			false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	}

	pdf.SetY(y + g.cfg.QRSizeMM + 2)
	pdf.SetFont("Courier", "", g.cfg.BodySize-2)
	pdf.CellFormat(g.contentWidth(), 4, s.AWB, "", 1, "C", false, 0, "")
}

// ─── pembuat gambar kode ─────────────────────────────────────────────────

// code128PNG menghasilkan PNG barcode Code128 dengan lebar/tinggi tertentu.
func code128PNG(value string, widthMM, heightMM float64) ([]byte, error) {
	bc, err := code128.Encode(value)
	if err != nil {
		return nil, err
	}
	return scaleAndEncode(bc, widthMM, heightMM)
}

// qrPNG menghasilkan PNG QR code persegi.
func qrPNG(value string, sizeMM float64) ([]byte, error) {
	// Level koreksi M: toleran terhadap noda/lipatan pada label fisik.
	qc, err := qr.Encode(value, qr.M, qr.Auto)
	if err != nil {
		return nil, err
	}
	return scaleAndEncode(qc, sizeMM, sizeMM)
}

// scaleAndEncode menskalakan barcode ke ukuran piksel lalu menyandikannya PNG.
// 8 px/mm ≈ 203 dpi, memadai untuk printer label termal umum.
func scaleAndEncode(bc barcode.Barcode, widthMM, heightMM float64) ([]byte, error) {
	const pxPerMM = 8
	w := int(widthMM * pxPerMM)
	h := int(heightMM * pxPerMM)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	scaled, err := barcode.Scale(bc, w, h)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, scaled); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// joinLimited menggabungkan maksimal n item lalu menambahkan "+N lainnya".
func joinLimited(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:n], ", ") + fmt.Sprintf(" +%d lainnya", len(items)-n)
}

// formatIDR memformat nominal rupiah untuk label cetak (tanpa simbol berat).
func formatIDR(amount int64) string { return platform.FormatIDR(amount) }
