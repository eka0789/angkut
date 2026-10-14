package scoring

import (
	"testing"
)

// testConfig menghasilkan konfigurasi bobot yang sama dengan default dev,
// tetapi ditulis eksplisit agar test tidak ikut berubah bila default bergeser.
func testConfig() Config {
	return Config{
		WeightPriceBps:       4000,
		WeightSpeedBps:       2500,
		WeightReliabilityBps: 3500,
		ReasonThresholdPct:   5,
		HighReliabilityScore: 90,
		ConfidenceBaseBps:    6000,
		ConfidenceSpanBps:    3500,
		MaxReasons:           3,
	}
}

func TestRankLabelsPerCriterion(t *testing.T) {
	// Empat opsi dengan pemenang berbeda per kriteria:
	//   murah  : harga terendah, ETA paling lambat, reliabilitas sedang
	//   cepat  : ETA tercepat, harga tertinggi
	//   andal  : reliabilitas tertinggi
	//   nilai  : kombinasi harga+reliabilitas terbaik (hampir semurah MURAH
	//            tetapi jauh lebih andal)
	in := []Input{
		{Key: "MURAH:REG", TotalPrice: 10000, EtaMinDays: 5, EtaMaxDays: 6, ReliabilityScore: 70},
		{Key: "CEPAT:BEST", TotalPrice: 30000, EtaMinDays: 1, EtaMaxDays: 1, ReliabilityScore: 60},
		{Key: "ANDAL:PRIME", TotalPrice: 25000, EtaMinDays: 3, EtaMaxDays: 4, ReliabilityScore: 99},
		{Key: "NILAI:EZ", TotalPrice: 12000, EtaMinDays: 3, EtaMaxDays: 3, ReliabilityScore: 90},
	}

	results, summary := New(testConfig()).Rank(in)

	if summary.Cheapest != "MURAH:REG" {
		t.Errorf("cheapest = %q, ingin MURAH:REG", summary.Cheapest)
	}
	if summary.Fastest != "CEPAT:BEST" {
		t.Errorf("fastest = %q, ingin CEPAT:BEST", summary.Fastest)
	}
	if summary.MostReliable != "ANDAL:PRIME" {
		t.Errorf("most_reliable = %q, ingin ANDAL:PRIME", summary.MostReliable)
	}
	if summary.BestValue != "NILAI:EZ" {
		t.Errorf("best_value = %q, ingin NILAI:EZ", summary.BestValue)
	}

	if got := len(results); got != len(in) {
		t.Fatalf("panjang hasil = %d, ingin %d", got, len(in))
	}
	// Setiap label harus menempel pada opsi pemenangnya.
	assertHasLabel(t, results[0], LabelBestPrice)
	assertHasLabel(t, results[1], LabelFastest)
	assertHasLabel(t, results[2], LabelMostReliable)
	assertHasLabel(t, results[3], LabelBestValue)

	// Skor wajib berada di rentang 0..100.
	for i, r := range results {
		if r.Score < 0 || r.Score > 100 {
			t.Errorf("opsi %d skor %v di luar 0..100", i, r.Score)
		}
	}
}

func TestRankExactlyOneRecommendation(t *testing.T) {
	in := []Input{
		{Key: "A:REG", TotalPrice: 10000, EtaMinDays: 3, EtaMaxDays: 4, ReliabilityScore: 70},
		{Key: "B:REG", TotalPrice: 30000, EtaMinDays: 1, EtaMaxDays: 2, ReliabilityScore: 95},
		{Key: "C:REG", TotalPrice: 20000, EtaMinDays: 2, EtaMaxDays: 3, ReliabilityScore: 85},
	}

	results, _ := New(testConfig()).Rank(in)

	count := 0
	for _, r := range results {
		if r.IsRecommended {
			count++
			if r.Recommendation == nil {
				t.Fatal("opsi rekomendasi wajib membawa alasan")
			}
			if len(r.Recommendation.Reasons) == 0 {
				t.Error("daftar alasan rekomendasi tidak boleh kosong")
			}
			if c := r.Recommendation.Confidence; c <= 0 || c > 1 {
				t.Errorf("confidence %v di luar (0,1]", c)
			}
		}
	}
	if count != 1 {
		t.Errorf("jumlah rekomendasi = %d, ingin tepat 1", count)
	}
}

