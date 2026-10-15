package auth

import (
	"context"
	"strings"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/user"
	"github.com/angkut/angkut/apps/internal/infrastructure/cryptox"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
)

// ForgotPassword membuat token reset dan mengirimkannya lewat email.
//
// SELALU mengembalikan sukses, termasuk untuk email yang tidak terdaftar.
// Perbedaan respons akan mengubah endpoint ini menjadi alat untuk memetakan
// email mana yang punya akun.
func (s *Service) ForgotPassword(ctx context.Context, email string, meta ContextInfo) error {
	email = user.NormalizeEmail(email)
	if email == "" {
		return shared.Validation(map[string]string{"email": "wajib diisi."})
	}

	u, err := s.deps.DB.Users().GetByEmail(ctx, email)
	if err != nil {
		// Tidak ditemukan → diam. Tetap catat agar terlihat bila seseorang
		// menyisir daftar email.
		_ = s.deps.Audit.Record(ctx, s.auditEntry(audit.ActionPasswordResetReq, meta).
			WithChanges(nil, map[string]any{"email": email, "found": false}))
		return nil
	}

	rawToken, tokenHash, err := user.GenerateOpaqueToken()
	if err != nil {
		return err
	}
	t := user.NewPasswordResetToken(u.ID, tokenHash, meta.IP)
	if err := s.deps.DB.Tokens().CreatePasswordReset(ctx, t); err != nil {
		return err
	}

	_ = s.deps.Audit.Record(ctx, s.auditEntry(audit.ActionPasswordResetReq, meta).
		WithEntity("user", u.ID).
		WithChanges(nil, map[string]any{"email": email, "found": true}))

	if s.deps.Mailer != nil {
		_ = s.deps.Mailer.SendPasswordReset(ctx, u.Email, u.Name, rawToken)
	}
	return nil
}

// ResetPassword mengganti password memakai token sekali-pakai.
//
// Dampak keamanan: seluruh sesi dicabut. Siapa pun yang memegang token reset
// berarti sudah membuktikan akses ke email — sesi lama tidak boleh tetap hidup.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string, meta ContextInfo) error {
	tokenHash := user.HashToken(strings.TrimSpace(token))
	if strings.TrimSpace(token) == "" {
		return user.ErrTokenInvalid
	}
	if err := user.ValidatePassword(newPassword); err != nil {
		return err
	}

	hash, err := cryptox.HashPassword(newPassword, s.deps.Params)
	if err != nil {
		return err
	}

	return s.deps.Tx.InTx(ctx, func(q database.Querier) error {
		st := s.deps.DB.WithTx(q)
		now := s.now()

		t, err := st.Tokens().GetPasswordReset(ctx, tokenHash)
		if err != nil {
			return err
		}
		if !t.IsUsable(now) {
			return user.ErrTokenInvalid
		}

		u, err := st.Users().GetByID(ctx, t.UserID)
		if err != nil {
			return err
		}

		u.PasswordHash = hash
		u.PasswordChangedAt = &now
		u.FailedLoginCount = 0
		u.LockedUntil = nil
		if err := st.Users().UpdatePassword(ctx, u); err != nil {
			return err
		}
		if err := st.Tokens().UsePasswordReset(ctx, t.ID, now); err != nil {
			return err
		}
		// Cabut semua sesi: password lama tidak boleh dipakai lagi di mana pun.
		revoked, err := st.Sessions().RevokeAll(ctx, u.ID, now)
		if err != nil {
			return err
		}
		return st.Audit().Record(ctx, s.auditEntry(audit.ActionPasswordReset, meta).
			WithEntity("user", u.ID).
			WithChanges(nil, map[string]any{"revoked_sessions": revoked}))
	})
}

// ChangePassword mengganti password untuk pengguna yang sudah login (wajib
// membuktikan password lama).
func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword, currentSessionID string, meta ContextInfo) error {
	if err := user.ValidatePassword(newPassword); err != nil {
		return err
	}

	u, err := s.deps.DB.Users().GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !u.HasPassword() {
		return shared.New("PASSWORD_NOT_SET",
			"Akun ini belum memiliki kata sandi. Gunakan reset kata sandi untuk membuatnya.",
			shared.ErrValidation)
	}

	ok, err := cryptox.VerifyPassword(currentPassword, u.PasswordHash)
	if err != nil || !ok {
		_ = s.deps.Audit.Record(ctx, s.auditEntry("auth.password_change_failed", meta).
			WithEntity("user", u.ID))
		return shared.New("INVALID_PASSWORD", "Kata sandi saat ini salah.", shared.ErrValidation)
	}

	hash, err := cryptox.HashPassword(newPassword, s.deps.Params)
	if err != nil {
		return err
	}

	return s.deps.Tx.InTx(ctx, func(q database.Querier) error {
		st := s.deps.DB.WithTx(q)
		now := s.now()

		u.PasswordHash = hash
		u.PasswordChangedAt = &now
		if err := st.Users().UpdatePassword(ctx, u); err != nil {
			return err
		}

		// Cabut semua sesi LAIN, sisakan sesi ini supaya pengguna tidak
		// ter-logout dari perangkat yang sedang dipakai.
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
		return st.Audit().Record(ctx, s.auditEntry(audit.ActionPasswordChanged, meta).
			WithEntity("user", u.ID).
			WithChanges(nil, map[string]any{"revoked_sessions": revoked}))
	})
}
