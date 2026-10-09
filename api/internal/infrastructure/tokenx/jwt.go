// Package tokenx menerbitkan dan memvalidasi access token JWT.
//
// Access token sengaja dibuat STATELESS dan berumur pendek (ADR-007). Yang
// membawa state sesi adalah refresh token (opaque, tersimpan sebagai hash).
// Klaim `sid` menautkan access token ke sesi sehingga pencabutan sesi dapat
// diberlakukan lebih awal daripada masa berlaku token.
package tokenx

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrTokenInvalid menandai access token yang tidak sah (tanda tangan salah,
// kedaluwarsa, atau klaim tidak lengkap).
var ErrTokenInvalid = errors.New("access token tidak valid")

// Claims adalah isi access token.
type Claims struct {
	UserID         string `json:"uid"`
	SessionID      string `json:"sid"`
	OrganizationID string `json:"org,omitempty"`
	RoleCode       string `json:"role,omitempty"`
	Email          string `json:"email,omitempty"`
	jwt.RegisteredClaims
}

// Issuer menerbitkan & memverifikasi access token dengan algoritma HS256.
// Asimetris (RS256) tidak dipakai karena hanya satu penerbit dan satu verifier
// — simetri lebih sederhana dan kuncinya sudah rahasia server.
type Issuer struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

// New membuat issuer. TTL default 15 menit bila tidak diisi.
func New(secret string, ttl time.Duration) (*Issuer, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT_ACCESS_SECRET minimal 32 karakter")
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &Issuer{secret: []byte(secret), ttl: ttl, issuer: "angkut"}, nil
}

// TTL mengembalikan masa berlaku access token (dipakai untuk `expires_in`).
func (i *Issuer) TTL() time.Duration { return i.ttl }

// Issue membuat access token untuk sesi tertentu.
func (i *Issuer) Issue(userID, sessionID, email, organizationID, roleCode string, now time.Time) (string, error) {
	claims := Claims{
		UserID:         userID,
		SessionID:      sessionID,
		OrganizationID: organizationID,
		RoleCode:       roleCode,
		Email:          email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)), // toleransi clock skew
			ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(i.secret)
}

// Verify memvalidasi tanda tangan + klaim standar, lalu mengembalikan claims.
func (i *Issuer) Verify(raw string, now time.Time) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		// Tolak algoritma lain secara eksplisit — mencegah serangan alg-confusion.
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("algoritma tanda tangan tidak diizinkan: %s", t.Method.Alg())
		}
		return i.secret, nil
	},
		jwt.WithIssuer(i.issuer),
		jwt.WithTimeFunc(func() time.Time { return now }),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	if claims.UserID == "" || claims.SessionID == "" {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}
