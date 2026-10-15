package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/application/rates"
	"github.com/angkut/angkut/apps/internal/domain/courier"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/go-chi/chi/v5"
)

// maxBodyBytes membatasi ukuran body JSON permintaan tarif.
const maxBodyBytes = 64 << 10

// CourierHandler menangani endpoint registry kurir dan pencarian tarif.
type CourierHandler struct {
	svc *rates.Service
}

// NewCourierHandler membuat handler kurir.
func NewCourierHandler(svc *rates.Service) *CourierHandler {
	return &CourierHandler{svc: svc}
}

// RegisterRoutes mendaftarkan rute di /api/v1/couriers.
// Auth + tenant dipasang di router level; semua rute read-only
// (shipment.view) — konfigurasi kurir lewat Admin Panel (Phase 7).
func (h *CourierHandler) RegisterRoutes(r chi.Router) {
	// Rute statis didahulukan agar tidak tertelan /{code}.
	r.Post("/rates/preview", h.RatesPreview)
	r.Get("/rates", h.Rates)
	r.Post("/recommend", h.Recommend)
	r.Get("/", h.List)
	r.Get("/{code}/services", h.Services)
}

// List menangani GET /api/v1/couriers — daftar kurir aktif.
func (h *CourierHandler) List(w http.ResponseWriter, r *http.Request) {
	if middleware.IdentityFrom(r.Context()) == nil {
		response.Fail(w, unauthorized())
		return
	}
	couriers, err := h.svc.ListActive(r.Context())
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, couriers)
}

// Services menangani GET /api/v1/couriers/{code}/services.
func (h *CourierHandler) Services(w http.ResponseWriter, r *http.Request) {
	if middleware.IdentityFrom(r.Context()) == nil {
		response.Fail(w, unauthorized())
		return
	}
	c, services, err := h.svc.ServicesFor(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{
		"courier":  c,
		"services": services,
	})
}

// Rates menangani GET /api/v1/couriers/rates.
//
// Query: origin_city_id, destination_city_id, weight (kg), length/width/height
// (cm, opsional), is_cod, cod_amount, insurance, declared_value.
func (h *CourierHandler) Rates(w http.ResponseWriter, r *http.Request) {
	if middleware.IdentityFrom(r.Context()) == nil {
		response.Fail(w, unauthorized())
		return
	}
	query, err := parseRateQuery(r.URL.Query(), nil)
	if err != nil {
		response.Fail(w, err)
		return
	}
	res, err := h.svc.Quote(r.Context(), query)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, res)
}

// RatesPreview menangani POST /api/v1/couriers/rates/preview.
//
// Body JSON memakai field yang sama dengan query GET; parameter query juga
// diterima agar kontrak "GET → POST" tetap sederhana untuk paket besar.
func (h *CourierHandler) RatesPreview(w http.ResponseWriter, r *http.Request) {
	if middleware.IdentityFrom(r.Context()) == nil {
		response.Fail(w, unauthorized())
		return
	}
	body, err := decodeRateBody(r)
	if err != nil {
		response.Fail(w, err)
		return
	}
	query, err := parseRateQuery(r.URL.Query(), body)
	if err != nil {
		response.Fail(w, err)
		return
	}
	res, err := h.svc.Quote(r.Context(), query)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, res)
}

// Recommend menangani POST /api/v1/couriers/recommend.
//
// Body: { "quote_id": "q1..." } — atau parameter yang sama dengan /rates bila
// klien tidak menyimpan quote_id.
func (h *CourierHandler) Recommend(w http.ResponseWriter, r *http.Request) {
	if middleware.IdentityFrom(r.Context()) == nil {
		response.Fail(w, unauthorized())
		return
	}
	body, err := decodeRateBody(r)
	if err != nil {
		response.Fail(w, err)
		return
	}

	if quoteID := stringField(body, "quote_id"); quoteID != "" {
		res, best, err := h.svc.Recommend(r.Context(), quoteID)
		if err != nil {
			response.Fail(w, err)
			return
		}
		response.OK(w, recommendPayload(res, best))
		return
	}

	query, err := parseRateQuery(r.URL.Query(), body)
	if err != nil {
		response.Fail(w, err)
		return
	}
	res, err := h.svc.Quote(r.Context(), query)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, recommendPayload(res, findRecommended(res.Options)))
}

// recommendPayload menyusun ringkasan rekomendasi tanpa menduplikasi seluruh
// daftar opsi — klien yang butuh semuanya memanggil /rates.
func recommendPayload(res *rates.QuoteResult, best *rates.Option) map[string]any {
	if res == nil {
		return map[string]any{"quote_id": "", "recommended": nil}
	}
	return map[string]any{
		"quote_id":          res.QuoteID,
		"chargeable_weight": res.ChargeableWeight,
		"summary_labels":    res.SummaryLabels,
		"recommended":       best,
	}
}

