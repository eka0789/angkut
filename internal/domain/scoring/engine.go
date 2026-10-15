// Package scoring memeringkat opsi tarif dan memilih rekomendasi — murni,
// tanpa DB, tanpa HTTP, tanpa I/O. Seluruh bobot dan ambang berasal dari
// Config (internal/config.ScoringConfig, isi dari env SCORING_*); tidak ada
// angka bisnis yang ditulis sebagai literal di sini.
//
// Rumus:
//
//	price_score = harga_termurah / harga_opsi            (1.0 untuk termurah)
//	speed_score = estimasi_tercepat / estimasi_opsi      (1.0 untuk tercepat)
//	rel_score   = reliability_score / 100
//	value_score = price_score × rel_score
//	skor        = (w_price×price + w_speed×speed + w_rel×rel) / 10000 × 100
//
// ETA tak diketahui diperlakukan KONSERVATIF: dianggap selambat opsi lain
// yang ETA-nya diketahui, bukan diberi hadiah. Reliability ≤ 0 dianggap
// tanpa bukti (skor 0), bukan netral.
package scoring

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Label kanonik yang dipakai kontrak API dan ringkasan label.
const (
	LabelBestPrice    = "best_price"
	LabelFastest      = "fastest"
	LabelMostReliable = "most_reliable"
	LabelBestValue    = "best_value"
)

// Skala keluaran skor (0..100) — konstanta presentasi, bukan tuas bisnis.
const maxScore = 100.0

// Presisi pembulatan keluaran. Alasan memakai bilangan bulat persen.
const (
	scoreDecimals       = 1
	confidenceDecimals  = 2
	successRateDecimals = 4
	percentScale        = 100
)

// Config adalah parameter engine scoring. Nilainya diisi dari env.
type Config struct {
	// Bobot kriteria dalam basis points; ketiganya harus berjumlah 10000.
	WeightPriceBps       int64
	WeightSpeedBps       int64
	WeightReliabilityBps int64

	// ReasonThresholdPct adalah beda minimum (persen) agar sebuah alasan
	// dianggap layak ditulis — mencegah alasan sepele ("1% lebih cepat").
	ReasonThresholdPct int

	// HighReliabilityScore adalah ambang reliability_score agar opsi disebut
	// berperforma historis tinggi.
	HighReliabilityScore float64

	// Keyakinan = base + span × margin skor juara vs runner-up (0..1).
	ConfidenceBaseBps int64
	ConfidenceSpanBps int64

	// MaxReasons membatasi panjang daftar alasan.
	MaxReasons int
}

// Engine memeringkat opsi tarif dari konfigurasi yang diberikan.
type Engine struct{ cfg Config }

// New membuat engine scoring.
func New(cfg Config) *Engine { return &Engine{cfg: cfg} }

// Config mengembalikan konfigurasi engine (dipakai pemanggil untuk validasi).
func (e *Engine) Config() Config { return e.cfg }

// Input adalah satu opsi tarif yang akan dinilai. Key adalah identitas stabil
// opsi dalam bentuk "KODE_KURIR:KODE_LAYANAN" — dipakai sebagai nilai label.
type Input struct {
	Key              string
	TotalPrice       int64
	EtaMinDays       int
	EtaMaxDays       int
	ReliabilityScore float64 // 0..100
	SupportsCOD      bool
	SupportsPickup   bool
}

// Recommendation adalah alasan terstruktur untuk opsi yang direkomendasikan.
type Recommendation struct {
	Reasons    []string `json:"reasons"`
	Confidence float64  `json:"confidence"`
}

// Result adalah hasil penilaian satu opsi, sejajar indeks dengan Input.
type Result struct {
	Score                float64         `json:"score"`
	Labels               []string        `json:"labels"`
	IsRecommended        bool            `json:"is_recommended"`
	EstimatedSuccessRate float64         `json:"estimated_success_rate"`
	Recommendation       *Recommendation `json:"recommendation,omitempty"`
}

// Summary memetakan setiap label ke key opsi pemenangnya.
type Summary struct {
	Cheapest     string `json:"cheapest"`
	Fastest      string `json:"fastest"`
	MostReliable string `json:"most_reliable"`
	BestValue    string `json:"best_value"`
}

