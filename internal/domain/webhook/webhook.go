// Package webhook berisi penerimaan event dari pihak ketiga (kurir, store,
// payment). Prinsip utamanya: SIMPAN DULU, PROSES KEMUDIAN.
//
// Payload mentah selalu disimpan sebelum diparsing, sehingga perubahan format
// di sisi provider tidak menghilangkan data — event tetap bisa diproses ulang
// setelah parser diperbaiki.
package webhook

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// Status adalah keadaan satu event webhook selama diproses.
type Status string

const (
	StatusReceived  Status = "received"
	StatusProcessed Status = "processed"
	StatusFailed    Status = "failed"
	StatusIgnored   Status = "ignored"
)

// Algo adalah algoritma HMAC yang dipakai provider. Nilai HARUS sama dengan
// CHECK di migrasi 0018.
type Algo string

const (
	AlgoSHA256 Algo = "hmac-sha256"
	AlgoSHA1   Algo = "hmac-sha1"
)

// Valid melaporkan apakah algoritma didukung.
func (a Algo) Valid() bool { return a == AlgoSHA256 || a == AlgoSHA1 }

// Event adalah satu event masuk dari pihak ketiga.
type Event struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organization_id,omitempty"`
	Provider       string         `json:"provider"`
	EventID        string         `json:"event_id,omitempty"`
	EventType      string         `json:"event_type,omitempty"`
	PayloadHash    string         `json:"payload_hash"`
	SignatureValid bool           `json:"signature_valid"`
	SignatureError string         `json:"signature_error,omitempty"`
	Payload        map[string]any `json:"payload"`

	// RawBody adalah byte ASLI yang diterima. Disimpan apa adanya ke kolom
	// raw_body agar event dapat diproses ulang setelah parser diperbaiki.
	// Sengaja tanpa tag json: byte mentah tidak pernah dikirim ke klien.
	RawBody []byte `json:"-"`

	ContentType string         `json:"content_type,omitempty"`
	Headers     map[string]any `json:"headers,omitempty"`
	AWB         string         `json:"awb,omitempty"`
	Status      Status         `json:"status"`
	Attempts    int            `json:"attempts"`
	LastError   string         `json:"last_error,omitempty"`
	SourceIP    string         `json:"source_ip,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// Validate memeriksa invariant event masuk.
func (e *Event) Validate() error {
	fields := make(map[string]string)
	if strings.TrimSpace(e.Provider) == "" {
		fields["provider"] = "Provider wajib diisi."
	}
	if strings.TrimSpace(e.PayloadHash) == "" {
		fields["payload_hash"] = "Hash payload wajib diisi."
	}
	if e.Status == "" {
		e.Status = StatusReceived
	}
	if len(fields) > 0 {
		return shared.Validation(fields)
	}
	return nil
}

// VerifySignature memeriksa HMAC atas body mentah terhadap signature yang
// dikirim provider. Perbandingan memakai hmac.Equal (konstan waktu) sehingga
// tidak membocorkan informasi lewat perbedaan waktu.
//
// signatureHeader bisa berisi beberapa nilai dipisahkan koma (sebagian provider
// mengirim "algo=...,signature=..."), maka setiap kandidat diperiksa.
func VerifySignature(algo Algo, secret string, rawBody []byte, signatureHeader string) error {
	if secret == "" {
		return shared.New("WEBHOOK_SECRET_MISSING",
			"Rahasia signature belum dikonfigurasi untuk provider ini.", shared.ErrForbidden)
	}
	header := strings.TrimSpace(signatureHeader)
	if header == "" {
		return shared.New("WEBHOOK_SIGNATURE_MISSING",
			"Header signature tidak ada pada permintaan ini.", shared.ErrForbidden)
	}
	if !algo.Valid() {
		return shared.New("WEBHOOK_ALGO_UNSUPPORTED",
			"Algoritma signature tidak didukung.", shared.ErrForbidden)
	}

	expected := computeMAC(algo, secret, rawBody)
	for _, candidate := range signatureCandidates(header) {
		if hmac.Equal([]byte(strings.ToLower(candidate)), []byte(expected)) {
			return nil
		}
	}
	return shared.New("WEBHOOK_SIGNATURE_INVALID",
		"Signature tidak cocok dengan isi permintaan.", shared.ErrForbidden)
}

// computeMAC menghitung HMAC heksadesimal huruf kecil.
func computeMAC(algo Algo, secret string, body []byte) string {
	switch algo {
	case AlgoSHA1:
		m := hmac.New(sha1.New, []byte(secret))
		m.Write(body)
		return hex.EncodeToString(m.Sum(nil))
	default:
		m := hmac.New(sha256.New, []byte(secret))
		m.Write(body)
		return hex.EncodeToString(m.Sum(nil))
	}
}

// signatureCandidates memecah header menjadi kandidat signature tunggal.
// Format yang umum ditemui:
//
//	"<hex>"                            → apa adanya
//	"sha256=<hex>"                     → ambil setelah '='
//	"algo=sha256, sig=<hex>"           → ambil pasangan yang mengandung '='
func signatureCandidates(header string) []string {
	parts := strings.FieldsFunc(header, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';'
	})
	out := make([]string, 0, len(parts)+1)
	out = append(out, header)
	for _, p := range parts {
		if i := strings.LastIndexByte(p, '='); i >= 0 && i+1 < len(p) {
			out = append(out, p[i+1:])
			continue
		}
		out = append(out, p)
	}
	return out
}

// HashPayload menghasilkan identitas isi payload. Dipakai sebagai kunci
// dedup ketika provider tidak mengirim event_id. SHA-256 tanpa kunci: ini
// murni identitas isi, bukan alat keamanan.
func HashPayload(rawBody []byte) string {
	sum := sha256.Sum256(rawBody)
	return hex.EncodeToString(sum[:])
}

// ParseResult adalah hasil ekstraksi ringan dari payload untuk kebutuhan
// operasional (bukan pengganti parser domain).
type ParseResult struct {
	AWB       string
	EventType string
}

// ExtractAWB mencari nomor resi pada payload secara defensif.
//
// Payload kurir tidak seragam, jadi beberapa kunci umum dicoba. Nilai kosong
// berarti tidak ditemukan — itu BUKAN kesalahan; event tetap tersimpan mentah.
func ExtractAWB(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	candidates := []string{"awb", "AWB", "waybill", "waybill_no", "tracking_number", "resi", "shipment_id_external"}
	for _, key := range candidates {
		if v, ok := payload[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	// Sebagian provider membungkus data di dalam objek.
	for _, key := range []string{"data", "shipment", "detail"} {
		if nested, ok := payload[key].(map[string]any); ok {
			if awb := ExtractAWB(nested); awb != "" {
				return awb
			}
		}
	}
	return ""
}

// ExtractEventType mencari jenis event pada payload.
func ExtractEventType(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	for _, key := range []string{"event", "event_type", "type", "status"} {
		if v, ok := payload[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ExtractEventID mencari id event dari provider.
func ExtractEventID(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	for _, key := range []string{"event_id", "id", "notification_id", "message_id"} {
		if v, ok := payload[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ─── sentinel error ──────────────────────────────────────────────────────

var (
	ErrNotFound   = shared.NotFound("WEBHOOK_NOT_FOUND", "Event webhook tidak ditemukan.")
	ErrDuplicate  = shared.Conflict("WEBHOOK_DUPLICATE", "Event webhook ini sudah pernah diterima.")
	ErrSecretless = shared.NotFound("WEBHOOK_SECRET_NOT_FOUND", "Rahasia webhook untuk provider ini belum diatur.")
)

// ErrFmt membungkus error verifikasi dengan nama provider untuk log yang jelas.
func ErrFmt(provider string, err error) error {
	return fmt.Errorf("webhook %s: %w", provider, err)
}
