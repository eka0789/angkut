package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/application/notificationapp"
	"github.com/angkut/angkut/apps/internal/domain/notification"
)

// NotificationService adalah antarmuka minimal service notifikasi untuk handler.
type NotificationService interface {
	List(ctx context.Context, orgID, userID string, f notificationapp.ListFilter) ([]notification.Notification, int, error)
	UnreadCount(ctx context.Context, orgID, userID string) (int, error)
	MarkRead(ctx context.Context, orgID, userID, id string) error
	MarkAllRead(ctx context.Context, orgID, userID string) (int, error)
	Templates(ctx context.Context, orgID string) ([]notificationapp.TemplateView, error)
	UpdateTemplate(ctx context.Context, orgID string, in notificationapp.UpdateTemplateInput) error
	Preferences(ctx context.Context, userID string) ([]notificationapp.PreferenceView, error)
	UpdatePreference(ctx context.Context, userID string, in notificationapp.UpdatePreferenceInput) error
}

// NotificationHandler menangani endpoint notifikasi tenant-scoped.
type NotificationHandler struct {
	svc NotificationService
}

// NewNotificationHandler membuat handler notifikasi.
func NewNotificationHandler(svc NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// RegisterRoutes mendaftarkan rute di /api/v1/notifications.
func (h *NotificationHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.List)
	r.Get("/unread-count", h.UnreadCount)
	r.Post("/read-all", h.MarkAllRead)
	r.Route("/{id}", func(one chi.Router) {
		one.Post("/read", h.MarkRead)
	})
}

// List menangani GET /api/v1/notifications?unread=&priority=&limit=&offset=.
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	items, total, err := h.svc.List(r.Context(), identity.OrganizationID, identity.UserID, notificationapp.ListFilter{
		Unread:   q.Get("unread") == "true" || q.Get("unread") == "1",
		Priority: strings.TrimSpace(q.Get("priority")),
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

// UnreadCount menangani GET /api/v1/notifications/unread-count.
func (h *NotificationHandler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	n, err := h.svc.UnreadCount(r.Context(), identity.OrganizationID, identity.UserID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{"unread": n})
}

// MarkRead menangani POST /api/v1/notifications/{id}/read.
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if err := h.svc.MarkRead(r.Context(), identity.OrganizationID, identity.UserID, id); err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{"read": true})
}

// MarkAllRead menangani POST /api/v1/notifications/read-all.
func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	n, err := h.svc.MarkAllRead(r.Context(), identity.OrganizationID, identity.UserID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{"marked": n})
}

// ── Template (admin+) ───────────────────────────────────────────────────

// Templates menangani GET /api/v1/notification-templates.
func (h *NotificationHandler) Templates(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	items, err := h.svc.Templates(r.Context(), identity.OrganizationID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, items)
}

type notificationTemplateUpdateRequest struct {
	Key   string `json:"key"`
	Title string `json:"title_template"`
	Body  string `json:"body_template"`
}

// UpdateTemplate menangani PUT /api/v1/notification-templates/{key}.
func (h *NotificationHandler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	key := strings.TrimSpace(chi.URLParam(r, "key"))
	req, ok := httpx.Decode[notificationTemplateUpdateRequest](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Key) != "" && strings.TrimSpace(req.Key) != key {
		response.Fail(w, ValidationError(map[string]string{
			"key": "Key pada body harus sama dengan key pada URL.",
		}))
		return
	}
	if err := h.svc.UpdateTemplate(r.Context(), identity.OrganizationID, notificationapp.UpdateTemplateInput{
		Key: key, Title: req.Title, Body: req.Body, Actor: identity.UserID,
	}); err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{"updated": true, "key": key})
}

// ── Preferensi (member) ─────────────────────────────────────────────────

// Preferences menangani GET /api/v1/notification-preferences.
func (h *NotificationHandler) Preferences(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	items, err := h.svc.Preferences(r.Context(), identity.UserID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, items)
}

type notificationPreferenceUpdateRequest struct {
	EventKey string `json:"event_key"`
	Channel  string `json:"channel"`
	Enabled  *bool  `json:"enabled"`
}

// UpdatePreference menangani PUT /api/v1/notification-preferences.
func (h *NotificationHandler) UpdatePreference(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[notificationPreferenceUpdateRequest](w, r)
	if !ok {
		return
	}
	if req.Enabled == nil {
		response.Fail(w, ValidationError(map[string]string{
			"enabled": "Nilai enabled wajib diisi (true/false).",
		}))
		return
	}
	if err := h.svc.UpdatePreference(r.Context(), identity.UserID, notificationapp.UpdatePreferenceInput{
		Key: req.EventKey, Channel: req.Channel, Enabled: *req.Enabled,
	}); err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{"updated": true})
}