// Rank menilai seluruh opsi dan mengembalikan hasil per opsi plus ringkasan
// label. Keluaran selalu sepanjang masukan, walau masukan kosong.
func (e *Engine) Rank(in []Input) ([]Result, Summary) {
	results := make([]Result, len(in))
	for i := range results {
		results[i].Labels = []string{}
	}
	if len(in) == 0 {
		return results, Summary{}
	}

	// ── Normalisasi ──────────────────────────────────────────────────────
	minPrice := minPositivePrice(in)
	minEta, maxEta := etaBounds(in)

	priceScore := make([]float64, len(in))
	speedScore := make([]float64, len(in))
	relScore := make([]float64, len(in))
	valueScore := make([]float64, len(in))
	expDays := make([]float64, len(in))

	for i, opt := range in {
		priceScore[i] = ratioScore(minPrice, opt.TotalPrice)
		relScore[i] = clamp01(opt.ReliabilityScore / maxScore)
		expDays[i] = expectedDays(opt)
		speedScore[i] = speed(expDays[i], minEta, maxEta)
		valueScore[i] = priceScore[i] * relScore[i]

		results[i].EstimatedSuccessRate = roundTo(relScore[i], successRateDecimals)
		results[i].Score = roundTo(e.weighted(priceScore[i], speedScore[i], relScore[i])*maxScore, scoreDecimals)
	}

	// ── Label per kriteria ───────────────────────────────────────────────
	// `fastest` hanya boleh jatuh ke opsi yang ETA-nya diketahui; opsi tanpa
	// ETA diskor konservatif sehingga bisa SERI dengan yang tercepat, dan seri
	// tidak boleh memberinya label hanya karena kebetulan urutan indeks.
	fastestIdx := argMaxSubset(speedScore, knownIndices(expDays))

	summary := Summary{
		Cheapest:     in[argMax(priceScore)].Key,
		Fastest:      in[fastestIdx].Key,
		MostReliable: in[argMax(relScore)].Key,
		BestValue:    in[argMax(valueScore)].Key,
	}
	attach := func(key, label string) {
		for i := range in {
			if in[i].Key == key {
				results[i].Labels = append(results[i].Labels, label)
				return
			}
		}
	}
	attach(summary.Cheapest, LabelBestPrice)
	attach(summary.Fastest, LabelFastest)
	attach(summary.MostReliable, LabelMostReliable)
	attach(summary.BestValue, LabelBestValue)

	// ── Rekomendasi ──────────────────────────────────────────────────────
	scores := make([]float64, len(in))
	for i := range results {
		scores[i] = results[i].Score
	}
	best := argMax(scores)
	results[best].IsRecommended = true
	results[best].Recommendation = &Recommendation{
		Reasons:    e.reasons(in, best, expDays, relScore),
		Confidence: e.confidence(scores, best),
	}

	return results, summary
}

// weighted menghitung skor berbobot dalam skala 0..1.
func (e *Engine) weighted(price, speed, rel float64) float64 {
	total := float64(e.cfg.WeightPriceBps)*price +
		float64(e.cfg.WeightSpeedBps)*speed +
		float64(e.cfg.WeightReliabilityBps)*rel
	if total <= 0 {
		return 0
	}
	return total / 10000
}

// reasons menyusun alasan untuk opsi juara, dibandingkan terhadap median
// kohort agar klaimnya jujur ("lebih cepat dari separuh opsi"), bukan
// dibandingkan terhadap angka yang dipilih sendiri.
func (e *Engine) reasons(in []Input, best int, expDays, relScore []float64) []string {
	threshold := e.cfg.ReasonThresholdPct
	out := make([]string, 0, e.cfg.MaxReasons)

	medDays := median(knownValues(expDays))
	medRel := median(relScore)

	if medDays > 0 && expDays[best] > 0 {
		if pct := percentDiff(medDays, expDays[best]); pct >= threshold {
			out = append(out, fmt.Sprintf("%d%% lebih cepat untuk rute ini", pct))
		}
	}
	if medRel > 0 && relScore[best] > 0 {
		medFailure := maxScore - medRel*maxScore
		bestFailure := maxScore - relScore[best]*maxScore
		if medFailure > 0 {
			if pct := percentDiff(medFailure, bestFailure); pct >= threshold {
				out = append(out, fmt.Sprintf("%d%% failure rate lebih rendah", pct))
			}
		}
	}
	if relScore[best]*maxScore >= e.cfg.HighReliabilityScore {
		out = append(out, "performa historis tinggi di kota tujuan")
	}
	if len(out) > e.cfg.MaxReasons {
		out = out[:e.cfg.MaxReasons]
	}
	if len(out) == 0 {
		// Jujur: tidak ada keunggulan menonjol, sebut apa adanya.
		out = append(out, fmt.Sprintf("skor keseluruhan tertinggi dari %d opsi", len(in)))
	}
	return out
}

// confidence mengubah margin skor juara vs runner-up menjadi tingkat keyakinan.
// Kohort satu opsi dianggap margin penuh (tidak ada pembanding) — keyakinan
// maksimum, karena tidak ada opsi lain yang bisa lebih baik.
func (e *Engine) confidence(scores []float64, best int) float64 {
	margin := 1.0
	if len(scores) > 1 {
		second := math.Inf(-1)
		for i, s := range scores {
			if i == best {
				continue
			}
			if s > second {
				second = s
			}
		}
		margin = clamp01((scores[best] - second) / maxScore)
	}
	bps := float64(e.cfg.ConfidenceBaseBps) + float64(e.cfg.ConfidenceSpanBps)*margin
	if bps > 10000 {
		bps = 10000
	}
	if bps < 0 {
		bps = 0
	}
	return roundTo(bps/10000, confidenceDecimals)
}

