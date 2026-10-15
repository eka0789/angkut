package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/application/trackingapp"
	"github.com/angkut/angkut/apps/internal/domain/shipment"
	"github.com/angkut/angkut/apps/internal/domain/tracking"
)

// TrackingService adalah antarmuka minimal service tracking untuk handler.
type TrackingService interface {
	Append(ctx context.Context, orgID, actorUserID, requestID string, in trackingapp.AppendInput) (*trackingapp.AppendResult, error)
	Timeline(ctx context.Context, orgID, shipmentID string) (*tracking.Timeline, *shipment.Shipment, error)
	TimelineByAWB(ctx context.Context, orgID, awb string) (*tracking.Timeline, *shipment.Shipment, error)
	List(ctx context.Context, orgID string, f tracking.Filter) ([]tracking.Event, int, error)
}

// TrackingHandler menangani endpoint pelacakan tenant-scoped.
type TrackingHandler struct {
	svc TrackingService
}

// NewTrackingHandler membuat handler pelacakan.
func NewTrackingHandler(svc TrackingService) *TrackingHandler {
	return &TrackingHandler{svc: svc}
}

// RegisterRoutes mendaftarkan rute di /api/v1/tracking.
func (h *TrackingHandler) RegisterRoutes(r chi.Router) {
	// AWB berbentuk teks bebas (bukan uuid), jadi rute memakai path param biasa.
	r.Get("/{awb}", h.ByAWB)
	r.Post("/events", h.Append)
	r.Get("/events", h.List)
}

// ByAWB menangani GET /api/v1/tracking/{awb} — timeline + penilaian keyakinan.
func (h *TrackingHandler) ByAWB(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	awb := strings.TrimSpace(chi.URLParam(r, "awb"))
	if awb == "" {
		response.Fail(w, ValidationError(map[string]string{
			"awb": "Nomor resi wajib diisi.",
		}))
		return
	}

	tl, sh, err := h.svc.TimelineByAWB(r.Context(), identity.OrganizationID, awb)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{
		"awb":              tl.AWB,
		"status":           tl.Status,
		"courier":          map[string]any{"code": sh.CourierCode, "service": sh.ServiceCode},
		"events":           tl.Events,
		"last_location":    tl.LastLocation,
		"last_lat":         tl.LastLat,
		"last_lng":         tl.LastLng,
		"last_occurred_at": tl.OccurredAt,
		"eta_min_days":     sh.EtaMinDays,
		"eta_max_days":     sh.EtaMaxDays,
		"confidence":       tracking.EvaluateConfidence(sh, nowUTC()),
		"shipment_id":      sh.ID,
	})
}

// Append menangani POST /api/v1/tracking/events — pencatatan manual oleh staf.
func (h *TrackingHandler) Append(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[trackingAppendRequest](w, r)
	if !ok {
		return
	}

	res, err := h.svc.Append(r.Context(), identity.OrganizationID, identity.UserID, requestID(r), trackingapp.AppendInput{
		ShipmentID:  strings.TrimSpace(req.ShipmentID),
		Status:      tracking.EventStatus(strings.TrimSpace(req.Status)),
		Description: req.Description,
		Location:    req.Location,
		Latitude:    req.Latitude,
		Longitude:   req.Longitude,
		OccurredAt:  parseOptionalTime(req.OccurredAt),
		Source:      tracking.SourceManual,
	})
	if err != nil {
		response.Fail(w, err)
		return
	}

	payload := map[string]any{
		"duplicate":       res.Duplicate,
		"status_changed":  res.StatusChanged,
		"shipment_status": string(res.ShipmentStatus),
	}
	if res.Event != nil {
		payload["event"] = res.Event
	}
	// Event yang sudah ada bukan kegagalan — dikembalikan 200 dengan flag agar
	// klien tahu permintaannya dipenuhi tanpa efek ganda.
	response.OK(w, payload)
}

// List menangani GET /api/v1/tracking/events.
func (h *TrackingHandler) List(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	f := tracking.Filter{
		ShipmentID: strings.TrimSpace(q.Get("shipment_id")),
		Status:     tracking.EventStatus(strings.TrimSpace(q.Get("status"))),
		Source:     tracking.Source(strings.TrimSpace(q.Get("source"))),
		From:       strings.TrimSpace(q.Get("from")),
		To:         strings.TrimSpace(q.Get("to")),
		Limit:      atoiDefault(q.Get("limit"), 50),
		Offset:     atoiDefault(q.Get("offset"), 0),
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

// ─── Request DTOs ─────────────────────────────────────────────────────────

type trackingAppendRequest struct {
	ShipmentID  string   `json:"shipment_id"`
	Status      string   `json:"status"`
	Description string   `json:"description"`
	Location    string   `json:"location"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
	OccurredAt  string   `json:"occurred_at"`
}
