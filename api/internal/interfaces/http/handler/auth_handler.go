// Package handler — AuthHandler. Tipis: decode → service → encode.
// Tidak ada aturan bisnis di file ini.
package handler

import (
	"net/http"

	"github.com/angkut/angkut/apps/api/internal/application/auth"
	"github.com/angkut/angkut/apps/api/internal/domain/user"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/dto"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
)

// AuthHandler menangani endpoint identitas.
type AuthHandler struct {
	svc   *auth.Service
	oauth auth.GoogleOAuth
	// stateSecret dipakai menandatangani parameter `state` OAuth agar callback
	// tidak dapat dipalsukan (anti CSRF).
	stateSecret string
}

// NewAuthHandler membuat handler auth.
func NewAuthHandler(svc *auth.Service, oauth auth.GoogleOAuth, stateSecret string) *AuthHandler {
	return &AuthHandler{svc: svc, oauth: oauth, stateSecret: stateSecret}
}

// Register menangani POST /api/v1/auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	req, ok := httpx.Decode[dto.RegisterRequest](w, r)
	if !ok {
		return
	}

	res, err := h.svc.Register(r.Context(), auth.RegisterCommand{
		Name:             req.Name,
		Email:            req.Email,
		Password:         req.Password,
		OrganizationName: req.OrganizationName,
		Context:          contextInfo(r),
	})
	if err != nil {
		response.Fail(w, err)
		return
	}

	response.Created(w, map[string]any{
		"user": dto.UserSummary{
			ID:            res.User.ID,
			Name:          res.User.Name,
			Email:         res.User.Email,
			Status:        string(res.User.Status),
			EmailVerified: res.User.IsEmailVerified(),
		},
		"organization": dto.OrganizationSummary{
			ID:   res.Organization.ID,
			Name: res.Organization.Name,
			Slug: res.Organization.Slug,
		},
		"next_step": "verify_email",
	})
}

// Login menangani POST /api/v1/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	req, ok := httpx.Decode[dto.LoginRequest](w, r)
	if !ok {
		return
	}

	res, err := h.svc.Login(r.Context(), auth.LoginCommand{
		Email:      req.Email,
		Password:   req.Password,
		DeviceName: req.DeviceName,
		Context:    contextInfo(r),
	})
	if err != nil {
		response.Fail(w, err)
		return
	}

	if res.RequiresMFA {
		response.OK(w, dto.MFARequiredResponse{MFARequired: true, MFAToken: res.MFAToken})
		return
	}
	response.OK(w, sessionResponse(res.Auth))
}

// VerifyTwoFactor menangani POST /api/v1/auth/2fa/verify.
func (h *AuthHandler) VerifyTwoFactor(w http.ResponseWriter, r *http.Request) {
	req, ok := httpx.Decode[dto.VerifyTwoFactorRequest](w, r)
	if !ok {
		return
	}

	res, err := h.svc.VerifyTwoFactorLogin(r.Context(), req.MFAToken, req.Code, req.DeviceName, contextInfo(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, sessionResponse(res))
}

// Refresh menangani POST /api/v1/auth/refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	req, ok := httpx.Decode[dto.RefreshRequest](w, r)
	if !ok {
		return
	}

	res, err := h.svc.Refresh(r.Context(), req.RefreshToken, req.DeviceName, contextInfo(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, sessionResponse(res))
}

// Logout menangani POST /api/v1/auth/logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, user.ErrTokenInvalid)
		return
	}
	if err := h.svc.Logout(r.Context(), identity.UserID, identity.SessionID, contextInfo(r)); err != nil {
		response.Fail(w, err)
		return
	}
	response.Message(w, http.StatusOK, "Anda telah keluar.")
}

// LogoutAll menangani POST /api/v1/auth/logout-all.
func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, user.ErrTokenInvalid)
		return
	}
	n, err := h.svc.LogoutAll(r.Context(), identity.UserID, contextInfo(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{"revoked_sessions": n})
}

