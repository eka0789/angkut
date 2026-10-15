package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/domain/store"
	"github.com/angkut/angkut/apps/internal/platform"
	"github.com/go-chi/chi/v5"
)

// StoreRepoAPI adalah antarmuka minimal repositori store untuk handler.
type StoreRepoAPI interface {
	Create(ctx context.Context, s *store.Store) error
	GetByID(ctx context.Context, orgID, id string) (*store.Store, error)
	List(ctx context.Context, orgID string, onlyActive bool) ([]store.Store, error)
	Update(ctx context.Context, s *store.Store) error
}

// WarehouseRepoAPI adalah antarmuka minimal repositori gudang untuk handler.
type WarehouseRepoAPI interface {
	Create(ctx context.Context, w *store.Warehouse) error
	GetByID(ctx context.Context, orgID, id string) (*store.Warehouse, error)
	List(ctx context.Context, orgID string, onlyActive bool) ([]store.Warehouse, error)
	Update(ctx context.Context, w *store.Warehouse) error
}

// StoreHandler menangani endpoint kanal penjualan dan gudang tenant-scoped.
type StoreHandler struct {
	stores     StoreRepoAPI
	warehouses WarehouseRepoAPI
}

// NewStoreHandler membuat handler store/warehouse.
func NewStoreHandler(stores StoreRepoAPI, warehouses WarehouseRepoAPI) *StoreHandler {
	return &StoreHandler{stores: stores, warehouses: warehouses}
}

// ─── Stores ──────────────────────────────────────────────────────────────

// ListStores menangani GET /api/v1/stores?is_active=.
func (h *StoreHandler) ListStores(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	onlyActive := r.URL.Query().Get("is_active") != "false"
	items, err := h.stores.List(r.Context(), identity.OrganizationID, onlyActive)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, items)
}

// GetStore menangani GET /api/v1/stores/{id}.
func (h *StoreHandler) GetStore(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	s, err := h.stores.GetByID(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, s)
}

// CreateStore menangani POST /api/v1/stores (admin+).
func (h *StoreHandler) CreateStore(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[storeCreateRequest](w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	s := &store.Store{
		ID:             platform.NewUUID(),
		OrganizationID: identity.OrganizationID,
		Name:           strings.TrimSpace(req.Name),
		Channel:        store.Channel(strings.TrimSpace(req.Channel)),
		AdapterType:    strings.TrimSpace(req.AdapterType),
		WarehouseID:    strings.TrimSpace(req.WarehouseID),
		IsActive:       true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if s.Channel == "" {
		s.Channel = store.ChannelManual
	}
	if s.AdapterType == "" {
		s.AdapterType = "mock"
	}
	if err := s.Validate(); err != nil {
		response.Fail(w, err)
		return
	}
	if err := h.stores.Create(r.Context(), s); err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, s)
}

// UpdateStore menangani PATCH /api/v1/stores/{id} (admin+).
func (h *StoreHandler) UpdateStore(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	s, err := h.stores.GetByID(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	req, ok := httpx.Decode[storeUpdateRequest](w, r)
	if !ok {
		return
	}
	if req.Name != nil {
		s.Name = strings.TrimSpace(*req.Name)
	}
	if req.Channel != nil {
		s.Channel = store.Channel(strings.TrimSpace(*req.Channel))
	}
	if req.AdapterType != nil {
		s.AdapterType = strings.TrimSpace(*req.AdapterType)
	}
	if req.WarehouseID != nil {
		s.WarehouseID = strings.TrimSpace(*req.WarehouseID)
	}
	if req.IsActive != nil {
		s.IsActive = *req.IsActive
	}
	if err := s.Validate(); err != nil {
		response.Fail(w, err)
		return
	}
	if err := h.stores.Update(r.Context(), s); err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, s)
}

// ─── Warehouses ──────────────────────────────────────────────────────────

// ListWarehouses menangani GET /api/v1/warehouses?is_active=.
func (h *StoreHandler) ListWarehouses(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	onlyActive := r.URL.Query().Get("is_active") != "false"
	items, err := h.warehouses.List(r.Context(), identity.OrganizationID, onlyActive)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, items)
}

// GetWarehouse menangani GET /api/v1/warehouses/{id}.
func (h *StoreHandler) GetWarehouse(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	wh, err := h.warehouses.GetByID(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, wh)
}

// CreateWarehouse menangani POST /api/v1/warehouses (admin+).
func (h *StoreHandler) CreateWarehouse(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[warehouseCreateRequest](w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	wh := &store.Warehouse{
		ID:             platform.NewUUID(),
		OrganizationID: identity.OrganizationID,
		Name:           strings.TrimSpace(req.Name),
		AddressID:      strings.TrimSpace(req.AddressID),
		Capacity:       req.Capacity,
		IsDefault:      req.IsDefault,
		IsActive:       true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := wh.Validate(); err != nil {
		response.Fail(w, err)
		return
	}
	if err := h.warehouses.Create(r.Context(), wh); err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, wh)
}

// UpdateWarehouse menangani PATCH /api/v1/warehouses/{id} (admin+).
func (h *StoreHandler) UpdateWarehouse(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	wh, err := h.warehouses.GetByID(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	req, ok := httpx.Decode[warehouseUpdateRequest](w, r)
	if !ok {
		return
	}
	if req.Name != nil {
		wh.Name = strings.TrimSpace(*req.Name)
	}
	if req.AddressID != nil {
		wh.AddressID = strings.TrimSpace(*req.AddressID)
	}
	if req.Capacity != nil {
		wh.Capacity = req.Capacity
	}
	if req.IsDefault != nil {
		wh.IsDefault = *req.IsDefault
	}
	if req.IsActive != nil {
		wh.IsActive = *req.IsActive
	}
	if err := wh.Validate(); err != nil {
		response.Fail(w, err)
		return
	}
	if err := h.warehouses.Update(r.Context(), wh); err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, wh)
}

// ─── Request DTOs ─────────────────────────────────────────────────────────

type storeCreateRequest struct {
	Name        string `json:"name"`
	Channel     string `json:"channel"`
	AdapterType string `json:"adapter_type"`
	WarehouseID string `json:"warehouse_id"`
}

type storeUpdateRequest struct {
	Name        *string `json:"name"`
	Channel     *string `json:"channel"`
	AdapterType *string `json:"adapter_type"`
	WarehouseID *string `json:"warehouse_id"`
	IsActive    *bool   `json:"is_active"`
}

type warehouseCreateRequest struct {
	Name      string `json:"name"`
	AddressID string `json:"address_id"`
	Capacity  *int   `json:"capacity"`
	IsDefault bool   `json:"is_default"`
}

type warehouseUpdateRequest struct {
	Name      *string `json:"name"`
	AddressID *string `json:"address_id"`
	Capacity  *int    `json:"capacity"`
	IsDefault *bool   `json:"is_default"`
	IsActive  *bool   `json:"is_active"`
}
