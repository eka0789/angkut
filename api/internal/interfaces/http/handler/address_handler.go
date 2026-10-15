package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/domain/address"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// AddressHandler menangani CRUD alamat tenant-scoped.
type AddressHandler struct {
	addrRepo   AddressRepo
	regionRepo RegionRepo
}

// AddressRepo adalah antarmuka minimal untuk operasi alamat.
type AddressRepo interface {
	Create(ctx context.Context, a *address.Address) error
	GetByID(ctx context.Context, orgID, id string) (*address.Address, error)
	Update(ctx context.Context, a *address.Address) error
	Delete(ctx context.Context, orgID, id string) error
	List(ctx context.Context, orgID string, status address.Status) ([]address.Address, error)
	Search(ctx context.Context, orgID, query string, limit int) ([]address.Address, error)
	UnsetDefaultsExcept(ctx context.Context, orgID, exceptID string) error
}

// NewAddressHandler membuat handler alamat.
func NewAddressHandler(addrRepo AddressRepo, regionRepo RegionRepo) *AddressHandler {
	return &AddressHandler{addrRepo: addrRepo, regionRepo: regionRepo}
}

// RegisterRoutes mendaftarkan rute alamat di /api/v1/addresses.
// Auth + tenant middleware sudah dipasang di router level.
func (h *AddressHandler) RegisterRoutes(r chi.Router) {
	r.Get("/search", h.Search)
	r.Post("/validate", h.Validate)
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Patch("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)
}

// Search menangani GET /api/v1/addresses/search?q=...
func (h *AddressHandler) Search(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		response.OK(w, []address.Address{})
		return
	}
	items, err := h.addrRepo.Search(r.Context(), identity.OrganizationID, q, 20)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, items)
}

// Validate menangani POST /api/v1/addresses/validate.
func (h *AddressHandler) Validate(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[addressValidateRequest](w, r)
	if !ok {
		return
	}
	result := h.validateRegion(r, req)
	response.OK(w, result)
}

// List menangani GET /api/v1/addresses.
func (h *AddressHandler) List(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	status := address.Status(strings.TrimSpace(r.URL.Query().Get("status")))
	items, err := h.addrRepo.List(r.Context(), identity.OrganizationID, status)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, items)
}

// Create menangani POST /api/v1/addresses.
func (h *AddressHandler) Create(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[addressCreateRequest](w, r)
	if !ok {
		return
	}
	a := &address.Address{
		ID:             uuid.NewString(),
		OrganizationID: identity.OrganizationID,
		Recipient:      strings.TrimSpace(req.Recipient),
		Phone:          strings.TrimSpace(req.Phone),
		ProvinceID:     strings.TrimSpace(req.ProvinceID),
		CityID:         strings.TrimSpace(req.CityID),
		DistrictID:     strings.TrimSpace(req.DistrictID),
		VillageID:      strings.TrimSpace(req.VillageID),
		PostalCode:     strings.TrimSpace(req.PostalCode),
		Street:         strings.TrimSpace(req.Street),
		Notes:          strings.TrimSpace(req.Notes),
		Latitude:       req.Latitude,
		Longitude:      req.Longitude,
		Label:          strings.TrimSpace(req.Label),
		IsDefault:      req.IsDefault,
		Status:         address.StatusActive,
		CreatedBy:      identity.UserID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := a.Validate(); err != nil {
		response.Fail(w, err)
		return
	}
	// Validasi FK wilayah sebelum simpan agar error lebih jelas.
	if err := h.validateFK(r, a); err != nil {
		response.Fail(w, err)
		return
	}
	if a.IsDefault {
		if err := h.addrRepo.UnsetDefaultsExcept(r.Context(), identity.OrganizationID, a.ID); err != nil {
			response.Fail(w, err)
			return
		}
	}
	if err := h.addrRepo.Create(r.Context(), a); err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, a)
}

