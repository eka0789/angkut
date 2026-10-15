// Package tokenx — TOTP (2FA) dan token MFA sementara.
package tokenx

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// TOTPConfig mengatur parameter TOTP. Nilai default mengikuti kebiasaan aplikasi
// authenticator (6 digit, periode 30 detik, SHA-1) agar kompatibel luas.
type TOTPConfig struct {
	Issuer string
	Digits otp.Digits
	Period uint
	Skew   uint // jumlah periode toleransi sebelum/sesudah (clock skew)
}

// DefaultTOTPConfig mengembalikan konfigurasi yang kompatibel dengan
// Google Authenticator, Authy, dan Microsoft Authenticator.
func DefaultTOTPConfig() TOTPConfig {
	return TOTPConfig{
		Issuer: "Angkut",
		Digits: otp.DigitsSix,
		Period: 30,
		Skew:   1,
	}
}

// NewTOTPSecret menghasilkan secret baru beserta URI otpauth:// yang bisa
// dirender sebagai QR code di frontend. `accountName` biasanya email user.
func NewTOTPSecret(cfg TOTPConfig, accountName string) (secret string, uri string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      cfg.Issuer,
		AccountName: accountName,
		Period:      cfg.Period,
		Digits:      cfg.Digits,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ErrTOTPMismatch dikembalikan saat kode 2FA tidak cocok.
var ErrTOTPMismatch = errors.New("kode verifikasi 2FA tidak cocok")

// ValidateTOTP memeriksa kode 6 digit terhadap secret.
func ValidateTOTP(cfg TOTPConfig, secret, code string, now time.Time) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrTOTPMismatch
	}
	ok, err := totp.ValidateCustom(code, secret, now, totp.ValidateOpts{
		Period:    cfg.Period,
		Skew:      cfg.Skew,
		Digits:    cfg.Digits,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		// Secret cacat (bukan sekadar kode salah) — jangan bocorkan detailnya.
		return ErrTOTPMismatch
	}
	if !ok {
		return ErrTOTPMismatch
	}
	return nil
}

// MFAPurpose adalah alasan token MFA sementara diterbitkan.
type MFAPurpose string

const (
	// MFAPurposeLogin: token untuk menyelesaikan login yang tertahan 2FA.
	MFAPurposeLogin MFAPurpose = "login"
)

// ErrMFAInvalid menandai token MFA yang tidak sah, kedaluwarsa, atau salah tujuan.
var ErrMFAInvalid = errors.New("token 2FA tidak valid atau sudah kedaluwarsa")

// MFAIssuer menerbitkan & memverifikasi token sementara untuk langkah kedua 2FA.
//
// Token ini BUKAN access token: ia hanya membuktikan "password sudah benar,
// tinggal verifikasi kode". Ia juga tidak memberi akses ke endpoint lain —
// middleware auth menolaknya karena issuer-nya berbeda.
type MFAIssuer struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

// NewMFAIssuer membuat issuer token MFA. Memakai kunci yang sama dengan access
// token tetapi issuer berbeda, sehingga kedua jenis token tidak dapat tertukar.
func NewMFAIssuer(secret string, ttl time.Duration) (*MFAIssuer, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT_ACCESS_SECRET minimal 32 karakter")
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &MFAIssuer{secret: []byte(secret), ttl: ttl, issuer: "angkut-mfa"}, nil
}

type mfaClaims struct {
	Purpose string `json:"purpose"`
	jwt.RegisteredClaims
}

// Issue menerbitkan token MFA untuk sebuah user.
func (i *MFAIssuer) Issue(userID string, purpose MFAPurpose, now time.Time) (string, error) {
	claims := mfaClaims{
		Purpose: string(purpose),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
}

// Verify memvalidasi token MFA dan memastikan tujuannya sesuai.
func (i *MFAIssuer) Verify(raw string, purpose MFAPurpose, now time.Time) (string, error) {
	claims := &mfaClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("algoritma tidak diizinkan: %s", t.Method.Alg())
		}
		return i.secret, nil
	},
		jwt.WithIssuer(i.issuer),
		jwt.WithTimeFunc(func() time.Time { return now }),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return "", ErrMFAInvalid
	}
	if claims.Purpose != string(purpose) || claims.Subject == "" {
		return "", ErrMFAInvalid
	}
	return claims.Subject, nil
}
