package auth

import (
	"context"

	"github.com/angkut/angkut/apps/api/internal/domain/audit"
	"github.com/angkut/angkut/apps/api/internal/domain/shared"
	"github.com/angkut/angkut/apps/api/internal/domain/user"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/cryptox"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/tokenx"
)

// TwoFASetup adalah hasil penyiapan 2FA.
type TwoFASetup struct {
	// Secret ditampilkan sekali agar pengguna dapat memasukkannya manual bila
	// QR tidak terbaca. Setelah ini, hanya versi terenkripsi yang tersimpan.
	Secret        string
	OTPAuthURI    string
	RecoveryCodes []string
}

// SetupTwoFactor membuat secret TOTP + kode pemulihan untuk pengguna yang
// sedang login. 2FA BELUM aktif sampai EnableTwoFactor dipanggil dengan kode
// yang benar — mencegah pengguna terkunci karena salah memindai QR.
func (s *Service) SetupTwoFactor(ctx context.Context, userID string, meta ContextInfo) (*TwoFASetup, error) {
	u, err := s.deps.DB.Users().GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u.TwoFAEnabled {
		return nil, shared.Conflict("TWOFA_ALREADY_ENABLED",
			"2FA sudah aktif. Nonaktifkan terlebih dahulu untuk menyiapkan ulang.")
	}

	secret, uri, err := tokenx.NewTOTPSecret(s.deps.TOTP, u.Email)
	if err != nil {
		return nil, err
	}
	encSecret, err := s.deps.Cipher.Encrypt([]byte(secret))
	if err != nil {
		return nil, err
	}

	plain, codes, err := user.GenerateRecoveryCodes(u.ID)
	if err != nil {
		return nil, err
	}

	err = s.deps.Tx.InTx(ctx, func(q database.Querier) error {
		st := s.deps.DB.WithTx(q)

		// Secret disimpan tetapi twofa_enabled tetap false sampai dikonfirmasi.
		u.TwoFASecretEnc = encSecret
		u.TwoFAEnabled = false
		u.TwoFAConfirmedAt = nil
		if err := st.Users().UpdateTwoFA(ctx, u); err != nil {
			return err
		}
		if err := st.Tokens().ReplaceRecoveryCodes(ctx, u.ID, codes); err != nil {
			return err
		}
		return st.Audit().Record(ctx, s.auditEntry(audit.ActionTwoFASetup, meta).
			WithEntity("user", u.ID))
	})
	if err != nil {
		return nil, err
	}

	return &TwoFASetup{Secret: secret, OTPAuthURI: uri, RecoveryCodes: plain}, nil
}

// EnableTwoFactor mengaktifkan 2FA setelah pengguna membuktikan kode valid.
func (s *Service) EnableTwoFactor(ctx context.Context, userID, code string, meta ContextInfo) error {
	u, err := s.deps.DB.Users().GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.TwoFAEnabled {
		return nil // idempoten: sudah aktif, tidak ada yang perlu dilakukan
	}
	if len(u.TwoFASecretEnc) == 0 {
		return shared.New("TWOFA_NOT_SETUP",
			"2FA belum disiapkan. Jalankan penyiapan terlebih dahulu.", shared.ErrValidation)
	}

	secret, err := s.deps.Cipher.Decrypt(u.TwoFASecretEnc)
	if err != nil {
		return err
	}
	now := s.now()
	if err := tokenx.ValidateTOTP(s.deps.TOTP, string(secret), code, now); err != nil {
		_ = s.deps.Audit.Record(ctx, s.auditEntry(audit.ActionTwoFAFailed, meta).
			WithEntity("user", u.ID).
			WithChanges(nil, map[string]any{"stage": "enable"}))
		return errInvalidOTP
	}

	u.TwoFAEnabled = true
	u.TwoFAConfirmedAt = &now
	if err := s.deps.DB.Users().UpdateTwoFA(ctx, u); err != nil {
		return err
	}

	_ = s.deps.Audit.Record(ctx, s.auditEntry(audit.ActionTwoFAEnabled, meta).
		WithEntity("user", u.ID))
	return nil
}