// Get menangani GET /api/v1/addresses/{id}.
func (h *AddressHandler) Get(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	id := chi.URLParam(r, "id")
	a, err := h.addrRepo.GetByID(r.Context(), identity.OrganizationID, id)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, a)
}

// Update menangani PATCH /api/v1/addresses/{id}.
func (h *AddressHandler) Update(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.addrRepo.GetByID(r.Context(), identity.OrganizationID, id)
	if err != nil {
		response.Fail(w, err)
		return
	}
	req, ok := httpx.Decode[addressUpdateRequest](w, r)
	if !ok {
		return
	}
	// Merge field yang dikirim.
	if req.Recipient != nil {
		existing.Recipient = strings.TrimSpace(*req.Recipient)
	}
	if req.Phone != nil {
		existing.Phone = strings.TrimSpace(*req.Phone)
	}
	if req.ProvinceID != nil {
		existing.ProvinceID = strings.TrimSpace(*req.ProvinceID)
	}
	if req.CityID != nil {
		existing.CityID = strings.TrimSpace(*req.CityID)
	}
	if req.DistrictID != nil {
		existing.DistrictID = strings.TrimSpace(*req.DistrictID)
	}
	if req.VillageID != nil {
		existing.VillageID = strings.TrimSpace(*req.VillageID)
	}
	if req.PostalCode != nil {
		existing.PostalCode = strings.TrimSpace(*req.PostalCode)
	}
	if req.Street != nil {
		existing.Street = strings.TrimSpace(*req.Street)
	}
	if req.Notes != nil {
		existing.Notes = strings.TrimSpace(*req.Notes)
	}
	if req.Latitude != nil {
		existing.Latitude = req.Latitude
	}
	if req.Longitude != nil {
		existing.Longitude = req.Longitude
	}
	if req.Label != nil {
		existing.Label = strings.TrimSpace(*req.Label)
	}
	if req.IsDefault != nil {
		existing.IsDefault = *req.IsDefault
	}
	if req.Status != nil {
		existing.Status = address.Status(*req.Status)
	}
	existing.UpdatedAt = time.Now()

	if err := existing.Validate(); err != nil {
		response.Fail(w, err)
		return
	}
	if err := h.validateFK(r, existing); err != nil {
		response.Fail(w, err)
		return
	}
	if existing.IsDefault {
		if err := h.addrRepo.UnsetDefaultsExcept(r.Context(), identity.OrganizationID, existing.ID); err != nil {
			response.Fail(w, err)
			return
		}
	}
	if err := h.addrRepo.Update(r.Context(), existing); err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, existing)
}

// Delete menangani DELETE /api/v1/addresses/{id}.
func (h *AddressHandler) Delete(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.addrRepo.Delete(r.Context(), identity.OrganizationID, id); err != nil {
		response.Fail(w, err)
		return
	}
	response.NoContent(w)
}

// ─── Request DTOs ─────────────────────────────────────────────────────────────

type addressCreateRequest struct {
	Recipient  string   `json:"recipient"`
	Phone      string   `json:"phone"`
	ProvinceID string   `json:"province_id"`
	CityID     string   `json:"city_id"`
	DistrictID string   `json:"district_id"`
	VillageID  string   `json:"village_id"`
	PostalCode string   `json:"postal_code"`
	Street     string   `json:"street"`
	Notes      string   `json:"notes"`
	Latitude   *float64 `json:"latitude"`
	Longitude  *float64 `json:"longitude"`
	Label      string   `json:"label"`
	IsDefault  bool     `json:"is_default"`
}

type addressUpdateRequest struct {
	Recipient  *string  `json:"recipient"`
	Phone      *string  `json:"phone"`
	ProvinceID *string  `json:"province_id"`
	CityID     *string  `json:"city_id"`
	DistrictID *string  `json:"district_id"`
	VillageID  *string  `json:"village_id"`
	PostalCode *string  `json:"postal_code"`
	Street     *string  `json:"street"`
	Notes      *string  `json:"notes"`
	Latitude   *float64 `json:"latitude"`
	Longitude  *float64 `json:"longitude"`
	Label      *string  `json:"label"`
	IsDefault  *bool    `json:"is_default"`
	Status     *string  `json:"status"`
}

