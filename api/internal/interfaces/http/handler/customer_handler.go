package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/domain/customer"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// CustomerHandler menangani CRUD pelanggan tenant-scoped.
type CustomerHandler struct {
	repo CustomerRepo
}

// CustomerRepo adalah antarmuka minimal untuk operasi pelanggan.
type CustomerRepo interface {
	Create(ctx context.Context, c *customer.Customer) error
	GetByID(ctx context.Context, orgID, id string) (*customer.Customer, error)
	GetByPhone(ctx context.Context, orgID, phone string) (*customer.Customer, error)
	Update(ctx context.Context, c *customer.Customer) error
	Delete(ctx context.Context, orgID, id string) error
	List(ctx context.Context, orgID string, segment customer.Segment, query string, limit, offset int) ([]customer.Customer, int, error)
}

// NewCustomerHandler membuat handler pelanggan.
func NewCustomerHandler(repo CustomerRepo) *CustomerHandler {
	return &CustomerHandler{repo: repo}
}

// RegisterRoutes mendaftarkan rute di /api/v1/customers.
// Auth + tenant sudah dipasang di router level; otorisasi granular di sini.
func (h *CustomerHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Route("/{id}", func(one chi.Router) {
		one.Get("/", h.Get)
		one.Patch("/", h.Update)
		one.Delete("/", h.Delete)
		one.Get("/orders", h.ListOrders)
		one.Get("/stats", h.Stats)
		one.Post("/segment", h.SetSegment)
	})
}

// List menangani GET /api/v1/customers?search=&segment=&limit=&offset=.
func (h *CustomerHandler) List(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	segment := customer.Segment(strings.TrimSpace(q.Get("segment")))
	search := strings.TrimSpace(q.Get("search"))
	limit := atoiDefault(q.Get("limit"), 20)
	offset := atoiDefault(q.Get("offset"), 0)

	items, total, err := h.repo.List(r.Context(), identity.OrganizationID, segment, search, limit, offset)
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

// Create menangani POST /api/v1/customers.
func (h *CustomerHandler) Create(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[customerCreateRequest](w, r)
	if !ok {
		return
	}
	c := &customer.Customer{
		ID:             uuid.NewString(),
		OrganizationID: identity.OrganizationID,
		Name:           strings.TrimSpace(req.Name),
		Phone:          strings.TrimSpace(req.Phone),
		Email:          strings.TrimSpace(req.Email),
		Notes:          strings.TrimSpace(req.Notes),
		Segment:        customer.SegmentNew,
		SegmentSource:  "auto",
		Metrics:        customer.Metrics{},
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := c.Validate(); err != nil {
		response.Fail(w, err)
		return
	}
	// Nomor telepon adalah kunci pencarian utama; duplikat ditolak jelas.
	if existing, err := h.repo.GetByPhone(r.Context(), identity.OrganizationID, c.Phone); err == nil {
		_ = existing
		response.Fail(w, customer.ErrPhoneTaken)
		return
	}
	if err := h.repo.Create(r.Context(), c); err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, c)
}

// Get menangani GET /api/v1/customers/{id}.
func (h *CustomerHandler) Get(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	c, err := h.repo.GetByID(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, c)
}

// Update menangani PATCH /api/v1/customers/{id}.
func (h *CustomerHandler) Update(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	existing, err := h.repo.GetByID(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	req, ok := httpx.Decode[customerUpdateRequest](w, r)
	if !ok {
		return
	}
	if req.Name != nil {
		existing.Name = strings.TrimSpace(*req.Name)
	}
	if req.Phone != nil {
		existing.Phone = strings.TrimSpace(*req.Phone)
	}
	if req.Email != nil {
		existing.Email = strings.TrimSpace(*req.Email)
	}
	if req.Notes != nil {
		existing.Notes = strings.TrimSpace(*req.Notes)
	}
	if req.DefaultAddressID != nil {
		existing.DefaultAddressID = strings.TrimSpace(*req.DefaultAddressID)
	}
	existing.UpdatedAt = time.Now()

	if err := existing.Validate(); err != nil {
		response.Fail(w, err)
		return
	}
	if err := h.repo.Update(r.Context(), existing); err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, existing)
}

// Delete menangani DELETE /api/v1/customers/{id} — soft delete (admin+).
func (h *CustomerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	if err := h.repo.Delete(r.Context(), identity.OrganizationID, chi.URLParam(r, "id")); err != nil {
		response.Fail(w, err)
		return
	}
	response.NoContent(w)
}

// ListOrders menangani GET /api/v1/customers/{id}/orders.
//
// Modul Order (TASK-026) belum ada; endpoint dikembalikan kosong agar kontrak
// API stabil dan FE bisa dipasang lebih awal.
func (h *CustomerHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	if _, err := h.repo.GetByID(r.Context(), identity.OrganizationID, chi.URLParam(r, "id")); err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, []any{}, map[string]any{"total": 0, "note": "orders belum tersedia"})
}

// Stats menangani GET /api/v1/customers/{id}/stats — metrik dari jsonb.
func (h *CustomerHandler) Stats(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	c, err := h.repo.GetByID(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, customer.FromMetrics(c.Metrics))
}

// SetSegment menangani POST /api/v1/customers/{id}/segment.
//
// Body: { "segment": "vip" } untuk override manual, atau { "segment": "" }
// untuk menghapus override dan menghitung otomatis kembali.
func (h *CustomerHandler) SetSegment(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	c, err := h.repo.GetByID(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	req, ok := httpx.Decode[customerSegmentRequest](w, r)
	if !ok {
		return
	}

	if seg := strings.TrimSpace(req.Segment); seg != "" {
		s := customer.Segment(seg)
		if !s.Valid() {
			response.Fail(w, ValidationError(map[string]string{
				"segment": "Segmen tidak dikenal. Pilihan: new, regular, vip, high_risk, cod_risk.",
			}))
			return
		}
		c.Segment = s
		c.SegmentSource = "manual"
	} else {
		// Hapus override: kembali ke perhitungan otomatis.
		c.Segment = customer.AutoSegment(c.Metrics)
		c.SegmentSource = "auto"
	}

	if err := h.repo.Update(r.Context(), c); err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, c)
}

// ─── Request DTOs ─────────────────────────────────────────────────────────────

type customerCreateRequest struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email"`
	Notes string `json:"notes"`
}

type customerUpdateRequest struct {
	Name             *string `json:"name"`
	Phone            *string `json:"phone"`
	Email            *string `json:"email"`
	Notes            *string `json:"notes"`
	DefaultAddressID *string `json:"default_address_id"`
}

type customerSegmentRequest struct {
	Segment string `json:"segment"`
}

// atoiDefault mengurai integer query param dengan fallback.
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
		if n > 1_000_000 {
			return def
		}
	}
	return n
}
