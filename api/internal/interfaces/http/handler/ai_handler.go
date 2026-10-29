package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/application/aiapp"
	"github.com/angkut/angkut/apps/internal/application/analyticsapp"
)

// AIService antarmuka AI untuk handler.
type AIService interface {
	Chat(ctx context.Context, in aiapp.ChatInput) (*aiapp.ChatResult, error)
	Confirm(ctx context.Context, orgID, userID, actionID string, confirm bool) (string, error)
	Insights(ctx context.Context, orgID string) ([]aiapp.Insight, error)
}

// AnalyticsService antarmuka analytics untuk handler.
type AnalyticsService interface {
	Dashboard(ctx context.Context, orgID string) (*analyticsapp.Dashboard, error)
	HealthScore(ctx context.Context, orgID string) (*analyticsapp.HealthScore, error)
}

// AIHandler menangani /ai dan /analytics.
type AIHandler struct {
	ai AIService
	an AnalyticsService
}

// NewAIHandler membuat handler AI+analytics.
func NewAIHandler(ai AIService, an AnalyticsService) *AIHandler {
	return &AIHandler{ai: ai, an: an}
}

// RegisterAI mendaftarkan rute /ai.
func (h *AIHandler) RegisterAI(r chi.Router) {
	r.Post("/chat", h.Chat)
	r.Post("/actions/{id}/confirm", h.ConfirmAction)
	r.Get("/insights", h.Insights)
}

// RegisterAnalytics mendaftarkan rute /analytics.
func (h *AIHandler) RegisterAnalytics(r chi.Router) {
	r.Get("/dashboard", h.Dashboard)
	r.Get("/health-score", h.HealthScore)
}

type chatRequest struct {
	Message string `json:"message"`
}

// Chat menangani POST /api/v1/ai/chat.
func (h *AIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[chatRequest](w, r)
	if !ok {
		return
	}
	res, err := h.ai.Chat(r.Context(), aiapp.ChatInput{
		OrganizationID: identity.OrganizationID, UserID: identity.UserID,
		Message: req.Message,
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	if res.RequiresCfm && res.Pending != nil {
		response.OK(w, map[string]any{
			"content":               res.Content,
			"requires_confirmation": true,
			"pending_action": map[string]any{
				"id": res.Pending.ID, "type": res.Pending.Type,
				"params": res.Pending.Params, "summary": res.Pending.Summary,
			},
		})
		return
	}
	response.OK(w, map[string]any{
		"content": res.Content, "tool_calls": res.ToolCalls,
		"requires_confirmation": false,
	})
}

type confirmRequest struct {
	Confirm *bool `json:"confirm"`
}

// ConfirmAction menangani POST /api/v1/ai/actions/{id}/confirm.
func (h *AIHandler) ConfirmAction(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[confirmRequest](w, r)
	if !ok || req.Confirm == nil {
		response.Fail(w, ValidationError(map[string]string{"confirm": "Nilai confirm (true/false) wajib."}))
		return
	}
	msg, err := h.ai.Confirm(r.Context(), identity.OrganizationID, identity.UserID,
		chi.URLParam(r, "id"), *req.Confirm)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{"result": msg})
}

// Insights menangani GET /api/v1/ai/insights.
func (h *AIHandler) Insights(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	items, err := h.ai.Insights(r.Context(), identity.OrganizationID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, items)
}

// Dashboard menangani GET /api/v1/analytics/dashboard.
func (h *AIHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	d, err := h.an.Dashboard(r.Context(), identity.OrganizationID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, d)
}

// HealthScore menangani GET /api/v1/analytics/health-score.
func (h *AIHandler) HealthScore(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	hs, err := h.an.HealthScore(r.Context(), identity.OrganizationID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, hs)
}