// ─── helper murni ────────────────────────────────────────────────────────

// ratioScore mengembalikan best/value, dibatasi 0..1. Nilai non-positif
// berarti tanpa informasi → 0 (konservatif).
func ratioScore(best, value int64) float64 {
	if best <= 0 || value <= 0 {
		return 0
	}
	return clamp01(float64(best) / float64(value))
}

// speed menilai kecepatan relatif. ETA tak diketahui (0) diperlakukan
// selambat batas atas kohort; bila seluruh kohort tak diketahui, kecepatan
// tidak membedakan (semua 1.0) alih-alih mengarang peringkat.
func speed(days, minEta, maxEta float64) float64 {
	if days <= 0 {
		if maxEta <= 0 {
			return 1
		}
		days = maxEta
	}
	if minEta <= 0 {
		return 1
	}
	return clamp01(minEta / days)
}

// expectedDays memakai rata-rata ETA minimum dan maksimum; 0 berarti tak diketahui.
func expectedDays(opt Input) float64 {
	if opt.EtaMinDays <= 0 && opt.EtaMaxDays <= 0 {
		return 0
	}
	minD, maxD := opt.EtaMinDays, opt.EtaMaxDays
	if minD <= 0 {
		minD = maxD
	}
	if maxD <= 0 {
		maxD = minD
	}
	return float64(minD+maxD) / 2
}

// minPositivePrice mengabaikan harga non-positif (tanpa informasi tarif).
func minPositivePrice(in []Input) int64 {
	var min int64
	for _, opt := range in {
		if opt.TotalPrice > 0 && (min == 0 || opt.TotalPrice < min) {
			min = opt.TotalPrice
		}
	}
	return min
}

// etaBounds mengembalikan ETA tercepat dan terlambat yang diketahui.
func etaBounds(in []Input) (minEta, maxEta float64) {
	for _, opt := range in {
		d := expectedDays(opt)
		if d <= 0 {
			continue
		}
		if minEta == 0 || d < minEta {
			minEta = d
		}
		if d > maxEta {
			maxEta = d
		}
	}
	return minEta, maxEta
}

// knownValues membuang nol agar median tidak tertarik ke bawah oleh data kosong.
func knownValues(v []float64) []float64 {
	out := make([]float64, 0, len(v))
	for _, x := range v {
		if x > 0 {
			out = append(out, x)
		}
	}
	return out
}

// median mengembalikan median; kosong berarti 0.
func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sorted := append([]float64(nil), v...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

// percentDiff menghitung berapa persen `better` lebih kecil dari `baseline`.
// Negatif berarti lebih buruk — pemanggil membandingkan dengan ambang.
func percentDiff(baseline, better float64) int {
	if baseline <= 0 {
		return 0
	}
	return int(math.Round((baseline - better) / baseline * percentScale))
}

// argMax mengembalikan indeks nilai terbesar; seri dimenangkan indeks pertama.
func argMax(v []float64) int {
	best := 0
	for i, x := range v {
		if x > v[best] {
			best = i
		}
	}
	return best
}

// knownIndices mengembalikan indeks bernilai > 0; kosong bila tidak ada.
func knownIndices(v []float64) []int {
	out := make([]int, 0, len(v))
	for i, x := range v {
		if x > 0 {
			out = append(out, i)
		}
	}
	return out
}

// argMaxSubset mengembalikan indeks terbesar dari subset; bila subset kosong
// ia mundur ke seluruh kohort agar pemanggil selalu mendapat indeks sah.
func argMaxSubset(v []float64, subset []int) int {
	if len(subset) == 0 {
		return argMax(v)
	}
	best := subset[0]
	for _, i := range subset[1:] {
		if v[i] > v[best] {
			best = i
		}
	}
	return best
}

func clamp01(v float64) float64 {
	if v < 0 || math.IsNaN(v) {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// roundTo membulatkan ke sejumlah desimal.
func roundTo(v float64, decimals int) float64 {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	factor := math.Pow(10, float64(decimals))
	return math.Round(v*factor) / factor
}

// Key menyusun identitas opsi "KODE_KURIR:KODE_LAYANAN" untuk ringkasan label.
func Key(courierCode, serviceCode string) string {
	return strings.ToUpper(strings.TrimSpace(courierCode)) + ":" + strings.ToUpper(strings.TrimSpace(serviceCode))
}