// DisableTwoFactor mematikan 2FA. Membutuhkan password DAN kode 2FA — tanpa
// keduanya, pencuri yang sudah memegang sesi dapat mematikan proteksi.
func (s *Service) DisableTwoFactor(ctx context.Context, userID, password, code, currentSessionID string, meta ContextInfo) error {
	u, err := s.deps.DB.Users().GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !u.TwoFAEnabled {
		return nil // idempoten
	}

	if !u.HasPassword() {
		return shared.New("PASSWORD_NOT_SET",
			"Akun ini tidak memiliki kata sandi untuk diverifikasi.", shared.ErrValidation)
	}
	ok, err := cryptox.VerifyPassword(password, u.PasswordHash)
	if err != nil || !ok {
		_ = s.deps.Audit.Record(ctx, s.auditEntry("auth.2fa_disable_failed", meta).
			WithEntity("user", u.ID).
			WithChanges(nil, map[string]any{"reason": "bad_password"}))
		return shared.New("INVALID_PASSWORD", "Kata sandi salah.", shared.ErrValidation)
	}

	now := s.now()
	if err := s.checkSecondFactor(ctx, u, code, now); err != nil {
		_ = s.deps.Audit.Record(ctx, s.auditEntry("auth.2fa_disable_failed", meta).
			WithEntity("user", u.ID).
			WithChanges(nil, map[string]any{"reason": "bad_otp"}))
		return err
	}

	return s.deps.Tx.InTx(ctx, func(q database.Querier) error {
		st := s.deps.DB.WithTx(q)

		u.TwoFAEnabled = false
		u.TwoFASecretEnc = nil
		u.TwoFAConfirmedAt = nil
		if err := st.Users().UpdateTwoFA(ctx, u); err != nil {
			return err
		}
		// Kode pemulihan ikut dihapus — tidak boleh ada sisa kredensial 2FA.
		if err := st.Tokens().ReplaceRecoveryCodes(ctx, u.ID, nil); err != nil {
			return err
		}
		// Menurunkan proteksi akun = peristiwa keamanan: cabut sesi lain.
		sessions, err := st.Sessions().ListActiveByUser(ctx, u.ID, now)
		if err != nil {
			return err
		}
		revoked := 0
		for _, sess := range sessions {
			if sess.ID == currentSessionID {
				continue
			}
			if ok, err := st.Sessions().Revoke(ctx, u.ID, sess.ID, now); err != nil {
				return err
			} else if ok {
				revoked++
			}
		}
		return st.Audit().Record(ctx, s.auditEntry(audit.ActionTwoFADisabled, meta).
			WithEntity("user", u.ID).
			WithChanges(nil, map[string]any{"revoked_sessions": revoked}))
	})
}

// RegenerateRecoveryCodes membuat ulang kode pemulihan (membutuhkan kode 2FA
// valid). Berguna saat kode lama hilang atau sudah banyak terpakai.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID, code string, meta ContextInfo) ([]string, error) {
	u, err := s.deps.DB.Users().GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !u.TwoFAEnabled {
		return nil, shared.New("TWOFA_NOT_ENABLED", "2FA belum aktif.", shared.ErrValidation)
	}
	if err := s.checkSecondFactor(ctx, u, code, s.now()); err != nil {
		return nil, err
	}

	plain, codes, err := user.GenerateRecoveryCodes(u.ID)
	if err != nil {
		return nil, err
	}
	if err := s.deps.DB.Tokens().ReplaceRecoveryCodes(ctx, u.ID, codes); err != nil {
		return nil, err
	}
	_ = s.deps.Audit.Record(ctx, s.auditEntry("auth.2fa_recovery_regenerated", meta).
		WithEntity("user", u.ID))
	return plain, nil
}

// TwoFAStatus melaporkan status 2FA pengguna (untuk halaman keamanan).
type TwoFAStatus struct {
	Enabled        bool
	ConfirmedAt    string
	RemainingCodes int
}

// StatusTwoFactor mengembalikan ringkasan 2FA. Tidak ada rahasia yang bocor.
func (s *Service) StatusTwoFactor(ctx context.Context, userID string) (*TwoFAStatus, error) {
	u, err := s.deps.DB.Users().GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	st := &TwoFAStatus{Enabled: u.TwoFAEnabled}
	if u.TwoFAConfirmedAt != nil {
		st.ConfirmedAt = u.TwoFAConfirmedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if u.TwoFAEnabled {
		codes, err := s.deps.DB.Tokens().ListRecoveryCodes(ctx, userID)
		if err != nil {
			return nil, err
		}
		st.RemainingCodes = len(codes)
	}
	return st, nil
}
