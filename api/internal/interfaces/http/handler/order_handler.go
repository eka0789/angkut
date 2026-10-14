package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/angkut/angkut/apps/api/internal/application/orderapp"
	"github.com/angkut/angkut/apps/api/internal/domain/order"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/go-chi/chi/v5"
)

// OrderService adalah antarmuka minimal service order yang dipakai handler.
type OrderService interface {
	Create(ctx context.Context, orgID, actorUserID, requestID string, in orderapp.CreateInput) (*order.Order, error)
	Get(ctx context.Context, orgID, id string) (*order.Order, error)
	List(ctx context.Context, orgID string, f order.ListFilter) ([]order.Order, int, error)
	Transition(ctx context.Context, orgID, actorUserID, requestID, id string, target order.Status, reason string) (*order.Order, error)
	GetSummary(ctx context.Context, orgID string) (orderapp.Summary, error)
}

// OrderHandler menangani endpoint order tenant-scoped.
type OrderHandler struct {
	svc OrderService
}

// NewOrderHandler membuat handler order.
func NewOrderHandler(svc OrderService) *OrderHandler {
	return &OrderHandler{svc: svc}
}

// RegisterRoutes mendaftarkan rute di /api/v1/orders.
// Auth + tenant + RBAC dipasang di router level; rute tulis dibatasi di sana.
func (h *OrderHandler) RegisterRoutes(r chi.Router) {
	// Rute statis didahulukan agar tidak tertelan /{id}.
	r.Get("/summary", h.Summary)
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Route("/{id}", func(one chi.Router) {
		one.Get("/", h.Get)
		one.Patch("/", h.Update)
		one.Post("/cancel", h.Cancel)
	})
}

// List menangani GET /api/v1/orders.
func (h *OrderHandler) List(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	f := order.ListFilter{
		Status:     order.Status(strings.TrimSpace(q.Get("status"))),
		StoreID:    strings.TrimSpace(q.Get("store_id")),
		CustomerID: strings.TrimSpace(q.Get("customer_id")),
		Search:     strings.TrimSpace(q.Get("search")),
		From:       strings.TrimSpace(q.Get("from")),
		To:         strings.TrimSpace(q.Get("to")),
		Limit:      atoiDefault(q.Get("limit"), 20),
		Offset:     atoiDefault(q.Get("offset"), 0),
	}
	if f.Status != "" && !f.Status.Valid() {
		response.Fail(w, ValidationError(map[string]string{
			"status": "Status order tidak dikenal.",
		}))
		return
	}

	items, total, err := h.svc.List(r.Context(), identity.OrganizationID, f)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
	})
}

// Create menangani POST /api/v1/orders.
func (h *OrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[orderCreateRequest](w, r)
	if !ok {
		return
	}
	items := make([]orderapp.CreateItemInput, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, orderapp.CreateItemInput{
			VariantID:   it.VariantID,
			ProductName: it.ProductName,
			VariantName: it.VariantName,
			Qty:         it.Qty,
			UnitPrice:   it.UnitPrice,
		})
	}

	o, err := h.svc.Create(r.Context(), identity.OrganizationID, identity.UserID, requestID(r), orderapp.CreateInput{
		StoreID:           req.StoreID,
		CustomerID:        req.CustomerID,
		ShippingAddressID: req.ShippingAddressID,
		PaymentType:       order.PaymentType(strings.TrimSpace(req.PaymentType)),
		ShippingCost:      req.ShippingCost,
		Discount:          req.Discount,
		Notes:             req.Notes,
		ExternalOrderID:   req.ExternalOrderID,
		Items:             items,
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, o)
}

// Get menangani GET /api/v1/orders/{id}.
func (h *OrderHandler) Get(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	o, err := h.svc.Get(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, o)
}

// Update menangani PATCH /api/v1/orders/{id} — memindahkan status order.
// Body: { "status": "confirmed" } atau { "status": "draft", "reason": "..." }.
// Transisi ilegal ditolak 409 oleh state machine domain.
func (h *OrderHandler) Update(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[orderUpdateRequest](w, r)
	if !ok {
		return
	}
	target := order.Status(strings.TrimSpace(req.Status))
	if !target.Valid() {
		response.Fail(w, ValidationError(map[string]string{
			"status": "Status order tidak dikenal.",
		}))
		return
	}
	o, err := h.svc.Transition(r.Context(), identity.OrganizationID, identity.UserID, requestID(r),
		chi.URLParam(r, "id"), target, strings.TrimSpace(req.Reason))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, o)
}

// Cancel menangani POST /api/v1/orders/{id}/cancel.
// Body: { "reason": "..." }.
func (h *OrderHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[orderCancelRequest](w, r)
	if !ok {
		return
	}
	o, err := h.svc.Transition(r.Context(), identity.OrganizationID, identity.UserID, requestID(r),
		chi.URLParam(r, "id"), order.StatusCancelled, strings.TrimSpace(req.Reason))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, o)
}

// Summary menangani GET /api/v1/orders/summary.
func (h *OrderHandler) Summary(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	sum, err := h.svc.GetSummary(r.Context(), identity.OrganizationID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, sum)
}

// ─── Request DTOs ─────────────────────────────────────────────────────────

type orderCreateRequest struct {
	StoreID           string `json:"store_id"`
	CustomerID        string `json:"customer_id"`
	ShippingAddressID string `json:"shipping_address_id"`
	PaymentType       string `json:"payment_type"`
	ShippingCost      int64  `json:"shipping_cost"`
	Discount          int64  `json:"discount"`
	Notes             string `json:"notes"`
	ExternalOrderID   string `json:"external_order_id"`
	Items             []struct {
		VariantID   string `json:"variant_id"`
		ProductName string `json:"product_name"`
		VariantName string `json:"variant_name"`
		Qty         int    `json:"qty"`
		UnitPrice   int64  `json:"unit_price"`
	} `json:"items"`
}

type orderCancelRequest struct {
	Reason string `json:"reason"`
}

type orderUpdateRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}
