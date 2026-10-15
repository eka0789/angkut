package auth

import (
	"context"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/organization"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/user"
	"github.com/angkut/angkut/apps/internal/infrastructure/cryptox"
	"github.com/angkut/angkut/apps/internal/infrastructure/tokenx"
)

// LoginCommand adalah permintaan login.
type LoginCommand struct {
	Email      string
	Password   string
	DeviceName string
	Context    ContextInfo
}

// ErrInvalidCredentials adalah pesan tunggal untuk email tidak ada maupun
// password salah. Membedakannya akan membocorkan email mana yang terdaftar.
var ErrInvalidCredentials = shared.New(
	"INVALID_CREDENTIALS",
	"Email atau kata sandi salah.",
	shared.ErrUnauthorized,
)

// Login memvalidasi kredensial dan (bila 2FA aktif) menahan sesi di langkah kedua.
func (s *Service) Login(ctx context.Context, cmd LoginCommand) (*LoginResult, error) {
	email := user.NormalizeEmail(cmd.Email)
	if email == "" || cmd.Password == "" {
		return nil, ErrInvalidCredentials
	}

	u, err := s.deps.DB.Users().GetByEmail(ctx, email)
	if err != nil {
		if shared.Is(err, shared.ErrNotFound) {
			// Tetap jalankan verifikasi dummy agar waktu respons tidak
			// membedakan "email tidak ada" dari "password salah" (timing attack).
			s.verifyDummy(cmd.Password)
			_ = s.deps.Audit.Record(ctx, s.auditEntry(audit.ActionLoginFailed, cmd.Context).
				WithChanges(nil, map[string]any{"email": email, "reason": "not_found"}))
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	now := s.now()
	if err := u.CanLogin(now); err != nil {
		// Dua sebab berbeda harus terlihat berbeda di audit: akun terkunci
		// karena percobaan gagal, versus akun yang memang belum aktif.
		action := audit.ActionLoginBlocked
		if u.LockedUntil != nil && now.Before(*u.LockedUntil) {
			action = audit.ActionLoginLocked
		}
		_ = s.deps.Audit.Record(ctx, s.auditEntry(action, cmd.Context).
			WithEntity("user", u.ID).
			WithChanges(nil, map[string]any{"status": string(u.Status)}))
		return nil, err
	}

	if !u.HasPassword() {
		// Akun OAuth-only tidak punya password; arahkan ke jalur Google.
		_ = s.deps.Audit.Record(ctx, s.auditEntry(audit.ActionLoginFailed, cmd.Context).
			WithChanges(nil, map[string]any{"email": email, "reason": "no_password"}))
		return nil, shared.New(
			"PASSWORD_NOT_SET",
			"Akun ini dibuat melalui Google. Masuk dengan tombol Google.",
			shared.ErrUnauthorized,
		)
	}

	ok, err := cryptox.VerifyPassword(cmd.Password, u.PasswordHash)
	if err != nil || !ok {
		locked := u.RegisterFailure(now)
		if uerr := s.deps.DB.Users().UpdateLoginState(ctx, u); uerr != nil {
			return nil, uerr
		}
		action := audit.ActionLoginFailed
		if locked {
			action = audit.ActionLoginLocked
		}
		_ = s.deps.Audit.Record(ctx, s.auditEntry(action, cmd.Context).
			WithEntity("user", u.ID).
			WithChanges(nil, map[string]any{"failed_count": u.FailedLoginCount}))
		return nil, ErrInvalidCredentials
	}

	// Hash lama dengan parameter lebih lemah dinaikkan diam-diam saat login —
	// pengguna tidak perlu diminta mengganti password.
	if cryptox.NeedsRehash(u.PasswordHash, s.deps.Params) {
		if newHash, err := cryptox.HashPassword(cmd.Password, s.deps.Params); err == nil {
			u.PasswordHash = newHash
			nowPtr := now
			u.PasswordChangedAt = &nowPtr
			_ = s.deps.DB.Users().UpdatePassword(ctx, u)
		}
	}

	u.RegisterSuccess(now)
	if err := s.deps.DB.Users().UpdateLoginState(ctx, u); err != nil {
		return nil, err
	}

	// 2FA aktif → jangan terbitkan sesi; minta langkah kedua.
	if u.TwoFAEnabled && len(u.TwoFASecretEnc) > 0 {
		mfaToken, err := s.deps.MFA.Issue(u.ID, tokenx.MFAPurposeLogin, now)
		if err != nil {
			return nil, err
		}
		_ = s.deps.Audit.Record(ctx, s.auditEntry("auth.mfa_required", cmd.Context).
			WithEntity("user", u.ID))
		return &LoginResult{RequiresMFA: true, MFAToken: mfaToken}, nil
	}

	auth, err := s.startSession(ctx, u, "", cmd.DeviceName, cmd.Context, audit.ActionLogin)
	if err != nil {
		return nil, err
	}
	return &LoginResult{Auth: auth}, nil
}

// VerifyTwoFactorLogin menyelesaikan login yang tertahan 2FA.
func (s *Service) VerifyTwoFactorLogin(ctx context.Context, mfaToken, code, deviceName string, meta ContextInfo) (*AuthResult, error) {
	now := s.now()
	userID, err := s.deps.MFA.Verify(mfaToken, tokenx.MFAPurposeLogin, now)
	if err != nil {
		return nil, shared.New("MFA_TOKEN_INVALID",
			"Sesi 2FA tidak valid atau sudah kedaluwarsa. Silakan masuk lagi.",
			shared.ErrUnauthorized)
	}

	u, err := s.deps.DB.Users().GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !u.TwoFAEnabled || len(u.TwoFASecretEnc) == 0 {
		return nil, shared.New("MFA_NOT_ENABLED", "2FA tidak aktif pada akun ini.", shared.ErrValidation)
	}

	if err := s.checkSecondFactor(ctx, u, code, now); err != nil {
		_ = s.deps.Audit.Record(ctx, s.auditEntry(audit.ActionTwoFAFailed, meta).
			WithEntity("user", u.ID))
		return nil, err
	}

	return s.startSession(ctx, u, "", deviceName, meta, audit.ActionLogin)
}

// checkSecondFactor menerima kode TOTP ATAU kode pemulihan. Kode pemulihan
// dihanguskan saat dipakai dan dicatat berbeda di audit supaya aktivitas yang
// tidak biasa ini terlihat.
func (s *Service) checkSecondFactor(ctx context.Context, u *user.User, code string, now time.Time) error {
	secret, err := s.deps.Cipher.Decrypt(u.TwoFASecretEnc)
	if err == nil {
		if tokenx.ValidateTOTP(s.deps.TOTP, string(secret), code, now) == nil {
			return nil
		}
	}

	// Jatuh ke kode pemulihan.
	normalized := strings.TrimSpace(code)
	if normalized == "" {
		return errInvalidOTP
	}
	codes, err := s.deps.DB.Tokens().ListRecoveryCodes(ctx, u.ID)
	if err != nil {
		return err
	}
	for _, c := range codes {
		if user.MatchRecoveryCode(normalized, c.CodeHash) {
			ok, err := s.deps.DB.Tokens().ConsumeRecoveryCode(ctx, c.ID, now)
			if err != nil {
				return err
			}
			if ok {
				_ = s.deps.Audit.Record(ctx, audit.New(audit.ActionRecoveryUsed, u.ID, "").
					WithEntity("user", u.ID))
				return nil
			}
		}
	}
	return errInvalidOTP
}

// errInvalidOTP adalah pesan tunggal untuk kode 2FA salah — tidak membedakan
// TOTP dan kode pemulihan.
var errInvalidOTP = shared.New(
	"INVALID_OTP",
	"Kode verifikasi salah. Periksa aplikasi authenticator Anda.",
	shared.ErrUnauthorized,
)

// startSession membuat sesi perangkat + menerbitkan access & refresh token.
func (s *Service) startSession(ctx context.Context, u *user.User, orgID, deviceName string, meta ContextInfo, auditAction string) (*AuthResult, error) {
	now := s.now()

	raw, hash, err := user.GenerateOpaqueToken()
	if err != nil {
		return nil, err
	}

	memberships, err := s.deps.DB.Orgs().ListForUser(ctx, u.ID)
	if err != nil {
		return nil, err
	}

	// Organisasi aktif: yang diminta bila user memang anggotanya, jika tidak
	// organisasi pertama (paling lama). Tidak ada cara memilih organisasi yang
	// bukan milik user.
	activeOrg, activeRole := pickActiveOrg(memberships, orgID)

	sess := user.NewSession(u.ID, hash, deviceName, meta.UserAgent, meta.IP, user.RefreshTokenTTL)
	if err := s.deps.DB.Sessions().Create(ctx, sess); err != nil {
		return nil, err
	}

	access, err := s.deps.JWT.Issue(u.ID, sess.ID, u.Email, activeOrg, string(activeRole), now)
	if err != nil {
		return nil, err
	}

	_ = s.deps.Audit.Record(ctx, s.auditEntry(auditAction, meta).
		WithEntity("user", u.ID).
		WithChanges(nil, map[string]any{"session_id": sess.ID, "device": sess.DeviceLabel()}))

	return &AuthResult{
		Session: IssuedSession{
			AccessToken:  access,
			RefreshToken: raw,
			ExpiresIn:    int(s.deps.JWT.TTL().Seconds()),
			SessionID:    sess.ID,
		},
		User:          u,
		Organizations: memberships,
		ActiveOrgID:   activeOrg,
		ActiveRole:    activeRole,
	}, nil
}

// pickActiveOrg memilih organisasi aktif dari keanggotaan yang benar-benar
// dimiliki user. Permintaan orgID diabaikan bila user bukan anggotanya.
func pickActiveOrg(memberships []organization.MembershipInfo, requested string) (string, organization.RoleCode) {
	if len(memberships) == 0 {
		return "", ""
	}
	if requested != "" {
		for _, m := range memberships {
			if m.Organization.ID == requested {
				return m.Organization.ID, m.RoleCode
			}
		}
	}
	return memberships[0].Organization.ID, memberships[0].RoleCode
}

// verifyDummy menjalankan verifikasi hash palsu agar waktu respons login tidak
// membocorkan keberadaan akun.
func (s *Service) verifyDummy(password string) {
	const dummy = "$argon2id$v=19$m=65536,t=3,p=2$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	_, _ = cryptox.VerifyPassword(password, dummy)
}
