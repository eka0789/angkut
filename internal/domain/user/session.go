package user

import (
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Session adalah satu perangkat/sesi login. ID-nya menjadi identitas "family"
// refresh token: rotasi membuat baris baru yang menunjuk kembali ke sesi yang
// sama, sehingga pencurian token dapat dideteksi dan seluruh family dicabut.
type Session struct {
	ID               string
	UserID           string
	RefreshTokenHash string
	DeviceName       string
	UserAgent        string
	IPAddress        string
	ExpiresAt        time.Time
	RevokedAt        *time.Time
	ReplacedByID     *string
	CreatedAt        time.Time
	LastUsedAt       *time.Time
}

// IsActive melaporkan apakah sesi masih dapat dipakai pada waktu tertentu.
func (s *Session) IsActive(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

// Revoke menandai sesi tidak berlaku lagi.
func (s *Session) Revoke(now time.Time) {
	if s.RevokedAt == nil {
		s.RevokedAt = &now
	}
}

// ErrSessionRevoked dikembalikan saat memakai sesi yang sudah dicabut. Ini
// sinyal khusus: pemanggil harus mencabut SELURUH sesi user (dugaan pencurian
// token), bukan sekadar menolak satu request.
var ErrSessionRevoked = shared.New(
	"SESSION_REVOKED",
	"Sesi ini sudah tidak berlaku. Silakan masuk kembali.",
	shared.ErrUnauthorized,
)

// ErrSessionExpired dikembalikan saat sesi melewati masa berlaku.
var ErrSessionExpired = shared.New(
	"SESSION_EXPIRED",
	"Sesi Anda berakhir. Silakan masuk kembali.",
	shared.ErrUnauthorized,
)

// ErrRefreshReuse terdeteksi saat refresh token yang sudah dirotasi dipakai
// lagi. Ini indikasi kuat token dicuri.
var ErrRefreshReuse = shared.New(
	"REFRESH_TOKEN_REUSED",
	"Demi keamanan, semua sesi Anda dihentikan karena terdeteksi pemakaian ulang token. Silakan masuk kembali.",
	shared.ErrUnauthorized,
)

// NewSession membuat sesi baru dengan hash refresh token yang sudah dihitung
// pemanggil (domain tidak pernah memegang token mentah).
func NewSession(userID, refreshHash, deviceName, userAgent, ip string, ttl time.Duration) *Session {
	now := time.Now().UTC()
	return &Session{
		ID:               platform.NewUUID(),
		UserID:           userID,
		RefreshTokenHash: refreshHash,
		DeviceName:       deviceName,
		UserAgent:        userAgent,
		IPAddress:        ip,
		ExpiresAt:        now.Add(ttl),
		CreatedAt:        now,
	}
}

// DeviceLabel adalah nama perangkat yang ramah untuk daftar sesi. Jatuh ke
// "Perangkat tidak dikenal" agar UI tidak pernah kosong.
func (s *Session) DeviceLabel() string {
	if s.DeviceName != "" {
		return s.DeviceName
	}
	if s.UserAgent != "" {
		return SummarizeUserAgent(s.UserAgent)
	}
	return "Perangkat tidak dikenal"
}

// SummarizeUserAgent memampatkan user-agent menjadi label pendek yang berguna
// di daftar perangkat. Sengaja sederhana (bukan parser lengkap) — hanya untuk
// membantu manusia mengenali perangkatnya.
func SummarizeUserAgent(ua string) string {
	platform := ""
	switch {
	case contains(ua, "Android"):
		platform = "Android"
	case contains(ua, "iPhone"), contains(ua, "iPad"):
		platform = "iOS"
	case contains(ua, "Windows"):
		platform = "Windows"
	case contains(ua, "Macintosh"), contains(ua, "Mac OS"):
		platform = "macOS"
	case contains(ua, "Linux"):
		platform = "Linux"
	}

	browser := ""
	switch {
	case contains(ua, "Edg/"):
		browser = "Edge"
	case contains(ua, "OPR/"), contains(ua, "Opera"):
		browser = "Opera"
	case contains(ua, "Chrome/"):
		browser = "Chrome"
	case contains(ua, "Safari/"):
		browser = "Safari"
	case contains(ua, "Firefox/"):
		browser = "Firefox"
	case contains(ua, "curl/"):
		browser = "curl"
	case contains(ua, "Postman"):
		browser = "Postman"
	}

	switch {
	case platform != "" && browser != "":
		return browser + " di " + platform
	case platform != "":
		return platform
	case browser != "":
		return browser
	default:
		return "Perangkat tidak dikenal"
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