func TestRankSingleOptionGetsFullConfidence(t *testing.T) {
	in := []Input{{Key: "A:REG", TotalPrice: 15000, EtaMinDays: 2, EtaMaxDays: 3, ReliabilityScore: 88}}

	results, summary := New(testConfig()).Rank(in)

	if !results[0].IsRecommended {
		t.Fatal("opsi tunggal harus direkomendasikan")
	}
	if summary.Cheapest != "A:REG" || summary.BestValue != "A:REG" {
		t.Error("ringkasan label harus menunjuk opsi tunggal yang ada")
	}
	// base 6000 + span 3500 × margin penuh (1.0) = 9500 bps = 0.95
	if got := results[0].Recommendation.Confidence; got != 0.95 {
		t.Errorf("confidence = %v, ingin 0.95", got)
	}
}

func TestRankEmptyInput(t *testing.T) {
	results, summary := New(testConfig()).Rank(nil)

	if len(results) != 0 {
		t.Fatalf("hasil kosong diharapkan, dapat %d", len(results))
	}
	if summary != (Summary{}) {
		t.Errorf("ringkasan harus kosong, dapat %+v", summary)
	}
}

func TestRankUnknownEtaDoesNotWinFastest(t *testing.T) {
	// Opsi tanpa ETA harus diperlakukan selambat batas kohort — bukan
	// otomatis dianggap tercepat karena ETA-nya 0.
	in := []Input{
		{Key: "TANPA_ETA:REG", TotalPrice: 10000, ReliabilityScore: 70},
		{Key: "CEPAT:BEST", TotalPrice: 11000, EtaMinDays: 1, EtaMaxDays: 2, ReliabilityScore: 70},
	}

	_, summary := New(testConfig()).Rank(in)

	if summary.Fastest != "CEPAT:BEST" {
		t.Errorf("fastest = %q, ingin CEPAT:BEST", summary.Fastest)
	}
}

func TestRankHighReliabilityReason(t *testing.T) {
	in := []Input{
		{Key: "A:REG", TotalPrice: 20000, EtaMinDays: 3, EtaMaxDays: 3, ReliabilityScore: 95},
		{Key: "B:REG", TotalPrice: 20000, EtaMinDays: 3, EtaMaxDays: 3, ReliabilityScore: 60},
	}

	results, _ := New(testConfig()).Rank(in)

	// Seri harga & ETA: pemenang ditentukan reliabilitas.
	if !results[0].IsRecommended {
		t.Fatal("opsi dengan reliabilitas tertinggi harus menang saat harga & ETA seri")
	}
	found := false
	for _, r := range results[0].Recommendation.Reasons {
		if r == "performa historis tinggi di kota tujuan" {
			found = true
		}
	}
	if !found {
		t.Errorf("alasan performa historis tidak muncul: %v", results[0].Recommendation.Reasons)
	}
}

func TestRankReasonsCapped(t *testing.T) {
	cfg := testConfig()
	cfg.MaxReasons = 1

	in := []Input{
		{Key: "A:REG", TotalPrice: 10000, EtaMinDays: 1, EtaMaxDays: 1, ReliabilityScore: 99},
		{Key: "B:REG", TotalPrice: 40000, EtaMinDays: 9, EtaMaxDays: 9, ReliabilityScore: 40},
	}

	results, _ := New(cfg).Rank(in)

	if got := len(results[0].Recommendation.Reasons); got != 1 {
		t.Errorf("jumlah alasan = %d, ingin dibatasi ke 1", got)
	}
}

func TestEstimatedSuccessRateNormalisedToUnit(t *testing.T) {
	in := []Input{
		{Key: "A:REG", TotalPrice: 10000, EtaMinDays: 2, EtaMaxDays: 3, ReliabilityScore: 92},
		{Key: "B:REG", TotalPrice: 20000, EtaMinDays: 2, EtaMaxDays: 3, ReliabilityScore: 50},
	}

	results, _ := New(testConfig()).Rank(in)

	if got := results[0].EstimatedSuccessRate; got != 0.92 {
		t.Errorf("success rate = %v, ingin 0.92", got)
	}
	if got := results[1].EstimatedSuccessRate; got != 0.5 {
		t.Errorf("success rate = %v, ingin 0.5", got)
	}
}

func TestKeyNormalisation(t *testing.T) {
	if got := Key("jne", "reg"); got != "JNE:REG" {
		t.Errorf("Key = %q, ingin JNE:REG", got)
	}
	if got := Key(" sicepat ", " best "); got != "SICEPAT:BEST" {
		t.Errorf("Key = %q, ingin SICEPAT:BEST", got)
	}
}

// assertHasLabel memeriksa label menempel pada hasil.
func assertHasLabel(t *testing.T, r Result, label string) {
	t.Helper()
	for _, l := range r.Labels {
		if l == label {
			return
		}
	}
	t.Errorf("label %q tidak ditemukan di %v", label, r.Labels)
}
