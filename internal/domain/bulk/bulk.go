// Package bulk berisi model & aturan validasi pengiriman massal (bulk
// shipping) yang murni: tanpa akses DB, tanpa jaringan.
//
// Prinsip "per baris independen" (CODING_RULES): setiap baris divalidasi dan
// dievaluasi sendiri; satu baris bermasalah tidak membatalkan baris lain.
// Semua fungsi di sini bisa diuji tanpa infrastruktur.
package bulk

import (
	"fmt"
	"strings"
	"time"
)

// Status sebuah baris dalam hidupnya: valid → creating → created/failed.
// invalid berarti baris ditolak sebelum eksekusi.
type Status string

const (
	StatusValid    Status = "valid"
	StatusInvalid  Status = "invalid"
	StatusCreating Status = "creating"
	StatusCreated  Status = "created"
	StatusFailed   Status = "failed"
)

// JobStatus adalah status job massal.
type JobStatus string

const (
	JobPending    JobStatus = "pending"
	JobProcessing JobStatus = "processing"
	JobCompleted  JobStatus = "completed"
	JobFailed     JobStatus = "failed"
)

// Valid melaporkan apakah status baris dikenal.
func (s Status) Valid() bool {
	switch s {
	case StatusValid, StatusInvalid, StatusCreating, StatusCreated, StatusFailed:
		return true
	}
	return false
}

// Valid melaporkan apakah status job dikenal.
func (s JobStatus) Valid() bool {
	switch s {
	case JobPending, JobProcessing, JobCompleted, JobFailed:
		return true
	}
	return false
}

// Row adalah satu baris input bulk setelah dinormalisasi (mentah dari CSV
// atau JSON). Field mengikuti kolom tabel bulk_job_rows; tag JSON dipakai
// untuk menyimpan baris unggahan sebagai jsonb.
type Row struct {
	RecipientName  string   `json:"recipient_name"`
	RecipientPhone string   `json:"recipient_phone"`
	Street         string   `json:"street"`
	PostalCode     string   `json:"postal_code"`
	WeightKG       float64  `json:"weight_kg"`
	LengthCM       *float64 `json:"length_cm,omitempty"`
	WidthCM        *float64 `json:"width_cm,omitempty"`
	HeightCM       *float64 `json:"height_cm,omitempty"`
	CODAmount      int64    `json:"cod_amount"`
	DeclaredValue  int64    `json:"declared_value"`
	Insurance      bool     `json:"insurance"`
	Notes          string   `json:"notes,omitempty"`
}

// Severity membedakan masalah yang memblokir baris dari yang hanya advisori.
type Severity string

const (
	// SeverityError menolak baris (tidak dieksekusi).
	SeverityError Severity = "error"
	// SeverityWarning tetap mengeksekusi baris, tetapi laporan memberi tahu
	// pengirim bahwa sebuah opsi dilepas/diubah (mis. asuransi tanpa dukungan
	// layanan).
	SeverityWarning Severity = "warning"
)

// Issue adalah satu masalah pada satu field baris. Kosongnya Severity berarti
// error — keputusan sadar: default harus menolak, bukan meneruskan.
type Issue struct {
	Field    string   `json:"field"`
	Message  string   `json:"message"`
	Severity Severity `json:"severity,omitempty"`
}

// IsError melaporkan apakah issue ini memblokir baris.
func (i Issue) IsError() bool { return i.Severity == "" || i.Severity == SeverityError }

// Options membatasi angka yang wajar untuk sebuah kiriman.
type Options struct {
	// MaxWeightKG membatasi berat aktual per baris (default bila nol: 100 kg).
	MaxWeightKG float64
}

func (o Options) maxWeight() float64 {
	if o.MaxWeightKG <= 0 {
		return 100
	}
	return o.MaxWeightKG
}

// NormalizePhone menormalkan nomor telepon Indonesia ke bentuk lokal
// 0XXXXXXXXXX (digit saja). Mengembalikan error bila bukan nomor yang wajar.
func NormalizePhone(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("nomor telepon wajib diisi")
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteByte(byte(r))
		case r == '+' || r == '-' || r == ' ' || r == '(' || r == ')':
			// Pemisah yang lazim dibiarkan hilang.
		default:
			return "", fmt.Errorf("nomor telepon hanya boleh berisi angka")
		}
	}
	digits := b.String()

	switch {
	case strings.HasPrefix(digits, "+62") || strings.HasPrefix(digits, "62"):
		digits = "0" + digits[2:]
	}
	if len(digits) < 9 || len(digits) > 15 || strings.TrimLeft(digits, "0") == "" {
		return "", fmt.Errorf("nomor telepon tidak wajar (9–15 digit)")
	}
	if strings.HasPrefix(digits, "08") || strings.HasPrefix(digits, "02") || strings.HasPrefix(digits, "03") ||
		strings.HasPrefix(digits, "05") || strings.HasPrefix(digits, "07") || strings.HasPrefix(digits, "09") {
		return digits, nil
	}
	return "", fmt.Errorf("nomor telepon harus diawali kode area/operator (08…)")
}

// NormalizePostalCode menormalkan kode pos menjadi 5 digit.
func NormalizePostalCode(raw string) (string, error) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.TrimSpace(raw))
	if len(digits) != 5 {
		return "", fmt.Errorf("kode pos harus 5 digit")
	}
	return digits, nil
}

