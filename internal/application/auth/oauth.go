package auth

import (
	"context"
	"errors"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/organization"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/user"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
)

// ProviderGoogle adalah nilai `provider` di user_oauth_accounts.
const ProviderGoogle = "google"

// GoogleUser adalah identitas hasil verifikasi Google.
type GoogleUser struct {
	Subject       string // klaim `sub` — stabil dan tidak berubah walau email berganti
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

// GoogleOAuth adalah port integrasi Google. Implementasinya di infrastructure;
// saat kredensial belum diisi, dipakai mock yang menolak alih-alih diam-diam
// mengizinkan login palsu.
type GoogleOAuth interface {
	// AuthCodeURL menyusun URL consent dengan state anti-CSRF.
	AuthCodeURL(state string) string
	// Exchange menukar authorization code menjadi identitas pengguna.
	Exchange(ctx context.Context, code string) (*GoogleUser, error)
	// Enabled melaporkan apakah kredensial Google terkonfigurasi.
	Enabled() bool
}

// OAuthLogin menyelesaikan login lewat Google.
//
// Tiga kemungkinan, semuanya harus aman:
//  1. Identitas sudah tertaut → login biasa.
//  2. Email sudah punya akun → tautkan (account linking), TIDAK membuat akun kedua.
//  3. Belum ada → buat akun baru tanpa password + organisasi pertama.
func (s *Service) OAuthLogin(ctx context.Context, providerUser *GoogleUser, deviceName string, meta ContextInfo) (*AuthResult, error) {
	if providerUser == nil || providerUser.Subject == "" {
		return nil, shared.New("OAUTH_FAILED", "Gagal memverifikasi akun Google.", shared.ErrUnauthorized)
	}
	if !providerUser.EmailVerified {
		// Email Google yang belum terverifikasi tidak boleh dipercaya untuk
		// linking — bisa dipakai mengambil alih akun orang lain.
		return nil, shared.New("OAUTH_EMAIL_UNVERIFIED",
			"Email Google Anda belum terverifikasi.", shared.ErrForbidden)
	}

	email := user.NormalizeEmail(providerUser.Email)

	// 1. Sudah tertaut?
	if userID, err := s.deps.DB.OAuth().FindUserID(ctx, ProviderGoogle, providerUser.Subject); err == nil {
		u, err := s.deps.DB.Users().GetByID(ctx, userID)
		if err != nil {
			return nil, err
		}
		now := s.now()
		if err := u.CanLogin(now); err != nil {
			return nil, err
		}
		u.RegisterSuccess(now)
		_ = s.deps.DB.Users().UpdateLoginState(ctx, u)
		_ = s.deps.DB.OAuth().TouchLogin(ctx, ProviderGoogle, providerUser.Subject, now)
		return s.startSession(ctx, u, "", deviceName, meta, audit.ActionOAuthLogin)
	} else if !shared.Is(err, shared.ErrNotFound) {
		return nil, err
	}

	// 2. Akun dengan email sama sudah ada? Tautkan.
	if u, err := s.deps.DB.Users().GetByEmail(ctx, email); err == nil {
		now := s.now()
		if err := u.CanLogin(now); err != nil {
			return nil, err
		}
		if err := s.deps.DB.OAuth().Link(ctx, u.ID, ProviderGoogle, providerUser.Subject, email); err != nil {
			return nil, err
		}
		// Email Google yang terverifikasi sekaligus memverifikasi akun lokal.
		if !u.IsEmailVerified() {
			u.MarkEmailVerified(now)
			if err := s.deps.DB.Users().MarkEmailVerified(ctx, u); err != nil {
				return nil, err
			}
		}
		u.RegisterSuccess(now)
		_ = s.deps.DB.Users().UpdateLoginState(ctx, u)
		_ = s.deps.Audit.Record(ctx, s.auditEntry("auth.oauth_linked", meta).
			WithEntity("user", u.ID).
			WithChanges(nil, map[string]any{"provider": ProviderGoogle}))
		return s.startSession(ctx, u, "", deviceName, meta, audit.ActionOAuthLogin)
	} else if !shared.Is(err, shared.ErrNotFound) {
		return nil, err
	}

	// 3. Akun baru. Nama organisasi default diturunkan dari nama pengguna.
	u := user.NewUser(email, displayNameOr(providerUser))
	u.AvatarURL = providerUser.Picture
	u.MarkEmailVerified(s.now()) // Google sudah memverifikasi emailnya
	org := organization.NewOrganization(nameOrOrg(providerUser), u.ID)
	owner := organization.NewOwnerMember(org.ID, u.ID)

	err := s.deps.Tx.InTx(ctx, func(q database.Querier) error {
		st := s.deps.DB.WithTx(q)
		if err := st.Users().Create(ctx, u); err != nil {
			return err
		}
		if err := st.Orgs().CreateWithOwner(ctx, org, owner); err != nil {
			return err
		}
		if err := st.OAuth().Link(ctx, u.ID, ProviderGoogle, providerUser.Subject, email); err != nil {
			return err
		}
		return st.Audit().Record(ctx, s.auditEntry(audit.ActionRegister, meta).
			WithEntity("user", u.ID).
			WithChanges(nil, map[string]any{
				"email": email, "provider": ProviderGoogle, "organization_id": org.ID,
			}))
	})
	if err != nil {
		// Balapan: dua request membuat akun bersamaan. Ambil yang menang.
		var de *shared.Error
		if errors.As(err, &de) && de.Code == "EMAIL_TAKEN" {
			if existing, getErr := s.deps.DB.Users().GetByEmail(ctx, email); getErr == nil {
				_ = s.deps.DB.OAuth().Link(ctx, existing.ID, ProviderGoogle, providerUser.Subject, email)
				return s.startSession(ctx, existing, "", deviceName, meta, audit.ActionOAuthLogin)
			}
		}
		return nil, err
	}

	return s.startSession(ctx, u, "", deviceName, meta, audit.ActionOAuthLogin)
}

func displayNameOr(g *GoogleUser) string {
	if g.Name != "" {
		return g.Name
	}
	return g.Email
}

// nameOrOrg menurunkan nama organisasi awal dari nama pengguna.
func nameOrOrg(g *GoogleUser) string {
	base := g.Name
	if base == "" {
		base = g.Email
	}
	if len(base) > 40 {
		base = base[:40]
	}
	if len(base) < 2 {
		base = "Organisasi Saya"
	}
	return base
}
