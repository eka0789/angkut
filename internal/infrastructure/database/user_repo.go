package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/user"
)

// UserRepo adalah implementasi user.Repository berbasis pgx.
//
// Tabel `users` bersifat global (bukan per tenant), jadi query di sini sengaja
// tidak menyaring organization_id. Isolasi tenant berlaku untuk tabel bisnis.
type UserRepo struct {
	q Querier
}

// NewUserRepo membuat repository dengan querier (pool atau tx).
func NewUserRepo(q Querier) *UserRepo { return &UserRepo{q: q} }

// WithTx mengembalikan repository yang berjalan di dalam transaksi tertentu.
func (r *UserRepo) WithTx(q Querier) *UserRepo { return &UserRepo{q: q} }

const userColumns = `
	id, email, email_verified_at, password_hash, name, phone, avatar_url,
	locale, timezone, status, last_login_at, failed_login_count, locked_until,
	twofa_enabled, twofa_secret_enc, twofa_confirmed_at, password_changed_at,
	created_at, updated_at, deleted_at`

// Create menyimpan pengguna baru. Pelanggaran unique index email diterjemahkan
// menjadi error domain, bukan error driver yang bocor ke atas.
func (r *UserRepo) Create(ctx context.Context, u *user.User) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO users (
			id, email, email_verified_at, password_hash, name, phone, avatar_url,
			locale, timezone, status, twofa_enabled, twofa_secret_enc, twofa_confirmed_at,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		u.ID, u.Email, u.EmailVerifiedAt, nullIfEmpty(u.PasswordHash), u.Name, nullIfEmpty(u.Phone),
		nullIfEmpty(u.AvatarURL), u.Locale, u.Timezone, string(u.Status),
		u.TwoFAEnabled, u.TwoFASecretEnc, u.TwoFAConfirmedAt, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return shared.Conflict("EMAIL_TAKEN", "Email ini sudah terdaftar.")
		}
		return err
	}
	return nil
}

// GetByID mengambil user berdasarkan id.
func (r *UserRepo) GetByID(ctx context.Context, id string) (*user.User, error) {
	row := r.q.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1 AND deleted_at IS NULL`, id)
	return scanUser(row)
}

// GetByEmail mengambil user berdasarkan email (sudah dinormalisasi pemanggil).
func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	row := r.q.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1) AND deleted_at IS NULL`, email)
	return scanUser(row)
}

// UpdateProfile menyimpan perubahan profil.
func (r *UserRepo) UpdateProfile(ctx context.Context, u *user.User) error {
	_, err := r.q.Exec(ctx, `
		UPDATE users SET name=$2, phone=$3, avatar_url=$4, locale=$5, timezone=$6
		WHERE id=$1 AND deleted_at IS NULL`,
		u.ID, u.Name, nullIfEmpty(u.Phone), nullIfEmpty(u.AvatarURL), u.Locale, u.Timezone)
	return err
}

// UpdatePassword menyimpan hash baru dan menandai waktu perubahan. Waktu ini
// dipakai untuk menghentikan sesi yang diterbitkan sebelum password berganti.
func (r *UserRepo) UpdatePassword(ctx context.Context, u *user.User) error {
	_, err := r.q.Exec(ctx, `
		UPDATE users SET password_hash=$2, password_changed_at=$3, failed_login_count=0, locked_until=NULL
		WHERE id=$1 AND deleted_at IS NULL`,
		u.ID, u.PasswordHash, u.PasswordChangedAt)
	return err
}

// UpdateLoginState menyimpan penghitung gagal, kunci akun, dan waktu login.
func (r *UserRepo) UpdateLoginState(ctx context.Context, u *user.User) error {
	_, err := r.q.Exec(ctx, `
		UPDATE users SET failed_login_count=$2, locked_until=$3, last_login_at=$4
		WHERE id=$1`,
		u.ID, u.FailedLoginCount, u.LockedUntil, u.LastLoginAt)
	return err
}

