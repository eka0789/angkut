package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/application/inventoryapp"
	"github.com/angkut/angkut/apps/internal/domain/inventory"
)

// InventoryService adalah antarmuka minimal service inventory untuk handler.
type InventoryService interface {
	CreateProduct(ctx context.Context, orgID, requestID string, in inventoryapp.ProductInput) (*inventory.Product, error)
	GetProduct(ctx context.Context, orgID, id string) (*inventory.Product, error)
	ListProducts(ctx context.Context, orgID string, f inventoryapp.ListFilter) ([]inventory.Product, int, error)
	UpdateProduct(ctx context.Context, orgID, id, requestID string, in inventoryapp.ProductInput) (*inventory.Product, error)
	ArchiveProduct(ctx context.Context, orgID, id, actor, requestID string) error
	AddVariant(ctx context.Context, orgID, productID, requestID string, in inventoryapp.VariantInput) (*inventory.Variant, error)
	UpdateVariant(ctx context.Context, orgID, variantID, requestID string, in inventoryapp.VariantInput) (*inventory.Variant, error)
	Adjust(ctx context.Context, orgID, requestID string, in inventoryapp.AdjustInput) (*inventory.Stock, error)
	ListStock(ctx context.Context, orgID string, f inventoryapp.StockFilter) ([]inventory.StockRow, int, error)
	Movements(ctx context.Context, orgID string, f inventoryapp.MovementFilter) ([]inventory.Movement, int, error)
	LowStock(ctx context.Context, orgID, warehouseID string) ([]inventory.LowStockRow, error)
	Transfer(ctx context.Context, orgID, requestID string, in inventoryapp.TransferInput) (*inventory.StockTransfer, error)
}

// InventoryHandler menangani endpoint produk, inventory & transfer.
type InventoryHandler struct {
	svc InventoryService
}

// NewInventoryHandler membuat handler inventory.
func NewInventoryHandler(svc InventoryService) *InventoryHandler {
	return &InventoryHandler{svc: svc}
}

// RegisterRoutes mendaftarkan rute di /api/v1/products dan /api/v1/inventory.
func (h *InventoryHandler) RegisterProducts(r chi.Router) {
	r.Get("/", h.ListProducts)
	r.Post("/", h.CreateProduct)
	r.Route("/{id}", func(one chi.Router) {
		one.Get("/", h.GetProduct)
		one.Patch("/", h.UpdateProduct)
		one.Delete("/", h.ArchiveProduct)
		one.Post("/variants", h.AddVariant)
		one.Patch("/variants/{vid}", h.UpdateVariant)
	})
}

// RegisterInventory mendaftarkan rute di /api/v1/inventory.
func (h *InventoryHandler) RegisterInventory(r chi.Router) {
	r.Post("/adjust", h.Adjust)
	r.Get("/", h.ListStock)
	r.Get("/movements", h.Movements)
	r.Get("/low-stock", h.LowStock)
}

// RegisterWarehouseStock mendaftarkan rute stok & transfer di /warehouses.
// Router yang nil dilewati — pemanggil hanya memasang grup yang relevan.
func (h *InventoryHandler) RegisterWarehouseStock(stock chi.Router, transfer chi.Router) {
	if stock != nil {
		stock.Get("/stock", h.WarehouseStock)
	}
	if transfer != nil {
		transfer.Post("/transfers", h.CreateTransfer)
	}
}

// ── Produk ──────────────────────────────────────────────────────────────

type productRequest struct {
	Name     string   `json:"name"`
	Category string   `json:"category"`
	WeightKG *float64 `json:"weight_kg"`
	LengthCM *float64 `json:"length_cm"`
	WidthCM  *float64 `json:"width_cm"`
	HeightCM *float64 `json:"height_cm"`
}

