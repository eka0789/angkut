package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/application/platformapp"
)

// PlatformService antarmuka service platform untuk handler.
type PlatformService interface {
	CreateKey(ctx context.Context, orgID, requestID string, in platformapp.CreateKeyInput) (*platformapp.CreateKeyResult, error)
	ListKeys(ctx context.Context, orgID string) ([]platformapp.APIKey, error)
	RevokeKey(ctx context.Context, orgID, id, actor, requestID string) error
	Search(ctx context.Context, orgID, query string, limit int) ([]platformapp.SearchHit, error)
	CheckShipmentQuota(ctx context.Context, orgID string) (*platformapp.QuotaCheck, error)
}

// PlatformHandler menangani endpoint platform (API keys, search, quota).
type PlatformHandler struct {
	svc PlatformService
}

// NewPlatformHandler membuat handler platform.
func NewPlatformHandler(svc PlatformService) *PlatformHandler {
	return &PlatformHandler{svc: svc}
}

// RegisterKeys mendaftarkan rute /api-keys (admin+).
func (h *PlatformHandler) RegisterKeys(r chi.Router) {
	r.Get("/", h.ListKeys)
	r.Post("/", h.CreateKey)
	r.Post("/{id}/revoke", h.RevokeKey)
}

// RegisterSearch mendaftarkan GET /search (member).
func (h *PlatformHandler) RegisterSearch(r chi.Router) {
	r.Get("/", h.Search)
}

// RegisterQuota mendaftarkan GET /subscription/quota (member).
func (h *PlatformHandler) RegisterQuota(r chi.Router) {
	r.Get("/quota", h.Quota)
}

func (h *PlatformHandler) ListKeys(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	items, err := h.svc.ListKeys(r.Context(), identity.OrganizationID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, items)
}

type createKeyRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expires_at"`
}

func (h *PlatformHandler) CreateKey(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[createKeyRequest](w, r)
	if !ok {
		return
	}
	in := platformapp.CreateKeyInput{Name: req.Name, Scopes: req.Scopes, Actor: identity.UserID}
	if ts := strings.TrimSpace(orEmpty(req.ExpiresAt)); ts != "" {
		t, err := parseRFC3339(ts)
		if err != nil {
			response.Fail(w, ValidationError(map[string]string{"expires_at": "Format waktu tidak valid (RFC3339)."}))
			return
		}
		in.ExpiresAt = &t
	}
	res, err := h.svc.CreateKey(r.Context(), identity.OrganizationID, requestID(r), in)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, res)
}

func (h *PlatformHandler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	if err := h.svc.RevokeKey(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"), identity.UserID, requestID(r)); err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{"revoked": true})
}

// Search menangani GET /api/v1/search?q=.
func (h *PlatformHandler) Search(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	items, err := h.svc.Search(r.Context(), identity.OrganizationID,
		r.URL.Query().Get("q"), atoiDefault(r.URL.Query().Get("limit"), 10))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, items)
}

// Quota menangani GET /api/v1/subscription/quota.
func (h *PlatformHandler) Quota(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	qc, err := h.svc.CheckShipmentQuota(r.Context(), identity.OrganizationID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, qc)
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
