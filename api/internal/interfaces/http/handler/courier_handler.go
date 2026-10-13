package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/angkut/angkut/apps/api/internal/application/rates"
	"github.com/angkut/angkut/apps/api/internal/domain/courier"
	"github.com/angkut/angkut/apps/api/internal/domain/shared"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/go-chi/chi/v5"
)

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
	r.Get("/rates", h.Rates)
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
// Label/skor/ketjut rekomendasi ditambahkan rate comparison (TASK-024/025).
func (h *CourierHandler) Rates(w http.ResponseWriter, r *http.Request) {
	if middleware.IdentityFrom(r.Context()) == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()

	weight, err := strconv.ParseFloat(strings.TrimSpace(q.Get("weight")), 64)
	if err != nil {
		response.Fail(w, ValidationError(map[string]string{
			"weight": "Berat harus angka (kg).",
		}))
		return
	}

	fields := make(map[string]string)
	dims, dimsErr := parseDims(q.Get("length"), q.Get("width"), q.Get("height"))
	if dimsErr != nil {
		fields["dimensions"] = dimsErr.Error()
	}
	isCOD := q.Get("is_cod") == "true" || q.Get("is_cod") == "1"
	insured := q.Get("insurance") == "true" || q.Get("insurance") == "1"

	codAmount := int64(0)
	if isCOD {
		codAmount, err = strconv.ParseInt(strings.TrimSpace(q.Get("cod_amount")), 10, 64)
		if err != nil || codAmount <= 0 {
			fields["cod_amount"] = "Nilai COD wajib diisi untuk pengiriman COD."
		}
	}
	declaredValue := int64(0)
	if insured {
		declaredValue, err = strconv.ParseInt(strings.TrimSpace(q.Get("declared_value")), 10, 64)
		if err != nil || declaredValue <= 0 {
			fields["declared_value"] = "Nilai barang wajib diisi untuk asuransi."
		}
	}
	if len(fields) > 0 {
		response.Fail(w, ValidationError(fields))
		return
	}

	query := courier.RateQuery{
		OriginCityID:      strings.TrimSpace(q.Get("origin_city_id")),
		DestinationCityID: strings.TrimSpace(q.Get("destination_city_id")),
		ActualWeightKG:    weight,
		Dimensions:        dims,
		IsCOD:             isCOD,
		CODAmount:         codAmount,
		Insured:           insured,
		DeclaredValue:     declaredValue,
	}
	options, originZone, destZone, err := h.svc.Quote(r.Context(), query)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{
		"origin_zone":      originZone,
		"destination_zone": destZone,
		"options":          options,
	})
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