func (h *InventoryHandler) ListProducts(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	items, total, err := h.svc.ListProducts(r.Context(), identity.OrganizationID, inventoryapp.ListFilter{
		Search:   strings.TrimSpace(q.Get("search")),
		Category: strings.TrimSpace(q.Get("category")),
		LowStock: q.Get("low_stock") == "true" || q.Get("low_stock") == "1",
		Limit:    atoiDefault(q.Get("limit"), 20),
		Offset:   atoiDefault(q.Get("offset"), 0),
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{
		"total": total, "limit": atoiDefault(q.Get("limit"), 20), "offset": atoiDefault(q.Get("offset"), 0),
	})
}

func (h *InventoryHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[productRequest](w, r)
	if !ok {
		return
	}
	p, err := h.svc.CreateProduct(r.Context(), identity.OrganizationID, requestID(r), inventoryapp.ProductInput{
		Name: req.Name, Category: req.Category,
		WeightKG: req.WeightKG, LengthCM: req.LengthCM, WidthCM: req.WidthCM, HeightCM: req.HeightCM,
		Actor: identity.UserID,
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, p)
}

func (h *InventoryHandler) GetProduct(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	p, err := h.svc.GetProduct(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, p)
}

func (h *InventoryHandler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[productRequest](w, r)
	if !ok {
		return
	}
	p, err := h.svc.UpdateProduct(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"), requestID(r), inventoryapp.ProductInput{
		Name: req.Name, Category: req.Category,
		WeightKG: req.WeightKG, LengthCM: req.LengthCM, WidthCM: req.WidthCM, HeightCM: req.HeightCM,
		Actor: identity.UserID,
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, p)
}

func (h *InventoryHandler) ArchiveProduct(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	if err := h.svc.ArchiveProduct(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"), identity.UserID, requestID(r)); err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{"archived": true})
}

type variantRequest struct {
	SKU               string   `json:"sku"`
	Name              string   `json:"name"`
	WeightKG          *float64 `json:"weight_kg"`
	LengthCM          *float64 `json:"length_cm"`
	WidthCM           *float64 `json:"width_cm"`
	HeightCM          *float64 `json:"height_cm"`
	LowStockThreshold *int     `json:"low_stock_threshold"`
}

func toVariantInput(req variantRequest) inventoryapp.VariantInput {
	in := inventoryapp.VariantInput{
		SKU: req.SKU, Name: req.Name,
		WeightKG: req.WeightKG, LengthCM: req.LengthCM, WidthCM: req.WidthCM, HeightCM: req.HeightCM,
	}
	if req.LowStockThreshold != nil {
		in.LowStockThreshold = *req.LowStockThreshold
	}
	return in
}

func (h *InventoryHandler) AddVariant(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[variantRequest](w, r)
	if !ok {
		return
	}
	v, err := h.svc.AddVariant(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"), requestID(r), toVariantInput(req))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, v)
}

func (h *InventoryHandler) UpdateVariant(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[variantRequest](w, r)
	if !ok {
		return
	}
	v, err := h.svc.UpdateVariant(r.Context(), identity.OrganizationID, chi.URLParam(r, "vid"), requestID(r), toVariantInput(req))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, v)
}

// ── Saldo & movement ────────────────────────────────────────────────────

type adjustRequest struct {
	VariantID   string `json:"variant_id"`
	WarehouseID string `json:"warehouse_id"`
	QtyDelta    *int   `json:"qty_delta"`
	Reason      string `json:"reason"`
}

// Adjust menangani POST /api/v1/inventory/adjust.
func (h *InventoryHandler) Adjust(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[adjustRequest](w, r)
	if !ok {
		return
	}
	if req.QtyDelta == nil {
		response.Fail(w, ValidationError(map[string]string{"qty_delta": "Nilai penyesuaian wajib diisi."}))
		return
	}
	st, err := h.svc.Adjust(r.Context(), identity.OrganizationID, requestID(r), inventoryapp.AdjustInput{
		VariantID: req.VariantID, WarehouseID: req.WarehouseID,
		QtyDelta: *req.QtyDelta, Reason: req.Reason, Actor: identity.UserID,
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, st)
}

// ListStock menangani GET /api/v1/inventory.
func (h *InventoryHandler) ListStock(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	items, total, err := h.svc.ListStock(r.Context(), identity.OrganizationID, inventoryapp.StockFilter{
		WarehouseID: strings.TrimSpace(q.Get("warehouse_id")),
		VariantID:   strings.TrimSpace(q.Get("variant_id")),
		Search:      strings.TrimSpace(q.Get("search")),
		Limit:       atoiDefault(q.Get("limit"), 50),
		Offset:      atoiDefault(q.Get("offset"), 0),
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{
		"total": total, "limit": atoiDefault(q.Get("limit"), 50), "offset": atoiDefault(q.Get("offset"), 0),
	})
}

// Movements menangani GET /api/v1/inventory/movements.
func (h *InventoryHandler) Movements(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	items, total, err := h.svc.Movements(r.Context(), identity.OrganizationID, inventoryapp.MovementFilter{
		VariantID:   strings.TrimSpace(q.Get("variant_id")),
		WarehouseID: strings.TrimSpace(q.Get("warehouse_id")),
		Type:        strings.TrimSpace(q.Get("type")),
		Limit:       atoiDefault(q.Get("limit"), 50),
		Offset:      atoiDefault(q.Get("offset"), 0),
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{
		"total": total, "limit": atoiDefault(q.Get("limit"), 50), "offset": atoiDefault(q.Get("offset"), 0),
	})
}

// LowStock menangani GET /api/v1/inventory/low-stock.
func (h *InventoryHandler) LowStock(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	items, err := h.svc.LowStock(r.Context(), identity.OrganizationID, strings.TrimSpace(r.URL.Query().Get("warehouse_id")))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, items)
}

// ── Warehouse stock & transfer ──────────────────────────────────────────

// WarehouseStock menangani GET /api/v1/warehouses/{id}/stock?search=.
func (h *InventoryHandler) WarehouseStock(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	items, total, err := h.svc.ListStock(r.Context(), identity.OrganizationID, inventoryapp.StockFilter{
		WarehouseID: chi.URLParam(r, "id"),
		Search:      strings.TrimSpace(q.Get("search")),
		Limit:       atoiDefault(q.Get("limit"), 50),
		Offset:      atoiDefault(q.Get("offset"), 0),
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{
		"total": total, "limit": atoiDefault(q.Get("limit"), 50), "offset": atoiDefault(q.Get("offset"), 0),
	})
}

type transferRequest struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Notes string `json:"notes"`
	Items []struct {
		VariantID string `json:"variant_id"`
		Qty       int    `json:"qty"`
	} `json:"items"`
}

// CreateTransfer menangani POST /api/v1/warehouses/transfers.
func (h *InventoryHandler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[transferRequest](w, r)
	if !ok {
		return
	}
	items := make([]inventory.TransferItem, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, inventory.TransferItem{VariantID: it.VariantID, Qty: it.Qty})
	}
	t, err := h.svc.Transfer(r.Context(), identity.OrganizationID, requestID(r), inventoryapp.TransferInput{
		FromWarehouse: req.From, ToWarehouse: req.To, Notes: req.Notes,
		Items: items, Actor: identity.UserID,
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, t)
}
