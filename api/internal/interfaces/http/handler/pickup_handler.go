package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/application/pickupapp"
	"github.com/angkut/angkut/apps/internal/domain/pickup"
)

// PickupService adalah antarmuka minimal service pickup untuk handler.
type PickupService interface {
	Create(ctx context.Context, orgID, actorUserID, requestID string, in pickupapp.CreateInput) (*pickup.Request, error)
	Get(ctx context.Context, orgID, id string) (*pickup.Request, error)
	List(ctx context.Context, orgID string, f pickup.ListFilter) ([]pickup.Request, int, error)
	Schedule(ctx context.Context, orgID, actorUserID, requestID, id string, when time.Time) (*pickup.Request, error)
	Complete(ctx context.Context, orgID, actorUserID, requestID, id string) (*pickup.Request, error)
	Cancel(ctx context.Context, orgID, actorUserID, requestID, id string) (*pickup.Request, error)
}

// PickupHandler menangani endpoint pickup request tenant-scoped.
type PickupHandler struct {
	svc PickupService
}

// NewPickupHandler membuat handler pickup.
func NewPickupHandler(svc PickupService) *PickupHandler {
	return &PickupHandler{svc: svc}
}

// RegisterRoutes mendaftarkan rute di /api/v1/pickups.
func (h *PickupHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Route("/{id}", func(one chi.Router) {
		one.Get("/", h.Get)
		one.Post("/schedule", h.Schedule)
		one.Post("/complete", h.Complete)
		one.Post("/cancel", h.Cancel)
	})
}

func (h *PickupHandler) Create(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[pickupCreateRequest](w, r)
	if !ok {
		return
	}
	var when *time.Time
	if strings.TrimSpace(req.ScheduledAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ScheduledAt))
		if err != nil {
			response.Fail(w, ValidationError(map[string]string{"scheduled_at": "Format waktu tidak valid (RFC3339)."}))
			return
		}
		when = &t
	}
	pr, err := h.svc.Create(r.Context(), identity.OrganizationID, identity.UserID, requestID(r), pickupapp.CreateInput{
		WarehouseID: strings.TrimSpace(req.WarehouseID),
		CourierCode: strings.TrimSpace(req.CourierCode),
		ScheduledAt: when,
		Notes:       req.Notes,
		ShipmentIDs: req.ShipmentIDs,
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, pr)
}

func (h *PickupHandler) Get(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	pr, err := h.svc.Get(r.Context(), identity.OrganizationID, id)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, pr)
}

func (h *PickupHandler) List(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	f := pickup.ListFilter{
		Status:      pickup.Status(strings.TrimSpace(q.Get("status"))),
		WarehouseID: strings.TrimSpace(q.Get("warehouse_id")),
		Limit:       atoiDefault(q.Get("limit"), 50),
		Offset:      atoiDefault(q.Get("offset"), 0),
	}
	items, total, err := h.svc.List(r.Context(), identity.OrganizationID, f)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{"total": total, "limit": f.Limit, "offset": f.Offset})
}

func (h *PickupHandler) Schedule(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	req, ok := httpx.Decode[pickupScheduleRequest](w, r)
	if !ok {
		return
	}
	when, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ScheduledAt))
	if err != nil {
		response.Fail(w, ValidationError(map[string]string{"scheduled_at": "Format waktu tidak valid (RFC3339)."}))
		return
	}
	pr, err := h.svc.Schedule(r.Context(), identity.OrganizationID, identity.UserID, requestID(r), id, when)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, pr)
}

func (h *PickupHandler) Complete(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	pr, err := h.svc.Complete(r.Context(), identity.OrganizationID, identity.UserID, requestID(r), id)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, pr)
}

func (h *PickupHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	pr, err := h.svc.Cancel(r.Context(), identity.OrganizationID, identity.UserID, requestID(r), id)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, pr)
}

type pickupCreateRequest struct {
	WarehouseID string   `json:"warehouse_id"`
	CourierCode string   `json:"courier_code"`
	ScheduledAt string   `json:"scheduled_at"`
	Notes       string   `json:"notes"`
	ShipmentIDs []string `json:"shipment_ids"`
}

type pickupScheduleRequest struct {
	ScheduledAt string `json:"scheduled_at"`
}