// findRecommended mengembalikan opsi yang ditandai rekomendasi, atau nil.
func findRecommended(options []rates.Option) *rates.Option {
	for i := range options {
		if options[i].IsRecommended {
			return &options[i]
		}
	}
	return nil
}

// ─── parsing ─────────────────────────────────────────────────────────────

// decodeRateBody membaca body JSON opsional. Body kosong bukan error.
func decodeRateBody(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return nil, nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("membaca body: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, shared.Validation(map[string]string{
			"body": "Body harus berupa JSON objek yang valid.",
		})
	}
	return body, nil
}

// parseRateQuery menyatukan parameter dari query string dan body JSON.
// Body hanya dipakai bila query tidak menyediakan nilainya.
func parseRateQuery(q map[string][]string, body map[string]any) (courier.RateQuery, error) {
	pick := func(key string) string {
		if v := firstNonEmpty(q[key]); v != "" {
			return v
		}
		return bodyField(body, key)
	}

	fields := make(map[string]string)

	weight, err := strconv.ParseFloat(strings.TrimSpace(pick("weight")), 64)
	if err != nil {
		fields["weight"] = "Berat harus angka (kg)."
	}

	dims, dimsErr := parseDims(pick("length"), pick("width"), pick("height"))
	if dimsErr != nil {
		fields["dimensions"] = dimsErr.Error()
	}

	isCOD := isTruthy(pick("is_cod"))
	insured := isTruthy(pick("insurance"))

	codAmount := int64(0)
	if isCOD {
		codAmount, err = parsePositiveInt(pick("cod_amount"))
		if err != nil {
			fields["cod_amount"] = "Nilai COD wajib diisi untuk pengiriman COD."
		}
	}
	declaredValue := int64(0)
	if insured {
		declaredValue, err = parsePositiveInt(pick("declared_value"))
		if err != nil {
			fields["declared_value"] = "Nilai barang wajib diisi untuk asuransi."
		}
	}
	if len(fields) > 0 {
		return courier.RateQuery{}, ValidationError(fields)
	}

	query := courier.RateQuery{
		OriginCityID:      strings.TrimSpace(pick("origin_city_id")),
		DestinationCityID: strings.TrimSpace(pick("destination_city_id")),
		ActualWeightKG:    weight,
		Dimensions:        dims,
		IsCOD:             isCOD,
		CODAmount:         codAmount,
		Insured:           insured,
		DeclaredValue:     declaredValue,
	}
	// Invariant kota & berat ditegakkan di domain — satu sumber kebenaran.
	if err := query.Validate(); err != nil {
		return courier.RateQuery{}, err
	}
	return query, nil
}

// firstNonEmpty mengembalikan nilai pertama yang tidak kosong.
func firstNonEmpty(values []string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// bodyField mengambil field body sebagai string (angka & bool dinormalkan).
func bodyField(body map[string]any, key string) string {
	if body == nil {
		return ""
	}
	switch t := body[key].(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		return ""
	}
}

// stringField mengambil field body bertipe string murni.
func stringField(body map[string]any, key string) string {
	if body == nil {
		return ""
	}
	if s, ok := body[key].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// isTruthy menerima bentuk bool yang lazim dipakai klien.
func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

// parsePositiveInt mem-parse bilangan bulat > 0.
func parsePositiveInt(raw string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("harus bilangan bulat positif")
	}
	return n, nil
}

// parseDims menguraikan dimensi opsional (cm). Ketiganya harus diisi
// bersama-sama atau tidak sama sekali.
func parseDims(length, width, height string) (*shared.Dimensions, error) {
	allEmpty := length == "" && width == "" && height == ""
	allSet := length != "" && width != "" && height != ""
	if allEmpty {
		return nil, nil
	}
	if !allSet {
		return nil, fmt.Errorf("length, width, dan height harus diisi bersama-sama")
	}
	l, err1 := strconv.ParseFloat(strings.TrimSpace(length), 64)
	w, err2 := strconv.ParseFloat(strings.TrimSpace(width), 64)
	hgt, err3 := strconv.ParseFloat(strings.TrimSpace(height), 64)
	if err1 != nil || err2 != nil || err3 != nil || l <= 0 || w <= 0 || hgt <= 0 {
		return nil, fmt.Errorf("dimensi harus angka positif (cm)")
	}
	return &shared.Dimensions{Length: l, Width: w, Height: hgt}, nil
}