// MarkEmailVerified menyimpan status verifikasi dan aktivasi akun.
func (r *UserRepo) MarkEmailVerified(ctx context.Context, u *user.User) error {
	_, err := r.q.Exec(ctx, `
		UPDATE users SET email_verified_at=$2, status=$3
		WHERE id=$1 AND deleted_at IS NULL`,
		u.ID, u.EmailVerifiedAt, string(u.Status))
	return err
}

// UpdateTwoFA menyimpan status & secret 2FA.
func (r *UserRepo) UpdateTwoFA(ctx context.Context, u *user.User) error {
	_, err := r.q.Exec(ctx, `
		UPDATE users SET twofa_enabled=$2, twofa_secret_enc=$3, twofa_confirmed_at=$4
		WHERE id=$1 AND deleted_at IS NULL`,
		u.ID, u.TwoFAEnabled, u.TwoFASecretEnc, u.TwoFAConfirmedAt)
	return err
}

func scanUser(row pgx.Row) (*user.User, error) {
	var u user.User
	var status string
	var passwordHash, phone, avatar *string

	err := row.Scan(
		&u.ID, &u.Email, &u.EmailVerifiedAt, &passwordHash, &u.Name, &phone, &avatar,
		&u.Locale, &u.Timezone, &status, &u.LastLoginAt, &u.FailedLoginCount, &u.LockedUntil,
		&u.TwoFAEnabled, &u.TwoFASecretEnc, &u.TwoFAConfirmedAt, &u.PasswordChangedAt,
		&u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.NotFound("USER_NOT_FOUND", "Pengguna tidak ditemukan.")
		}
		return nil, err
	}
	u.Status = user.Status(status)
	if passwordHash != nil {
		u.PasswordHash = *passwordHash
	}
	if phone != nil {
		u.Phone = *phone
	}
	if avatar != nil {
		u.AvatarURL = *avatar
	}
	return &u, nil
}

// ─── Sesi ─────────────────────────────────────────────────────────────────

// SessionRepo adalah implementasi user.SessionRepository berbasis pgx.
type SessionRepo struct {
	q Querier
}

// NewSessionRepo membuat repository sesi.
func NewSessionRepo(q Querier) *SessionRepo { return &SessionRepo{q: q} }

// WithTx menjalankan repository di transaksi tertentu.
func (r *SessionRepo) WithTx(q Querier) *SessionRepo { return &SessionRepo{q: q} }

// ip_address di-cast ke text: pgx tidak dapat memindai tipe inet ke string,
// dan IP di sini hanya metadata tampilan — bukan nilai yang dihitung.
const sessionColumns = `
	id, user_id, refresh_token_hash, device_name, user_agent,
	COALESCE(ip_address::text, ''),
	expires_at, revoked_at, replaced_by_id, created_at, last_used_at`

// Create menyimpan sesi baru.
func (r *SessionRepo) Create(ctx context.Context, s *user.Session) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO user_sessions (
			id, user_id, refresh_token_hash, device_name, user_agent, ip_address,
			expires_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		s.ID, s.UserID, s.RefreshTokenHash, nullIfEmpty(s.DeviceName),
		nullIfEmpty(s.UserAgent), nullIfEmpty(s.IPAddress), s.ExpiresAt, s.CreatedAt)
	return err
}

// GetByID mengambil sesi berdasarkan id.
func (r *SessionRepo) GetByID(ctx context.Context, id string) (*user.Session, error) {
	row := r.q.QueryRow(ctx, `SELECT `+sessionColumns+` FROM user_sessions WHERE id=$1`, id)
	return scanSession(row)
}

// GetByRefreshHash mencari sesi lewat hash refresh token.
//
// Sengaja TIDAK menyaring `revoked_at IS NULL`: pemanggil perlu melihat sesi
// yang sudah dicabut untuk membedakan "kedaluwarsa" dari "token dipakai ulang".
func (r *SessionRepo) GetByRefreshHash(ctx context.Context, hash string) (*user.Session, error) {
	row := r.q.QueryRow(ctx, `SELECT `+sessionColumns+` FROM user_sessions WHERE refresh_token_hash=$1`, hash)
	return scanSession(row)
}