// ValidateRow memeriksa satu baris secara independen. Hasilnya daftar issue
// per field; kosong = baris valid. Pemeriksaan butuh referensi (kode pos
// terdaftar, kurir mendukung COD, dsb.) dilakukan di lapisan service dan
// digabungkan ke issue yang sama.
func ValidateRow(r Row, opts Options) []Issue {
	var issues []Issue
	add := func(field, msg string) { issues = append(issues, Issue{Field: field, Message: msg}) }

	if strings.TrimSpace(r.RecipientName) == "" {
		add("recipient_name", "Nama penerima wajib diisi.")
	}
	if _, err := NormalizePhone(r.RecipientPhone); err != nil {
		add("recipient_phone", err.Error())
	}
	if strings.TrimSpace(r.Street) == "" {
		add("street", "Alamat jalan wajib diisi.")
	}
	if _, err := NormalizePostalCode(r.PostalCode); err != nil {
		add("postal_code", err.Error())
	}
	if r.WeightKG <= 0 {
		add("weight_kg", "Berat harus lebih dari 0.")
	} else if r.WeightKG > opts.maxWeight() {
		add("weight_kg", fmt.Sprintf("Berat melebihi batas %.0f kg.", opts.maxWeight()))
	}
	if r.CODAmount < 0 {
		add("cod_amount", "Nilai COD tidak boleh negatif.")
	}
	if r.DeclaredValue < 0 {
		add("declared_value", "Nilai barang tidak boleh negatif.")
	}
	if r.Insurance && r.DeclaredValue <= 0 {
		add("declared_value", "Asuransi memerlukan nilai barang lebih dari 0.")
	}
	for _, d := range []struct {
		name string
		v    *float64
	}{
		{"length_cm", r.LengthCM}, {"width_cm", r.WidthCM}, {"height_cm", r.HeightCM},
	} {
		if d.v != nil && *d.v < 0 {
			add(d.name, "Dimensi tidak boleh negatif.")
		}
	}
	return issues
}

// DuplicateKey mengembalikan kunci duplikat alami sebuah baris: penerima
// dengan telepon + alamat + kode pos yang sama pada file yang sama hampir
// pasti kiriman ganda dari salah satu baris.
func DuplicateKey(r Row) string {
	phone, _ := NormalizePhone(r.RecipientPhone)
	postal, _ := NormalizePostalCode(r.PostalCode)
	return strings.ToLower(phone + "|" + strings.TrimSpace(r.Street) + "|" + postal)
}

// FindDuplicates menandai kemunculan KE-DUA dan seterusnya dari kunci yang
// sama sebagai issue "duplikat baris N". Baris pertama tetap valid — pilihan
// sadar: biarkan kiriman pertama tercatat, tolak pengulangannya.
func FindDuplicates(rows []Row) map[int][]Issue {
	seen := make(map[string]int, len(rows))
	out := make(map[int][]Issue)
	for i, r := range rows {
		key := DuplicateKey(r)
		if first, ok := seen[key]; ok {
			out[i] = append(out[i], Issue{
				Field:   "_row",
				Message: fmt.Sprintf("Baris duplikat dengan baris %d (penerima & alamat sama).", first+1),
			})
			continue
		}
		seen[key] = i
	}
	return out
}

// Upload adalah hasil parse unggahan CSV: baris terparse yang siap
// divalidasi lalu disubmit. File mentah tidak disimpan — hanya baris
// ternormalisasi, sehingga parser adalah satu-satunya tempat mentah dibaca.
type Upload struct {
	ID             string
	OrganizationID string
	Filename       string
	RowCount       int
	Rows           []Row
	CreatedBy      string
	CreatedAt      time.Time
	ConsumedAt     *time.Time
}

// Job adalah entitas job massal (disimpan oleh lapisan infrastruktur).
// Tag JSON dipakai langsung oleh endpoint status job (GET /bulk/{job_id}).
type Job struct {
	ID                string     `json:"id"`
	OrganizationID    string     `json:"organization_id"`
	IdempotencyKey    string     `json:"idempotency_key,omitempty"`
	Status            JobStatus  `json:"status"`
	CourierCode       string     `json:"courier_code"`
	ServiceCode       string     `json:"service_code"`
	OriginWarehouseID string     `json:"origin_warehouse_id"`
	TotalRows         int        `json:"total_rows"`
	AcceptedRows      int        `json:"accepted_rows"`
	RejectedRows      int        `json:"rejected_rows"`
	ProcessedRows     int        `json:"processed_rows"`
	CreatedRows       int        `json:"created_rows"`
	FailedRows        int        `json:"failed_rows"`
	ErrorSummary      string     `json:"error_summary,omitempty"`
	CreatedBy         string     `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// RowResult adalah hasil sebuah baris setelah validasi/eksekusi.
type RowResult struct {
	JobID      string    `json:"job_id"`
	RowNo      int       `json:"row_no"`
	Row        Row       `json:"row"`
	Status     Status    `json:"status"`
	Issues     []Issue   `json:"issues,omitempty"`
	ShipmentID string    `json:"shipment_id,omitempty"`
	AWB        string    `json:"awb,omitempty"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Valid melaporkan apakah baris lolos validasi (boleh dieksekusi).
func (rr RowResult) Valid() bool { return rr.Status == StatusValid }
