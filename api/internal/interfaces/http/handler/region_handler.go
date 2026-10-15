package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/domain/region"
	"github.com/go-chi/chi/v5"
)

// RegionHandler menangani endpoint publik wilayah (provinsi, kota, kecamatan, desa, kode pos).
type RegionHandler struct {
	repo RegionRepo
}

// RegionRepo adalah antarmuka minimal yang dibutuhkan handler.
// Dipisah agar handler tidak bergantung pada implementasi database konkret.
type RegionRepo interface {
	ListProvinces(ctx context.Context) ([]region.Province, error)
	ListCities(ctx context.Context, provinceID string) ([]region.City, error)
	ListDistricts(ctx context.Context, cityID string) ([]region.District, error)
	ListVillages(ctx context.Context, districtID string) ([]region.Village, error)
	ListPostalCodes(ctx context.Context, districtID string) ([]region.PostalCode, error)
	GetProvinceByID(ctx context.Context, id string) (*region.Province, error)
	GetCityByID(ctx context.Context, id string) (*region.City, error)
	GetDistrictByID(ctx context.Context, id string) (*region.District, error)
	GetVillageByID(ctx context.Context, id string) (*region.Village, error)
}

// NewRegionHandler membuat handler wilayah.
func NewRegionHandler(repo RegionRepo) *RegionHandler {
	return &RegionHandler{repo: repo}
}

// RegisterRoutes mendaftarkan rute wilayah di /api/v1/regions.
func (h *RegionHandler) RegisterRoutes(r chi.Router) {
	// Publik — tidak butuh auth. Rate limit per IP di level router.
	r.Get("/provinces", h.ListProvinces)
	r.Get("/cities", h.ListCities)
	r.Get("/districts", h.ListDistricts)
	r.Get("/villages", h.ListVillages)
	r.Get("/postal-codes", h.ListPostalCodes)
}

func (h *RegionHandler) ListProvinces(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	provinces, err := h.repo.ListProvinces(ctx)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, provinces)
}

func (h *RegionHandler) ListCities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	provinceID := strings.TrimSpace(r.URL.Query().Get("province_id"))
	if provinceID == "" {
		response.Fail(w, ValidationError(map[string]string{"province_id": "province_id wajib diisi"}))
		return
	}
	cities, err := h.repo.ListCities(ctx, provinceID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, cities)
}

func (h *RegionHandler) ListDistricts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cityID := strings.TrimSpace(r.URL.Query().Get("city_id"))
	if cityID == "" {
		response.Fail(w, ValidationError(map[string]string{"city_id": "city_id wajib diisi"}))
		return
	}
	districts, err := h.repo.ListDistricts(ctx, cityID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, districts)
}

func (h *RegionHandler) ListVillages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	districtID := strings.TrimSpace(r.URL.Query().Get("district_id"))
	if districtID == "" {
		response.Fail(w, ValidationError(map[string]string{"district_id": "district_id wajib diisi"}))
		return
	}
	villages, err := h.repo.ListVillages(ctx, districtID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, villages)
}

func (h *RegionHandler) ListPostalCodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	districtID := strings.TrimSpace(r.URL.Query().Get("district_id"))
	if districtID == "" {
		response.Fail(w, ValidationError(map[string]string{"district_id": "district_id wajib diisi"}))
		return
	}
	codes, err := h.repo.ListPostalCodes(ctx, districtID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, codes)
}
