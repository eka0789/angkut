// Package handler — OrganizationHandler. Tipis: decode → service → encode.
package handler

import (
	"net/http"

	"github.com/angkut/angkut/apps/api/internal/application/organizationapp"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/dto"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
)

// OrganizationHandler menangani endpoint organisasi & keanggotaan.
type OrganizationHandler struct {
	svc *organizationapp.Service
}

// NewOrganizationHandler membuat handler organisasi.
func NewOrganizationHandler(svc *organizationapp.Service) *OrganizationHandler {
	return &OrganizationHandler{svc: svc}
}

// List menangani GET /api/v1/organizations — organisasi milik pengguna.
func (h *OrganizationHandler) List(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	items, err := h.svc.ListForUser(r.Context(), identity.UserID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	out := make([]dto.MembershipSummary, 0, len(items))
	for _, m := range items {
		out = append(out, dto.MembershipSummary{
			ID:   m.Organization.ID,
			Name: m.Organization.Name,
			Slug: m.Organization.Slug,
			Role: string(m.RoleCode),
		})
	}
	response.OK(w, out)
}

// Create menangani POST /api/v1/organizations.
func (h *OrganizationHandler) Create(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[dto.CreateOrganizationRequest](w, r)
	if !ok {
		return
	}
	org, err := h.svc.Create(r.Context(), req.Name, identity.UserID, orgContextInfo(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, dto.OrganizationSummary{ID: org.ID, Name: org.Name, Slug: org.Slug})
}

// Get menangani GET /api/v1/organizations/{id}.
func (h *OrganizationHandler) Get(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	orgID := orgIDFromPathOrContext(r, identity.OrganizationID)
	org, role, err := h.svc.Get(r.Context(), orgID, identity.UserID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{
		"id":            org.ID,
		"name":          org.Name,
		"slug":          org.Slug,
		"legal_name":    org.LegalName,
		"business_type": org.BusinessType,
		"phone":         org.Phone,
		"email":         org.Email,
		"logo_url":      org.LogoURL,
		"timezone":      org.Timezone,
		"currency":      org.Currency,
		"status":        string(org.Status),
		"your_role":     string(role),
	})
}

// Update menangani PATCH /api/v1/organizations/{id}.
func (h *OrganizationHandler) Update(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[dto.UpdateOrganizationRequest](w, r)
	if !ok {
		return
	}
	orgID := orgIDFromPathOrContext(r, identity.OrganizationID)
	org, err := h.svc.Update(r.Context(), orgID, identity.UserID, organizationapp.UpdateProfileInput{
		Name:         req.Name,
		LegalName:    req.LegalName,
		BusinessType: req.BusinessType,
		Phone:        req.Phone,
		Email:        req.Email,
		Timezone:     req.Timezone,
	}, orgContextInfo(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, dto.OrganizationSummary{ID: org.ID, Name: org.Name, Slug: org.Slug})
}

// ListMembers menangani GET /api/v1/organizations/{id}/members.
func (h *OrganizationHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	orgID := orgIDFromPathOrContext(r, identity.OrganizationID)
	members, err := h.svc.ListMembers(r.Context(), orgID, identity.UserID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	out := make([]dto.MemberResponse, 0, len(members))
	for _, m := range members {
		out = append(out, dto.MemberResponse{
			ID:       m.ID,
			UserID:   m.UserID,
			Name:     m.Name,
			Email:    m.Email,
			Role:     string(m.Role),
			Status:   string(m.Status),
			JoinedAt: m.JoinedAt,
			IsYou:    m.IsYou,
		})
	}
	response.OK(w, out)
}

// InviteMember menangani POST /api/v1/organizations/{id}/members.
func (h *OrganizationHandler) InviteMember(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[dto.InviteMemberRequest](w, r)
	if !ok {
		return
	}
	orgID := orgIDFromPathOrContext(r, identity.OrganizationID)
	res, err := h.svc.Invite(r.Context(), orgID, identity.UserID,
		organizationapp.InviteInput{Email: req.Email, Role: req.Role}, orgContextInfo(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, dto.InviteMemberResponse{
		MemberID:          res.MemberID,
		Email:             res.Email,
		Role:              string(res.Role),
		AlreadyRegistered: res.AlreadyRegistered,
	})
}

// UpdateMember menangani PATCH /api/v1/organizations/{id}/members/{mid}.
func (h *OrganizationHandler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[dto.UpdateMemberRequest](w, r)
	if !ok {
		return
	}
	orgID := orgIDFromPathOrContext(r, identity.OrganizationID)
	memberID := httpx.PathParam(r, "mid")
	err := h.svc.UpdateMember(r.Context(), orgID, identity.UserID, memberID,
		organizationapp.UpdateMemberInput{Role: req.Role, Status: req.Status}, orgContextInfo(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Message(w, http.StatusOK, "Anggota diperbarui.")
}

// RemoveMember menangani DELETE /api/v1/organizations/{id}/members/{mid}.
func (h *OrganizationHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	orgID := orgIDFromPathOrContext(r, identity.OrganizationID)
	memberID := httpx.PathParam(r, "mid")
	if err := h.svc.RemoveMember(r.Context(), orgID, identity.UserID, memberID, orgContextInfo(r)); err != nil {
		response.Fail(w, err)
		return
	}
	response.Message(w, http.StatusOK, "Anggota dihapus dari organisasi.")
}

// ListRoles menangani GET /api/v1/organizations/{id}/roles.
func (h *OrganizationHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	orgID := orgIDFromPathOrContext(r, identity.OrganizationID)
	roles, err := h.svc.Roles(r.Context(), orgID, identity.UserID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		out = append(out, string(role))
	}
	response.OK(w, map[string]any{"assignable_roles": out})
}

// ListPermissions menangani GET /api/v1/organizations/{id}/permissions.
// Dipakai FE untuk menyembunyikan UI; server tetap otoritatif.
func (h *OrganizationHandler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	orgID := orgIDFromPathOrContext(r, identity.OrganizationID)
	perms, err := h.svc.Permissions(r.Context(), orgID, identity.UserID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{
		"role":        string(identity.RoleCode),
		"permissions": perms,
	})
}

// ─── helper ───────────────────────────────────────────────────────────────

// orgIDFromPathOrContext memilih organisasi dari path, dan bila tidak ada
// memakai organisasi aktif di sesi. Nilai dari path TETAP diverifikasi oleh
// service (keanggotaan) — bukan dipercaya begitu saja.
func orgIDFromPathOrContext(r *http.Request, contextOrgID string) string {
	if id := httpx.PathParam(r, "id"); id != "" {
		return id
	}
	return contextOrgID
}

func orgContextInfo(r *http.Request) organizationapp.ContextInfo {
	return organizationapp.ContextInfo{
		IP:        clientIP(r),
		UserAgent: r.UserAgent(),
		RequestID: requestID(r),
	}
}
