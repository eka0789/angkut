package auth

import (
	"context"
	"strings"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/organization"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/user"
	"github.com/angkut/angkut/apps/internal/infrastructure/cryptox"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
)

// RegisterCommand adalah permintaan pendaftaran.
type RegisterCommand struct {
	Name             string
	Email            string
	Password         string
	OrganizationName string
	Context          ContextInfo
}

// RegisterResult memuat user + organisasi yang baru dibuat.
type RegisterResult struct {
	User         *user.User
	Organization *organization.Organization
}

// Register membuat akun sekaligus organisasi pertama (user menjadi owner).
//
// Satu transaksi: user + organisasi + keanggotaan owner + token verifikasi.
// Setengah jadi bukan pilihan — organisasi tanpa owner atau token tanpa user
// sama-sama tidak boleh ada.
func (s *Service) Register(ctx context.Context, cmd RegisterCommand) (*RegisterResult, error) {
	email := user.NormalizeEmail(cmd.Email)
	if email == "" {
		return nil, shared.Validation(map[string]string{"email": "wajib diisi."})
	}
	if err := user.ValidateName(cmd.Name); err != nil {
		return nil, err
	}
	if err := user.ValidatePassword(cmd.Password); err != nil {
		return nil, err
	}
	if err := organization.ValidateName(cmd.OrganizationName); err != nil {
		return nil, shared.Validation(map[string]string{"organization_name": "minimal 2 karakter."})
	}

	hash, err := cryptox.HashPassword(cmd.Password, s.deps.Params)
	if err != nil {
		return nil, err
	}

	u := user.NewUser(email, cmd.Name)
	u.PasswordHash = hash

	org := organization.NewOrganization(cmd.OrganizationName, u.ID)
	owner := organization.NewOwnerMember(org.ID, u.ID)

	// Token verifikasi dibuat sebelum transaksi agar kegagalan acak tidak
	// membatalkan pekerjaan database yang sudah benar.
	rawToken, tokenHash, err := user.GenerateOpaqueToken()
	if err != nil {
		return nil, err
	}
	verify := user.NewEmailVerificationToken(u.ID, u.Email, tokenHash)

	err = s.deps.Tx.InTx(ctx, func(q database.Querier) error {
		st := s.deps.DB.WithTx(q)
		if err := st.Users().Create(ctx, u); err != nil {
			return err
		}
		if err := st.Orgs().CreateWithOwner(ctx, org, owner); err != nil {
			return err
		}
		if err := st.Tokens().CreateEmailVerification(ctx, verify); err != nil {
			return err
		}
		return st.Audit().Record(ctx, s.auditEntry(audit.ActionRegister, cmd.Context).
			WithEntity("user", u.ID).
			WithChanges(nil, map[string]any{
				"email": u.Email, "organization_id": org.ID, "slug": org.Slug,
			}))
	})
	if err != nil {
		return nil, err
	}

	// Email dikirim SETELAH commit: kalau pengiriman gagal, akun tetap ada dan
	// pengguna bisa meminta kirim ulang. Kegagalan email tidak boleh membatalkan
	// pendaftaran yang sudah sah.
	if s.deps.Mailer != nil {
		_ = s.deps.Mailer.SendEmailVerification(ctx, u.Email, u.Name, rawToken)
	}

	return &RegisterResult{User: u, Organization: org}, nil
}

// VerifyEmail menukar token verifikasi menjadi akun aktif.
func (s *Service) VerifyEmail(ctx context.Context, token string, meta ContextInfo) error {
	hash := user.HashToken(strings.TrimSpace(token))
	if hash == "" || strings.TrimSpace(token) == "" {
		return user.ErrTokenInvalid
	}

	return s.deps.Tx.InTx(ctx, func(q database.Querier) error {
		st := s.deps.DB.WithTx(q)

		t, err := st.Tokens().GetEmailVerification(ctx, hash)
		if err != nil {
			return err
		}
		now := s.now()
		if !t.IsUsable(now) {
			return user.ErrTokenInvalid
		}

		u, err := st.Users().GetByID(ctx, t.UserID)
		if err != nil {
			return err
		}

		// Sudah terverifikasi sebelumnya: token dihanguskan, tetapi jangan
		// menganggap ini sukses baru (agar audit tetap jujur).
		if u.IsEmailVerified() {
			return st.Tokens().UseEmailVerification(ctx, t.ID, now)
		}

		u.MarkEmailVerified(now)
		if err := st.Users().MarkEmailVerified(ctx, u); err != nil {
			return err
		}
		if err := st.Tokens().UseEmailVerification(ctx, t.ID, now); err != nil {
			return err
		}
		return st.Audit().Record(ctx, audit.New(audit.ActionEmailVerified, u.ID, "").
			WithRequest(meta.IP, meta.UserAgent, meta.RequestID).
			WithEntity("user", u.ID))
	})
}

// ResendVerification mengirim ulang tautan verifikasi.
//
// Selalu mengembalikan sukses (tanpa membocorkan apakah email terdaftar),
// kecuali email kosong yang jelas-jelas salah.
func (s *Service) ResendVerification(ctx context.Context, email string, meta ContextInfo) error {
	email = user.NormalizeEmail(email)
	if email == "" {
		return shared.Validation(map[string]string{"email": "wajib diisi."})
	}

	u, err := s.deps.DB.Users().GetByEmail(ctx, email)
	if err != nil || u == nil || u.IsEmailVerified() {
		// Diam: jangan ungkap keberadaan atau status akun.
		return nil
	}

	rawToken, tokenHash, err := user.GenerateOpaqueToken()
	if err != nil {
		return err
	}
	t := user.NewEmailVerificationToken(u.ID, u.Email, tokenHash)
	if err := s.deps.DB.Tokens().CreateEmailVerification(ctx, t); err != nil {
		return err
	}
	if s.deps.Mailer != nil {
		_ = s.deps.Mailer.SendEmailVerification(ctx, u.Email, u.Name, rawToken)
	}
	return nil
}

// GetProfile mengambil data pengguna yang sedang login.
func (s *Service) GetProfile(ctx context.Context, userID string) (*user.User, error) {
	return s.deps.DB.Users().GetByID(ctx, userID)
}

// UpdateProfile menyimpan perubahan profil pengguna yang sedang login.
func (s *Service) UpdateProfile(ctx context.Context, userID string, name, phone, avatarURL, locale, timezone string, meta ContextInfo) (*user.User, error) {
	if err := user.ValidateName(name); err != nil {
		return nil, err
	}
	u, err := s.deps.DB.Users().GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	before := map[string]any{"name": u.Name, "phone": u.Phone}
	u.Name = strings.TrimSpace(name)
	if phone != "" {
		normalized, err := shared.NormalizePhone(phone)
		if err != nil {
			return nil, shared.Validation(map[string]string{"phone": "format nomor tidak valid."})
		}
		u.Phone = normalized
	}
	u.AvatarURL = strings.TrimSpace(avatarURL)
	if locale != "" {
		u.Locale = locale
	}
	if timezone != "" {
		u.Timezone = timezone
	}

	if err := s.deps.DB.Users().UpdateProfile(ctx, u); err != nil {
		return nil, err
	}
	_ = s.deps.Audit.Record(ctx, audit.New("user.profile_updated", u.ID, "").
		WithRequest(meta.IP, meta.UserAgent, meta.RequestID).
		WithEntity("user", u.ID).
		WithChanges(before, map[string]any{"name": u.Name, "phone": u.Phone}))
	return u, nil
}
