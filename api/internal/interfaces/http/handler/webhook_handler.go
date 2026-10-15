package handler

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/webhook"
)

// maxWebhookBody membatasi ukuran body webhook. Nilai ini harus konsisten
// dengan batas di application layer (webhookapp) — dua lapis pertahanan.
const maxWebhookBody = 1 << 20 // 1 MiB

// WebhookService adalah antarmuka minimal service webhook untuk handler.
type WebhookService interface {
	Ingest(ctx context.Context, in webhook.IngestInput, signatureHeader string) (*webhook.Result, error)
	List(ctx context.Context, provider string, status webhook.Status, limit, offset int) ([]webhook.Event, int, error)
}

// WebhookHandler menangani event masuk dari pihak ketiga.
//
// Endpoint ini PUBLIK (dipanggil kurir, bukan pengguna), sehingga keamanannya
// bergantung sepenuhnya pada verifikasi HMAC signature — bukan pada token.
type WebhookHandler struct {
	svc WebhookService
}

// NewWebhookHandler membuat handler webhook.
func NewWebhookHandler(svc WebhookService) *WebhookHandler {
	return &WebhookHandler{svc: svc}
}

// RegisterRoutes mendaftarkan rute di /api/v1/webhooks.
func (h *WebhookHandler) RegisterRoutes(r chi.Router) {
	r.Post("/{provider}", h.Ingest)
	r.Get("/events", h.List)
}

// Ingest menangani POST /api/v1/webhooks/{provider}.
//
// Body dibaca MENTAH dan diteruskan apa adanya ke service: signature dihitung
// atas byte asli, sehingga membaca ulang setelah parse akan merusak verifikasi.
func (h *WebhookHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	if provider == "" {
		response.Fail(w, ValidationError(map[string]string{"provider": "Provider wajib diisi."}))
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody+1))
	if err != nil {
		response.Fail(w, shared.Validation(map[string]string{
			"body": "Body permintaan tidak dapat dibaca.",
		}))
		return
	}
	if len(raw) == 0 {
		response.Fail(w, shared.Validation(map[string]string{"body": "Body permintaan kosong."}))
		return
	}
	if len(raw) > maxWebhookBody {
		response.Fail(w, shared.Validation(map[string]string{
			"body": "Body melebihi batas ukuran yang diizinkan.",
		}))
		return
	}

	// Nama header signature berbeda antar provider; service yang tahu nama
	// pastinya (dari konfigurasi), jadi SEMUA header diteruskan apa adanya dan
	// service memilih yang benar. Ini menghindari handler menebak-nebak.
	sigHeader := extractSignatureHeader(r)

	res, err := h.svc.Ingest(r.Context(), webhook.IngestInput{
		Provider:    provider,
		RawBody:     raw,
		ContentType: r.Header.Get("Content-Type"),
		Headers:     sanitizedHeaders(r),
		SourceIP:    clientIP(r),
	}, sigHeader)
	if err != nil {
		// Signature tidak sah = 401. Penting: jangan membocorkan apakah
		// provider atau rahasianya yang salah.
		if shared.Is(err, shared.ErrForbidden) {
			response.Fail(w, err)
			return
		}
		response.Fail(w, err)
		return
	}

	// Event duplikat tetap 200: pengirim mengulang, dan kita sudah menerimanya.
	// Memberi 5xx akan membuat kurir mengulang selamanya.
	status := http.StatusAccepted
	if res.Duplicate {
		status = http.StatusOK
	}
	response.JSON(w, status, response.Envelope{
		Success: true,
		Data: map[string]any{
			"received":  true,
			"duplicate": res.Duplicate,
			"event_id":  res.Event.ID,
		},
	})
}

// List menangani GET /api/v1/webhooks/events — penelusuran operator.
func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	// Daftar event webhook memuat payload pihak ketiga; hanya staf ber-izin
	// (middleware RBAC di router) yang boleh melihat.
	if middleware.IdentityFrom(r.Context()) == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	provider := strings.TrimSpace(q.Get("provider"))
	status := webhook.Status(strings.TrimSpace(q.Get("status")))
	limit := atoiDefault(q.Get("limit"), 50)
	offset := atoiDefault(q.Get("offset"), 0)

	items, total, err := h.svc.List(r.Context(), provider, status, limit, offset)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// extractSignatureHeader mencari header signature yang umum dipakai provider.
//
// Hanya HEADER, tidak pernah query string: nilai di query mudah tercatat di
// log akses, sehingga tidak layak dipakai untuk membawa signature.
func extractSignatureHeader(r *http.Request) string {
	candidates := []string{
		"X-Signature", "X-Hub-Signature-256", "X-Hub-Signature",
		"X-Callback-Signature", "X-Webhook-Signature", "Signature",
		"X-Rajaongkir-Signature", "X-Courier-Signature",
	}
	for _, name := range candidates {
		if v := strings.TrimSpace(r.Header.Get(name)); v != "" {
			return v
		}
	}
	return ""
}

// sensitiveHeaders adalah header yang TIDAK boleh disimpan apa adanya.
// Otorisasi tidak pernah disimpan; signature disimpan terpisah di kolomnya.
var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"proxy-authorization": true,
}

// sanitizedHeaders menyalin header untuk penyimpanan, menyaring yang sensitif.
func sanitizedHeaders(r *http.Request) map[string]any {
	out := make(map[string]any, len(r.Header))
	for name, values := range r.Header {
		if sensitiveHeaders[strings.ToLower(name)] {
			out[name] = "[disunting]"
			continue
		}
		out[name] = strings.Join(values, ", ")
	}
	return out
}