type addressValidateRequest struct {
	ProvinceID string `json:"province_id"`
	CityID     string `json:"city_id"`
	DistrictID string `json:"district_id"`
	VillageID  string `json:"village_id"`
	PostalCode string `json:"postal_code"`
	Street     string `json:"street"`
}

// ─── Validation helpers ───────────────────────────────────────────────────────

func (h *AddressHandler) validateFK(r *http.Request, a *address.Address) error {
	ctx := r.Context()
	fields := make(map[string]string)

	if _, err := h.regionRepo.GetProvinceByID(ctx, a.ProvinceID); err != nil {
		fields["province_id"] = "Provinsi tidak ditemukan."
	}
	city, err := h.regionRepo.GetCityByID(ctx, a.CityID)
	if err != nil {
		fields["city_id"] = "Kota tidak ditemukan."
	} else if city.ProvinceID != a.ProvinceID {
		fields["city_id"] = "Kota tidak berada di provinsi yang dipilih."
	}
	dist, err := h.regionRepo.GetDistrictByID(ctx, a.DistrictID)
	if err != nil {
		fields["district_id"] = "Kecamatan tidak ditemukan."
	} else if dist.CityID != a.CityID {
		fields["district_id"] = "Kecamatan tidak berada di kota yang dipilih."
	}
	if a.VillageID != "" {
		v, err := h.regionRepo.GetVillageByID(ctx, a.VillageID)
		if err != nil {
			fields["village_id"] = "Desa/kelurahan tidak ditemukan."
		} else if v.DistrictID != a.DistrictID {
			fields["village_id"] = "Desa tidak berada di kecamatan yang dipilih."
		}
	}
	if len(fields) > 0 {
		return ValidationError(fields)
	}
	return nil
}

func (h *AddressHandler) validateRegion(r *http.Request, req addressValidateRequest) address.ValidationResult {
	ctx := r.Context()
	issues := make([]string, 0)
	suggestions := make([]string, 0)

	if req.ProvinceID == "" {
		issues = append(issues, "province_id kosong")
	}
	if req.CityID == "" {
		issues = append(issues, "city_id kosong")
	}
	if req.DistrictID == "" {
		issues = append(issues, "district_id kosong")
	}
	if req.Street == "" {
		issues = append(issues, "street kosong")
	}

	if req.ProvinceID != "" {
		if _, err := h.regionRepo.GetProvinceByID(ctx, req.ProvinceID); err != nil {
			issues = append(issues, "provinsi tidak ditemukan")
		}
	}
	if req.CityID != "" {
		city, err := h.regionRepo.GetCityByID(ctx, req.CityID)
		if err != nil {
			issues = append(issues, "kota tidak ditemukan")
		} else if req.ProvinceID != "" && city.ProvinceID != req.ProvinceID {
			issues = append(issues, "kota tidak sesuai provinsi")
		}
	}
	if req.DistrictID != "" {
		dist, err := h.regionRepo.GetDistrictByID(ctx, req.DistrictID)
		if err != nil {
			issues = append(issues, "kecamatan tidak ditemukan")
		} else if req.CityID != "" && dist.CityID != req.CityID {
			issues = append(issues, "kecamatan tidak sesuai kota")
		}
	}
	if req.VillageID != "" {
		v, err := h.regionRepo.GetVillageByID(ctx, req.VillageID)
		if err != nil {
			issues = append(issues, "desa/kelurahan tidak ditemukan")
		} else if req.DistrictID != "" && v.DistrictID != req.DistrictID {
			issues = append(issues, "desa tidak sesuai kecamatan")
		}
	}

	return address.ValidationResult{
		Valid:       len(issues) == 0,
		Issues:      issues,
		Suggestions: suggestions,
	}
}
