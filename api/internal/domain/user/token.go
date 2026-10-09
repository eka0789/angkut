package user

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/angkut/angkut/apps/api/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Masa berlaku token sekali-pakai. Reset password relatif pendek karena token
// ini setara akses akun; verifikasi email lebih longgar agar tidak menyiksa.
const (
	PasswordResetTTL  = 60 * time.Minute
	EmailVerifyTTL    = 24 * time.Hour
	RefreshTokenTTL   = 30 * 24 * time.Hour
	MFATokenTTL       = 5 * time.Minute
	RecoveryCodeCount = 10
)

// PasswordResetToken adalah token reset password sekali-pakai. Disimpan sebagai
// hash; token mentah hanya ada di tautan email.
type PasswordResetToken struct {
	ID          string
	UserID      string
	TokenHash   string
	ExpiresAt   time.Time
	UsedAt      *time.Time
	RequestedIP string
	CreatedAt   time.Time
}

// IsUsable melaporkan apakah token masih dapat dipakai.
func (t *PasswordResetToken) IsUsable(now time.Time) bool {
	return t.UsedAt == nil && now.Before(t.ExpiresAt)
}

// MarkUsed menghanguskan token (selalu dipanggil setelah pemakaian, termasuk
// saat password berhasil diganti).
func (t *PasswordResetToken) MarkUsed(now time.Time) { t.UsedAt = &now }

// EmailVerificationToken adalah token verifikasi alamat email.
type EmailVerificationToken struct {
	ID        string
	UserID    string
	Email     string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// IsUsable melaporkan apakah token verifikasi masih dapat dipakai.
func (t *EmailVerificationToken) IsUsable(now time.Time) bool {
	return t.UsedAt == nil && now.Before(t.ExpiresAt)
}

// MarkUsed menghanguskan token verifikasi.
func (t *EmailVerificationToken) MarkUsed(now time.Time) { t.UsedAt = &now }

// RecoveryCode adalah kode pemulihan 2FA sekali-pakai (di-hash).
type RecoveryCode struct {
	ID       string
	UserID   string
	CodeHash string
	UsedAt   *time.Time
}

// ErrTokenInvalid adalah pesan tunggal untuk semua kegagalan token sekali-pakai.
// Sengaja tidak membedakan "tidak ada", "kedaluwarsa", dan "sudah dipakai"
// supaya penyerang tidak dapat memetakan token yang pernah ada.
var ErrTokenInvalid = shared.New(
	"TOKEN_INVALID",
	"Tautan tidak valid atau sudah kedaluwarsa. Minta tautan baru.",
	shared.ErrValidation,
)

// GenerateOpaqueToken membuat token acak kriptografis untuk refresh token,
// token reset, dan token verifikasi. Mengembalikan token mentah + hash-nya.
func GenerateOpaqueToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, HashToken(raw), nil
}

// HashToken menghitung hash token untuk disimpan. SHA-256 cukup di sini karena
// token sudah acak 256-bit — tidak ada ruang tebak yang bisa diperkaya dengan
// KDF lambat (berbeda dari password).
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// NewPasswordResetToken menyusun token reset lengkap dengan hash.
func NewPasswordResetToken(userID, tokenHash, ip string) *PasswordResetToken {
	now := time.Now().UTC()
	return &PasswordResetToken{
		ID:          platform.NewUUID(),
		UserID:      userID,
		TokenHash:   tokenHash,
		ExpiresAt:   now.Add(PasswordResetTTL),
		RequestedIP: ip,
		CreatedAt:   now,
	}
}

// NewEmailVerificationToken menyusun token verifikasi email.
func NewEmailVerificationToken(userID, email, tokenHash string) *EmailVerificationToken {
	now := time.Now().UTC()
	return &EmailVerificationToken{
		ID:        platform.NewUUID(),
		UserID:    userID,
		Email:     email,
		TokenHash: tokenHash,
		ExpiresAt: now.Add(EmailVerifyTTL),
		CreatedAt: now,
	}
}

// GenerateRecoveryCodes membuat N kode pemulihan. Mengembalikan daftar token
// mentah (untuk ditampilkan sekali) beserta entitas ber-hash untuk disimpan.
func GenerateRecoveryCodes(userID string) (plain []string, codes []RecoveryCode, err error) {
	plain = make([]string, 0, RecoveryCodeCount)
	codes = make([]RecoveryCode, 0, RecoveryCodeCount)

	for i := 0; i < RecoveryCodeCount; i++ {
		code, err := randomCode()
		if err != nil {
			return nil, nil, err
		}
		plain = append(plain, code)
		codes = append(codes, RecoveryCode{
			ID:       platform.NewUUID(),
			UserID:   userID,
			CodeHash: HashToken(normalizeRecoveryCode(code)),
		})
	}
	return plain, codes, nil
}

// MatchRecoveryCode membandingkan kode yang dimasukkan pengguna dengan hash
// tersimpan. Normalisasi (huruf kecil, buang tanda hubung) dilakukan agar
// pengguna tidak dihukum karena salah menyalin format.
func MatchRecoveryCode(input string, codeHash string) bool {
	return HashToken(normalizeRecoveryCode(input)) == codeHash
}

func normalizeRecoveryCode(code string) string {
	out := make([]rune, 0, len(code))
	for _, r := range code {
		switch {
		case r == '-' || r == ' ':
			continue
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

func randomCode() (string, error) {
	const alphabet = "abcdefghijkmnpqrstuvwxyz23456789" // tanpa huruf/angka rancu
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b[0:4]) + "-" + string(b[4:8]), nil
}
