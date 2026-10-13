package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/angkut/angkut/apps/api/internal/application/rates"
	"github.com/angkut/angkut/apps/api/internal/domain/courier"
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
// Query: origin_city_id, destination_city_id, weight (kg), is_cod.
// cod_amount/declared_value dipakai pricing engine (TASK-023) dan label/score
// ditambahkan rate comparison (TASK-024); di sini harga dasar dulu.
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
	query := courier.RateQuery{
		OriginCityID:      strings.TrimSpace(q.Get("origin_city_id")),
		DestinationCityID: strings.TrimSpace(q.Get("destination_city_id")),
		WeightKG:          weight,
		IsCOD:             q.Get("is_cod") == "true" || q.Get("is_cod") == "1",
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

// ratesQuery dipakai POST /couriers/rates/preview pada TASK-024 (body sama
// dengan GET); didefinisikan sekarang agar bentuk kontrak terjaga.
type ratesQuery struct {
	OriginCityID      string  `json:"origin_city_id"`
	DestinationCityID string  `json:"destination_city_id"`
	Weight            float64 `json:"weight"`
	IsCOD             bool    `json:"is_cod"`
}
