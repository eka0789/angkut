// Package dto berisi bentuk request/response HTTP beserta tag validasinya.
// Validasi di sini adalah lapis pertama; server tetap otoritatif lewat domain.
package dto

// ─── Auth ─────────────────────────────────────────────────────────────────

// RegisterRequest adalah body pendaftaran.
type RegisterRequest struct {
	Name             string `json:"name" validate:"required,min=2,max=120"`
	Email            string `json:"email" validate:"required,email,max=254"`
	Password         string `json:"password" validate:"required,min=8,max=128"`
	OrganizationName string `json:"organization_name" validate:"required,min=2,max=120"`
	DeviceName       string `json:"device_name" validate:"omitempty,max=120"`
}

// LoginRequest adalah body login.
type LoginRequest struct {
	Email      string `json:"email" validate:"required,email,max=254"`
	Password   string `json:"password" validate:"required,max=128"`
	DeviceName string `json:"device_name" validate:"omitempty,max=120"`
}

// VerifyTwoFactorRequest menyelesaikan login yang tertahan 2FA.
type VerifyTwoFactorRequest struct {
	MFAToken   string `json:"mfa_token" validate:"required"`
	Code       string `json:"code" validate:"required,min=6,max=12"`
	DeviceName string `json:"device_name" validate:"omitempty,max=120"`
}

// RefreshRequest menukar refresh token.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
	DeviceName   string `json:"device_name" validate:"omitempty,max=120"`
}

// LogoutRequest mencabut satu sesi. Refresh token opsional: bila dikirim,
// sesi yang cocok dengan token itu yang dicabut.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"omitempty"`
}

// ForgotPasswordRequest meminta tautan reset.
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email,max=254"`
}

// ResetPasswordRequest menetapkan password baru.
type ResetPasswordRequest struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required,min=8,max=128"`
}

// ChangePasswordRequest mengganti password saat sudah login.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,max=128"`
	NewPassword     string `json:"new_password" validate:"required,min=8,max=128"`
}

// VerifyEmailRequest memverifikasi alamat email.
type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required"`
}

// ResendVerificationRequest mengirim ulang email verifikasi.
type ResendVerificationRequest struct {
	Email string `json:"email" validate:"required,email,max=254"`
}

// UpdateProfileRequest mengubah profil pengguna.
type UpdateProfileRequest struct {
	Name      string `json:"name" validate:"required,min=2,max=120"`
	Phone     string `json:"phone" validate:"omitempty,max=20"`
	AvatarURL string `json:"avatar_url" validate:"omitempty,url,max=500"`
	Locale    string `json:"locale" validate:"omitempty,oneof=id en"`
	Timezone  string `json:"timezone" validate:"omitempty,max=64"`
}

// SetupTwoFactorResponse mengembalikan rahasia 2FA sekali saja.
type SetupTwoFactorResponse struct {
	Secret        string   `json:"secret"`
	OTPAuthURI    string   `json:"otpauth_uri"`
	RecoveryCodes []string `json:"recovery_codes"`
}

// EnableTwoFactorRequest mengaktifkan 2FA dengan membuktikan kode.
type EnableTwoFactorRequest struct {
	Code string `json:"code" validate:"required,min=6,max=12"`
}

// DisableTwoFactorRequest mematikan 2FA (butuh password + kode).
type DisableTwoFactorRequest struct {
	Password string `json:"password" validate:"required,max=128"`
	Code     string `json:"code" validate:"required,min=6,max=12"`
}

// ─── Organization ─────────────────────────────────────────────────────────

// CreateOrganizationRequest membuat organisasi baru.
type CreateOrganizationRequest struct {
	Name string `json:"name" validate:"required,min=2,max=120"`
}

// UpdateOrganizationRequest mengubah profil organisasi (semua field opsional).
type UpdateOrganizationRequest struct {
	Name         *string `json:"name" validate:"omitempty,min=2,max=120"`
	LegalName    *string `json:"legal_name" validate:"omitempty,max=200"`
	BusinessType *string `json:"business_type" validate:"omitempty,max=50"`
	Phone        *string `json:"phone" validate:"omitempty,max=20"`
	Email        *string `json:"email" validate:"omitempty,email,max=254"`
	Timezone     *string `json:"timezone" validate:"omitempty,max=64"`
}

// InviteMemberRequest menambahkan anggota.
type InviteMemberRequest struct {
	Email string `json:"email" validate:"required,email,max=254"`
	Role  string `json:"role" validate:"required,oneof=owner admin finance warehouse staff viewer"`
}

// UpdateMemberRequest mengubah role/status anggota.
type UpdateMemberRequest struct {
	Role   *string `json:"role" validate:"omitempty,oneof=owner admin finance warehouse staff viewer"`
	Status *string `json:"status" validate:"omitempty,oneof=active suspended"`
}

// ─── Respons umum ─────────────────────────────────────────────────────────

// UserSummary adalah representasi pengguna yang aman untuk dikirim ke klien.
// TIDAK pernah memuat password hash, secret 2FA, atau token.
type UserSummary struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Status           string `json:"status"`
	EmailVerified    bool   `json:"email_verified"`
	TwoFactorEnabled bool   `json:"two_factor_enabled"`
	AvatarURL        string `json:"avatar_url,omitempty"`
	Locale           string `json:"locale,omitempty"`
	Timezone         string `json:"timezone,omitempty"`
}

// OrganizationSummary adalah ringkasan organisasi.
type OrganizationSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// MembershipSummary adalah organisasi + role pengguna di dalamnya.
type MembershipSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Role string `json:"role"`
}

// SessionResponse adalah token yang baru diterbitkan.
type SessionResponse struct {
	AccessToken   string              `json:"access_token"`
	RefreshToken  string              `json:"refresh_token"`
	ExpiresIn     int                 `json:"expires_in"`
	TokenType     string              `json:"token_type"`
	User          UserSummary         `json:"user"`
	Organizations []MembershipSummary `json:"organizations"`
	ActiveOrgID   string              `json:"active_organization_id,omitempty"`
	ActiveRole    string              `json:"active_role,omitempty"`
}

// MFARequiredResponse dikirim saat login tertahan langkah kedua.
type MFARequiredResponse struct {
	MFARequired bool   `json:"mfa_required"`
	MFAToken    string `json:"mfa_token"`
}

// SessionInfo adalah satu baris daftar perangkat.
type SessionInfo struct {
	ID         string `json:"id"`
	Device     string `json:"device"`
	IP         string `json:"ip,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	ExpiresAt  string `json:"expires_at"`
	IsCurrent  bool   `json:"is_current"`
}

// MemberResponse adalah satu baris daftar anggota.
type MemberResponse struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Status   string `json:"status"`
	JoinedAt string `json:"joined_at,omitempty"`
	IsYou    bool   `json:"is_you"`
}

// InviteMemberResponse menjelaskan hasil penambahan anggota.
type InviteMemberResponse struct {
	MemberID          string `json:"member_id"`
	Email             string `json:"email"`
	Role              string `json:"role"`
	AlreadyRegistered bool   `json:"already_registered"`
}

// TwoFactorStatusResponse adalah ringkasan status 2FA.
type TwoFactorStatusResponse struct {
	Enabled        bool   `json:"enabled"`
	ConfirmedAt    string `json:"confirmed_at,omitempty"`
	RemainingCodes int    `json:"remaining_recovery_codes"`
}