// ForgotPassword menangani POST /api/v1/auth/forgot-password.
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	req, ok := httpx.Decode[dto.ForgotPasswordRequest](w, r)
	if !ok {
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), req.Email, contextInfo(r)); err != nil {
		response.Fail(w, err)
		return
	}
	// Selalu sukses agar endpoint ini tidak dapat dipakai memetakan email terdaftar.
	response.Message(w, http.StatusOK,
		"Jika email tersebut terdaftar, kami telah mengirim tautan reset kata sandi.")
}

// ResetPassword menangani POST /api/v1/auth/reset-password.
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	req, ok := httpx.Decode[dto.ResetPasswordRequest](w, r)
	if !ok {
		return
	}
	if err := h.svc.ResetPassword(r.Context(), req.Token, req.Password, contextInfo(r)); err != nil {
		response.Fail(w, err)
		return
	}
	response.Message(w, http.StatusOK,
		"Kata sandi berhasil diubah. Semua sesi lain telah dihentikan untuk keamanan.")
}

// ChangePassword menangani POST /api/v1/auth/change-password.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, user.ErrTokenInvalid)
		return
	}
	req, ok := httpx.Decode[dto.ChangePasswordRequest](w, r)
	if !ok {
		return
	}
	err := h.svc.ChangePassword(r.Context(), identity.UserID,
		req.CurrentPassword, req.NewPassword, identity.SessionID, contextInfo(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Message(w, http.StatusOK, "Kata sandi berhasil diubah.")
}

// VerifyEmail menangani POST /api/v1/auth/verify-email.
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	req, ok := httpx.Decode[dto.VerifyEmailRequest](w, r)
	if !ok {
		return
	}
	if err := h.svc.VerifyEmail(r.Context(), req.Token, contextInfo(r)); err != nil {
		response.Fail(w, err)
		return
	}
	response.Message(w, http.StatusOK, "Email Anda berhasil diverifikasi. Silakan masuk.")
}

// ResendVerification menangani POST /api/v1/auth/resend-verification.
func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	req, ok := httpx.Decode[dto.ResendVerificationRequest](w, r)
	if !ok {
		return
	}
	if err := h.svc.ResendVerification(r.Context(), req.Email, contextInfo(r)); err != nil {
		response.Fail(w, err)
		return
	}
	response.Message(w, http.StatusOK,
		"Jika email tersebut terdaftar dan belum terverifikasi, kami telah mengirim tautan verifikasi.")
}

// Me mengembalikan profil pengguna yang sedang login.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, user.ErrTokenInvalid)
		return
	}
	u, err := h.svc.GetProfile(r.Context(), identity.UserID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, toUserSummary(u))
}

// UpdateProfile menangani PATCH /api/v1/auth/me.
func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, user.ErrTokenInvalid)
		return
	}
	req, ok := httpx.Decode[dto.UpdateProfileRequest](w, r)
	if !ok {
		return
	}
	u, err := h.svc.UpdateProfile(r.Context(), identity.UserID,
		req.Name, req.Phone, req.AvatarURL, req.Locale, req.Timezone, contextInfo(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, toUserSummary(u))
}

// ListSessions menangani GET /api/v1/auth/sessions.
func (h *AuthHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, user.ErrTokenInvalid)
		return
	}
	sessions, err := h.svc.ListSessions(r.Context(), identity.UserID, identity.SessionID)
	if err != nil {
		response.Fail(w, err)
		return
	}

	out := make([]dto.SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		item := dto.SessionInfo{
			ID:        s.ID,
			Device:    s.Device,
			IP:        s.IP,
			UserAgent: s.UserAgent,
			CreatedAt: s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			ExpiresAt: s.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"),
			IsCurrent: s.IsCurrent,
		}
		if s.LastUsedAt != nil {
			item.LastUsedAt = s.LastUsedAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		out = append(out, item)
	}
	response.OK(w, out)
}

