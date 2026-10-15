// Package user adalah domain identitas: pengguna, sesi perangkat, dan token
// sekali-pakai. Murni — tanpa I/O, tanpa driver. Semua aturan identitas hidup
// di sini agar tidak tersebar ke handler.
package user

import (
	"strings"
	"time"
	"unicode"

	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Status adalah siklus hidup akun.
type Status string

const (
	// StatusPending: terdaftar tetapi email belum diverifikasi.
	StatusPending Status = "pending"
	// StatusActive: dapat login penuh.
	StatusActive Status = "active"
	// StatusSuspended: diblokir admin; login ditolak.
	StatusSuspended Status = "suspended"
)

// User adalah akun pengguna. Tidak ada password mentah di sini — hanya hash.
type User struct {
	ID               string
	Email            string
	EmailVerifiedAt  *time.Time
	PasswordHash     string // kosong bila akun hanya OAuth
	Name             string
	Phone            string
	AvatarURL        string
	Locale           string
	Timezone         string
	Status           Status
	LastLoginAt      *time.Time
	FailedLoginCount int
	LockedUntil      *time.Time

	TwoFAEnabled     bool
	TwoFASecretEnc   []byte
	TwoFAConfirmedAt *time.Time

	PasswordChangedAt *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
}

// IsEmailVerified melaporkan apakah email sudah diverifikasi.
func (u *User) IsEmailVerified() bool { return u.EmailVerifiedAt != nil }

// CanLogin menolak akun yang belum aktif atau sedang terkunci.
// Membedakan "belum verifikasi" dari "salah password" penting untuk pesan yang
// jujur tanpa membocorkan status akun orang lain.
func (u *User) CanLogin(now time.Time) error {
	if u.DeletedAt != nil {
		return shared.New("ACCOUNT_DELETED", "Akun ini sudah tidak aktif.", shared.ErrForbidden)
	}
	switch u.Status {
	case StatusSuspended:
		return shared.New("ACCOUNT_SUSPENDED", "Akun Anda ditangguhkan. Hubungi dukungan.", shared.ErrForbidden)
	case StatusPending:
		return shared.New("ACCOUNT_PENDING", "Verifikasi email Anda terlebih dahulu sebelum masuk.", shared.ErrForbidden)
	case StatusActive:
	default:
		return shared.New("ACCOUNT_STATUS_UNKNOWN", "Status akun tidak dikenali.", shared.ErrForbidden)
	}
	if u.LockedUntil != nil && now.Before(*u.LockedUntil) {
		return shared.New("ACCOUNT_LOCKED",
			"Terlalu banyak percobaan gagal. Coba lagi beberapa saat lagi.", shared.ErrForbidden)
	}
	return nil
}

// HasPassword melaporkan apakah akun punya password (bukan OAuth-only).
func (u *User) HasPassword() bool { return u.PasswordHash != "" }

// LockDuration adalah berapa lama akun terkunci setelah percobaan gagal berlebih.
const (
	MaxFailedLogins = 10
	LockDuration    = 15 * time.Minute
)

// RegisterFailure menaikkan penghitung gagal dan mengunci akun bila melewati
// ambang batas. Mengembalikan true bila akun baru saja terkunci.
func (u *User) RegisterFailure(now time.Time) bool {
	u.FailedLoginCount++
	if u.FailedLoginCount >= MaxFailedLogins {
		until := now.Add(LockDuration)
		u.LockedUntil = &until
		return true
	}
	return false
}

// RegisterSuccess mereset penghitung gagal (dipakai setelah login berhasil).
func (u *User) RegisterSuccess(now time.Time) {
	u.FailedLoginCount = 0
	u.LockedUntil = nil
	u.LastLoginAt = &now
}

// NormalizeEmail menjadikan email konsisten (trim + huruf kecil). Dipakai untuk
// penyimpanan dan pencarian — DB menyimpan index unik pada lower(email).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidateName menolak nama kosong/terlalu panjang.
func ValidateName(name string) error {
	n := strings.TrimSpace(name)
	if len(n) < 2 {
		return shared.Validation(map[string]string{"name": "minimal 2 karakter."})
	}
	if len(n) > 120 {
		return shared.Validation(map[string]string{"name": "maksimal 120 karakter."})
	}
	return nil
}

// ValidatePassword menegakkan kebijakan password minimum. Sengaja tidak
// memaksa simbol: panjang lebih menentukan, dan aturan rumit justru mendorong
// password yang mudah ditebak.
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return shared.Validation(map[string]string{"password": "minimal 8 karakter."})
	}
	if len(password) > 128 {
		return shared.Validation(map[string]string{"password": "maksimal 128 karakter."})
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return shared.Validation(map[string]string{
			"password": "harus memuat huruf dan angka.",
		})
	}
	return nil
}

// NewUser menyusun pengguna baru dengan nilai awal yang konsisten.
func NewUser(email, name string) *User {
	now := time.Now().UTC()
	return &User{
		ID:        platform.NewUUID(),
		Email:     NormalizeEmail(email),
		Name:      strings.TrimSpace(name),
		Locale:    "id",
		Timezone:  "Asia/Jakarta",
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// MarkEmailVerified mengaktifkan akun setelah token verifikasi valid.
func (u *User) MarkEmailVerified(now time.Time) {
	u.EmailVerifiedAt = &now
	if u.Status == StatusPending {
		u.Status = StatusActive
	}
	u.UpdatedAt = now
}
