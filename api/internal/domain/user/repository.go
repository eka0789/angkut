package user

import (
	"context"
	"time"
)

// Repository adalah port penyimpanan identitas. Implementasinya di
// infrastructure/database — domain tidak pernah tahu SQL.
//
// Catatan penting: tabel `users` bersifat GLOBAL (bukan per organisasi), jadi
// query di sini sengaja TIDAK difilter organization_id. Isolasi tenant berlaku
// pada tabel domain bisnis, bukan pada identitas akun.
type Repository interface {
	// Create menyimpan pengguna baru. Mengembalikan ErrEmailTaken bila email
	// sudah dipakai akun aktif (constraint DB, bukan sekadar pre-check).
	Create(ctx context.Context, u *User) error

	// GetByID mengambil pengguna; ErrNotFound bila tidak ada.
	GetByID(ctx context.Context, id string) (*User, error)

	// GetByEmail mencari berdasarkan email (sudah dinormalisasi pemanggil).
	GetByEmail(ctx context.Context, email string) (*User, error)

	// UpdateProfile menyimpan perubahan nama/telepon/avatar/locale/timezone.
	UpdateProfile(ctx context.Context, u *User) error

	// UpdatePassword menyimpan hash baru dan menandai waktu perubahan.
	UpdatePassword(ctx context.Context, u *User) error

	// UpdateLoginState menyimpan penghitung gagal / lock / last_login.
	UpdateLoginState(ctx context.Context, u *User) error

	// MarkEmailVerified menyimpan status verifikasi.
	MarkEmailVerified(ctx context.Context, u *User) error

	// UpdateTwoFA menyimpan status & secret 2FA.
	UpdateTwoFA(ctx context.Context, u *User) error
}

// SessionRepository adalah port penyimpanan sesi perangkat.
type SessionRepository interface {
	// Create menyimpan sesi baru.
	Create(ctx context.Context, s *Session) error

	// GetByID mengambil sesi (dipakai validasi access token & daftar perangkat).
	GetByID(ctx context.Context, id string) (*Session, error)

	// GetByRefreshHash mencari sesi berdasarkan hash refresh token.
	//
	// Implementasi WAJIB mengembalikan baris yang sudah dicabut sekalipun,
	// supaya pemanggil dapat membedakan "token kedaluwarsa" dari "token dipakai
	// ulang" (indikasi pencurian).
	GetByRefreshHash(ctx context.Context, hash string) (*Session, error)

	// Rotate mengganti sesi lama dengan sesi baru dalam SATU transaksi:
	// sesi lama dicabut dan ditautkan ke sesi baru. Ini yang membuat pemakaian
	// ulang token lama terdeteksi.
	Rotate(ctx context.Context, oldSessionID string, next *Session) error

	// ListActiveByUser mengembalikan sesi aktif, terbaru lebih dulu.
	ListActiveByUser(ctx context.Context, userID string, now time.Time) ([]Session, error)

	// Revoke mencabut satu sesi milik user tertentu (org-scope tidak berlaku,
	// tetapi kepemilikan user WAJIB diverifikasi di query).
	Revoke(ctx context.Context, userID, sessionID string, now time.Time) (bool, error)

	// RevokeAll mencabut seluruh sesi aktif user; mengembalikan jumlahnya.
	RevokeAll(ctx context.Context, userID string, now time.Time) (int, error)

	// TouchLastUsed memperbarui waktu pemakaian terakhir.
	TouchLastUsed(ctx context.Context, sessionID string, now time.Time) error
}

// TokenRepository adalah port untuk token sekali-pakai.
type TokenRepository interface {
	// CreatePasswordReset menyimpan token reset (mengganti token aktif lama).
	CreatePasswordReset(ctx context.Context, t *PasswordResetToken) error

	// GetPasswordReset mencari token reset berdasarkan hash.
	GetPasswordReset(ctx context.Context, tokenHash string) (*PasswordResetToken, error)

	// UsePasswordReset menandai token terpakai.
	UsePasswordReset(ctx context.Context, id string, now time.Time) error

	// CreateEmailVerification menyimpan token verifikasi email.
	CreateEmailVerification(ctx context.Context, t *EmailVerificationToken) error

	// GetEmailVerification mencari token verifikasi berdasarkan hash.
	GetEmailVerification(ctx context.Context, tokenHash string) (*EmailVerificationToken, error)

	// UseEmailVerification menandai token verifikasi terpakai.
	UseEmailVerification(ctx context.Context, id string, now time.Time) error

	// ReplaceRecoveryCodes mengganti seluruh kode pemulihan user.
	ReplaceRecoveryCodes(ctx context.Context, userID string, codes []RecoveryCode) error

	// ListRecoveryCodes mengembalikan kode pemulihan yang belum terpakai.
	ListRecoveryCodes(ctx context.Context, userID string) ([]RecoveryCode, error)

	// ConsumeRecoveryCode menandai satu kode terpakai; false bila tidak cocok.
	ConsumeRecoveryCode(ctx context.Context, codeID string, now time.Time) (bool, error)
}

// OAuthRepository adalah port identitas OAuth.
type OAuthRepository interface {
	// FindUserID mencari akun yang tertaut ke identitas penyedia.
	FindUserID(ctx context.Context, provider, providerUserID string) (string, error)

	// Link menautkan identitas penyedia ke akun.
	Link(ctx context.Context, userID, provider, providerUserID, email string) error

	// TouchLogin memperbarui waktu login terakhir penyedia.
	TouchLogin(ctx context.Context, provider, providerUserID string, now time.Time) error
}

// LockError diterjemahkan repository ketika terdeteksi pelanggaran constraint
// unik — dipetakan ke shared.Conflict oleh caller.
type LockError struct{ Reason string }

func (e *LockError) Error() string { return e.Reason }