// RevokeSession menangani DELETE /api/v1/auth/sessions/{id}.
func (h *AuthHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, user.ErrTokenInvalid)
		return
	}
	sessionID := httpx.PathParam(r, "id")
	if sessionID == "" {
		response.Fail(w, user.ErrTokenInvalid)
		return
	}
	if err := h.svc.RevokeSession(r.Context(), identity.UserID, sessionID, contextInfo(r)); err != nil {
		response.Fail(w, err)
		return
	}
	response.Message(w, http.StatusOK, "Perangkat telah dikeluarkan.")
}

// SetupTwoFactor menangani POST /api/v1/auth/2fa/setup.
func (h *AuthHandler) SetupTwoFactor(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, user.ErrTokenInvalid)
		return
	}
	res, err := h.svc.SetupTwoFactor(r.Context(), identity.UserID, contextInfo(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, dto.SetupTwoFactorResponse{
		Secret:        res.Secret,
		OTPAuthURI:    res.OTPAuthURI,
		RecoveryCodes: res.RecoveryCodes,
	})
}

// EnableTwoFactor menangani POST /api/v1/auth/2fa/enable.
func (h *AuthHandler) EnableTwoFactor(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, user.ErrTokenInvalid)
		return
	}
	req, ok := httpx.Decode[dto.EnableTwoFactorRequest](w, r)
	if !ok {
		return
	}
	if err := h.svc.EnableTwoFactor(r.Context(), identity.UserID, req.Code, contextInfo(r)); err != nil {
		response.Fail(w, err)
		return
	}
	response.Message(w, http.StatusOK, "2FA berhasil diaktifkan. Simpan kode pemulihan Anda di tempat aman.")
}

// DisableTwoFactor menangani POST /api/v1/auth/2fa/disable.
func (h *AuthHandler) DisableTwoFactor(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, user.ErrTokenInvalid)
		return
	}
	req, ok := httpx.Decode[dto.DisableTwoFactorRequest](w, r)
	if !ok {
		return
	}
	err := h.svc.DisableTwoFactor(r.Context(), identity.UserID,
		req.Password, req.Code, identity.SessionID, contextInfo(r))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Message(w, http.StatusOK, "2FA dinonaktifkan.")
}

// TwoFactorStatus menangani GET /api/v1/auth/2fa.
func (h *AuthHandler) TwoFactorStatus(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, user.ErrTokenInvalid)
		return
	}
	st, err := h.svc.StatusTwoFactor(r.Context(), identity.UserID)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, dto.TwoFactorStatusResponse{
		Enabled:        st.Enabled,
		ConfirmedAt:    st.ConfirmedAt,
		RemainingCodes: st.RemainingCodes,
	})
}

// ─── helper ───────────────────────────────────────────────────────────────

func contextInfo(r *http.Request) auth.ContextInfo {
	return auth.ContextInfo{
		IP:        clientIP(r),
		UserAgent: r.UserAgent(),
		RequestID: requestID(r),
	}
}

func sessionResponse(res *auth.AuthResult) dto.SessionResponse {
	orgs := make([]dto.MembershipSummary, 0, len(res.Organizations))
	for _, m := range res.Organizations {
		orgs = append(orgs, dto.MembershipSummary{
			ID:   m.Organization.ID,
			Name: m.Organization.Name,
			Slug: m.Organization.Slug,
			Role: string(m.RoleCode),
		})
	}
	return dto.SessionResponse{
		AccessToken:   res.Session.AccessToken,
		RefreshToken:  res.Session.RefreshToken,
		ExpiresIn:     res.Session.ExpiresIn,
		TokenType:     "Bearer",
		User:          toUserSummary(res.User),
		Organizations: orgs,
		ActiveOrgID:   res.ActiveOrgID,
		ActiveRole:    string(res.ActiveRole),
	}
}

func toUserSummary(u *user.User) dto.UserSummary {
	if u == nil {
		return dto.UserSummary{}
	}
	return dto.UserSummary{
		ID:               u.ID,
		Name:             u.Name,
		Email:            u.Email,
		Status:           string(u.Status),
		EmailVerified:    u.IsEmailVerified(),
		TwoFactorEnabled: u.TwoFAEnabled,
		AvatarURL:        u.AvatarURL,
		Locale:           u.Locale,
		Timezone:         u.Timezone,
	}
}
