package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/user"
)

// TokenRepo adalah implementasi user.TokenRepository.
type TokenRepo struct {
	q Querier
}

// NewTokenRepo membuat repository token sekali-pakai.
func NewTokenRepo(q Querier) *TokenRepo { return &TokenRepo{q: q} }

// WithTx menjalankan repository di transaksi tertentu.
func (r *TokenRepo) WithTx(q Querier) *TokenRepo { return &TokenRepo{q: q} }

// CreatePasswordReset menyimpan token reset baru.
//
// Token aktif lama dihapus lebih dulu karena ada unique index parsial
// (satu token aktif per user) — meminta tautan baru harus membatalkan yang lama,
// sehingga tautan lama tidak bisa dipakai lagi.
func (r *TokenRepo) CreatePasswordReset(ctx context.Context, t *user.PasswordResetToken) error {
	if _, err := r.q.Exec(ctx,
		`DELETE FROM password_reset_tokens WHERE user_id=$1 AND used_at IS NULL`, t.UserID); err != nil {
		return err
	}
	_, err := r.q.Exec(ctx, `
		INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, requested_ip, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		t.ID, t.UserID, t.TokenHash, t.ExpiresAt, nullIfEmpty(t.RequestedIP), t.CreatedAt)
	return err
}

// GetPasswordReset mencari token reset berdasarkan hash.
func (r *TokenRepo) GetPasswordReset(ctx context.Context, tokenHash string) (*user.PasswordResetToken, error) {
	var t user.PasswordResetToken
	err := r.q.QueryRow(ctx, `
		SELECT id, user_id, token_hash, expires_at, used_at,
		       COALESCE(requested_ip::text, ''), created_at
		  FROM password_reset_tokens WHERE token_hash=$1`, tokenHash).
		Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.RequestedIP, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrTokenInvalid
		}
		return nil, err
	}
	return &t, nil
}

// UsePasswordReset menandai token terpakai. Kondisi `used_at IS NULL` membuat
// operasi ini idempoten-aman: dua request paralel tidak dapat sama-sama sukses.
func (r *TokenRepo) UsePasswordReset(ctx context.Context, id string, now time.Time) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE password_reset_tokens SET used_at=$2 WHERE id=$1 AND used_at IS NULL`, id, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return user.ErrTokenInvalid
	}
	return nil
}

// CreateEmailVerification menyimpan token verifikasi email baru (mengganti yang lama).
func (r *TokenRepo) CreateEmailVerification(ctx context.Context, t *user.EmailVerificationToken) error {
	if _, err := r.q.Exec(ctx,
		`DELETE FROM email_verification_tokens WHERE user_id=$1 AND used_at IS NULL`, t.UserID); err != nil {
		return err
	}
	_, err := r.q.Exec(ctx, `
		INSERT INTO email_verification_tokens (id, user_id, email, token_hash, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		t.ID, t.UserID, t.Email, t.TokenHash, t.ExpiresAt, t.CreatedAt)
	return err
}

// GetEmailVerification mencari token verifikasi berdasarkan hash.
func (r *TokenRepo) GetEmailVerification(ctx context.Context, tokenHash string) (*user.EmailVerificationToken, error) {
	var t user.EmailVerificationToken
	err := r.q.QueryRow(ctx, `
		SELECT id, user_id, email, token_hash, expires_at, used_at, created_at
		  FROM email_verification_tokens WHERE token_hash=$1`, tokenHash).
		Scan(&t.ID, &t.UserID, &t.Email, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrTokenInvalid
		}
		return nil, err
	}
	return &t, nil
}

// UseEmailVerification menandai token verifikasi terpakai.
func (r *TokenRepo) UseEmailVerification(ctx context.Context, id string, now time.Time) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE email_verification_tokens SET used_at=$2 WHERE id=$1 AND used_at IS NULL`, id, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return user.ErrTokenInvalid
	}
	return nil
}

// ReplaceRecoveryCodes mengganti seluruh kode pemulihan user.
func (r *TokenRepo) ReplaceRecoveryCodes(ctx context.Context, userID string, codes []user.RecoveryCode) error {
	if _, err := r.q.Exec(ctx, `DELETE FROM twofa_recovery_codes WHERE user_id=$1`, userID); err != nil {
		return err
	}
	for _, c := range codes {
		if _, err := r.q.Exec(ctx, `
			INSERT INTO twofa_recovery_codes (id, user_id, code_hash, created_at)
			VALUES ($1,$2,$3, now())`, c.ID, c.UserID, c.CodeHash); err != nil {
			return err
		}
	}
	return nil
}

// ListRecoveryCodes mengembalikan kode pemulihan yang belum terpakai.
func (r *TokenRepo) ListRecoveryCodes(ctx context.Context, userID string) ([]user.RecoveryCode, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, user_id, code_hash, used_at FROM twofa_recovery_codes
		 WHERE user_id=$1 AND used_at IS NULL`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []user.RecoveryCode
	for rows.Next() {
		var c user.RecoveryCode
		if err := rows.Scan(&c.ID, &c.UserID, &c.CodeHash, &c.UsedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ConsumeRecoveryCode menandai satu kode terpakai; false bila sudah dipakai
// atau tidak ada (mencegah pemakaian ganda saat dua request bersamaan).
func (r *TokenRepo) ConsumeRecoveryCode(ctx context.Context, codeID string, now time.Time) (bool, error) {
	tag, err := r.q.Exec(ctx,
		`UPDATE twofa_recovery_codes SET used_at=$2 WHERE id=$1 AND used_at IS NULL`, codeID, now)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ─── OAuth ────────────────────────────────────────────────────────────────

// OAuthRepo adalah implementasi user.OAuthRepository.
type OAuthRepo struct {
	q Querier
}

// NewOAuthRepo membuat repository identitas OAuth.
func NewOAuthRepo(q Querier) *OAuthRepo { return &OAuthRepo{q: q} }

// WithTx menjalankan repository di transaksi tertentu.
func (r *OAuthRepo) WithTx(q Querier) *OAuthRepo { return &OAuthRepo{q: q} }

// FindUserID mencari user yang tertaut ke identitas penyedia.
func (r *OAuthRepo) FindUserID(ctx context.Context, provider, providerUserID string) (string, error) {
	var userID string
	err := r.q.QueryRow(ctx,
		`SELECT user_id FROM user_oauth_accounts WHERE provider=$1 AND provider_user_id=$2`,
		provider, providerUserID).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", shared.NotFound("OAUTH_NOT_LINKED", "Akun belum tertaut.")
		}
		return "", err
	}
	return userID, nil
}

// Link menautkan identitas penyedia ke akun.
func (r *OAuthRepo) Link(ctx context.Context, userID, provider, providerUserID, email string) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO user_oauth_accounts (id, user_id, provider, provider_user_id, email)
		VALUES (gen_random_uuid(), $1,$2,$3,$4)
		ON CONFLICT (provider, provider_user_id) DO NOTHING`,
		userID, provider, providerUserID, nullIfEmpty(email))
	return err
}

// TouchLogin memperbarui waktu login terakhir penyedia.
func (r *OAuthRepo) TouchLogin(ctx context.Context, provider, providerUserID string, now time.Time) error {
	_, err := r.q.Exec(ctx,
		`UPDATE user_oauth_accounts SET last_login_at=$3 WHERE provider=$1 AND provider_user_id=$2`,
		provider, providerUserID, now)
	return err
}