// Rotate mencabut sesi lama dan membuat sesi baru, lalu menautkannya.
// Satu transaksi supaya tidak ada celah di mana keduanya sama-sama valid.
func (r *SessionRepo) Rotate(ctx context.Context, oldSessionID string, next *user.Session) error {
	// URUTAN PENTING: sesi baru disisipkan LEBIH DULU, karena kolom
	// `replaced_by_id` pada baris lama ber-FK ke sesi baru. Menautkan sebelum
	// baris tujuan ada akan melanggar foreign key.
	//
	// Fungsi ini selalu dipanggil di dalam transaksi (lihat application/auth.Refresh):
	// bila penautan gagal karena sesi sudah pernah dirotasi, sesi baru yang baru
	// disisipkan ikut ter-rollback — tidak ada baris yatim.
	if err := r.Create(ctx, next); err != nil {
		return err
	}

	tag, err := r.q.Exec(ctx, `
		UPDATE user_sessions
		   SET revoked_at = now(), replaced_by_id = $2
		 WHERE id = $1 AND revoked_at IS NULL`,
		oldSessionID, next.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Sudah dicabut sebelumnya → ini pemakaian ulang token lama.
		return user.ErrRefreshReuse
	}
	return nil
}

// ListActiveByUser mengembalikan sesi yang belum dicabut dan belum kedaluwarsa.
func (r *SessionRepo) ListActiveByUser(ctx context.Context, userID string, now time.Time) ([]user.Session, error) {
	rows, err := r.q.Query(ctx, `
		SELECT `+sessionColumns+` FROM user_sessions
		 WHERE user_id=$1 AND revoked_at IS NULL AND expires_at > $2
		 ORDER BY COALESCE(last_used_at, created_at) DESC`,
		userID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []user.Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Revoke mencabut satu sesi milik user. Kepemilikan diverifikasi di query
// (user_id) sehingga seseorang tidak dapat mencabut sesi orang lain.
func (r *SessionRepo) Revoke(ctx context.Context, userID, sessionID string, now time.Time) (bool, error) {
	tag, err := r.q.Exec(ctx, `
		UPDATE user_sessions SET revoked_at=$3
		 WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`,
		sessionID, userID, now)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// RevokeAll mencabut seluruh sesi aktif user.
func (r *SessionRepo) RevokeAll(ctx context.Context, userID string, now time.Time) (int, error) {
	tag, err := r.q.Exec(ctx, `
		UPDATE user_sessions SET revoked_at=$2
		 WHERE user_id=$1 AND revoked_at IS NULL`,
		userID, now)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// TouchLastUsed memperbarui waktu pemakaian terakhir.
func (r *SessionRepo) TouchLastUsed(ctx context.Context, sessionID string, now time.Time) error {
	_, err := r.q.Exec(ctx, `UPDATE user_sessions SET last_used_at=$2 WHERE id=$1`, sessionID, now)
	return err
}

func scanSession(row pgx.Row) (*user.Session, error) {
	var s user.Session
	var deviceName, userAgent *string

	err := row.Scan(
		&s.ID, &s.UserID, &s.RefreshTokenHash, &deviceName, &userAgent, &s.IPAddress,
		&s.ExpiresAt, &s.RevokedAt, &s.ReplacedByID, &s.CreatedAt, &s.LastUsedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.NotFound("SESSION_NOT_FOUND", "Sesi tidak ditemukan.")
		}
		return nil, err
	}
	if deviceName != nil {
		s.DeviceName = *deviceName
	}
	if userAgent != nil {
		s.UserAgent = *userAgent
	}
	return &s, nil
}

// ─── helper ───────────────────────────────────────────────────────────────

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// isUniqueViolation melaporkan pelanggaran constraint unik (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// isForeignKeyViolation melaporkan pelanggaran foreign key (SQLSTATE 23503).
func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// isCheckViolation melaporkan pelanggaran CHECK (SQLSTATE 23514).
func isCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514"
}
