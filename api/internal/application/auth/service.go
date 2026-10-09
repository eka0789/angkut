// Package auth berisi use case identitas: registrasi, login, rotasi sesi,
// reset password, verifikasi email, dan 2FA.
//
// Semua keputusan keamanan yang terlihat pengguna diambil di sini, sehingga
// handler tetap tipis dan dapat diuji tanpa HTTP.
package auth

import (
	"context"
	"time"

	"github.com/angkut/angkut/apps/api/internal/domain/audit"
	"github.com/angkut/angkut/apps/api/internal/domain/organization"
	"github.com/angkut/angkut/apps/api/internal/domain/user"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/cryptox"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/tokenx"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Mailer mengirim tautan verifikasi & reset password.
//
// Ini port sempit, bukan modul notifikasi: auth hanya butuh dua pesan ini.
// Implementasi MVP menulis ke log; modul Notification (Phase 4) akan
// menggantikannya tanpa mengubah use case di sini.
type Mailer interface {
	SendEmailVerification(ctx context.Context, to, name, token string) error
	SendPasswordReset(ctx context.Context, to, name, token string) error
}

// Deps adalah dependensi Service. Semua eksplisit — tidak ada global state.
type Deps struct {
	// DB adalah factory repository. Gunakan DB.WithTx(q) di dalam transaksi.
	DB    *database.Stores
	Tx    *database.TxManager
	Audit audit.Recorder

	JWT    *tokenx.Issuer
	MFA    *tokenx.MFAIssuer
	Cipher *cryptox.Cipher
	Params cryptox.Argon2Params
	TOTP   tokenx.TOTPConfig

	Mailer Mailer
	Clock  platform.Clock

	// WebURL dipakai menyusun tautan di email (mis. https://angkut.id).
	WebURL string
}

// Service mengimplementasikan use case identitas.
type Service struct {
	deps Deps
}

// New membuat Service dengan dependensi yang diberikan.
func New(d Deps) *Service {
	if d.Params.MemoryKiB == 0 {
		d.Params = cryptox.DefaultArgon2Params()
	}
	if d.TOTP.Issuer == "" {
		d.TOTP = tokenx.DefaultTOTPConfig()
	}
	if d.Clock == nil {
		d.Clock = platform.RealClock{}
	}
	return &Service{deps: d}
}

// now mengembalikan waktu UTC — seluruh penyimpanan waktu memakai UTC.
func (s *Service) now() time.Time { return s.deps.Clock.Now().UTC() }

// ─── Kontrak hasil ────────────────────────────────────────────────────────

// Session is one issued session with its tokens.
type IssuedSession struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int // detik
	SessionID    string
}

// AuthResult adalah hasil login/refresh yang berhasil.
type AuthResult struct {
	Session       IssuedSession
	User          *user.User
	Organizations []organization.MembershipInfo
	ActiveOrgID   string
	ActiveRole    organization.RoleCode
}

// LoginResult menampung dua kemungkinan: sesi langsung, atau langkah 2FA.
type LoginResult struct {
	// RequiresMFA true berarti password benar tetapi 2FA aktif; klien harus
	// memanggil VerifyTwoFactor dengan MFAToken.
	RequiresMFA bool
	MFAToken    string
	Auth        *AuthResult
}

// ContextInfo membawa metadata request untuk audit.
type ContextInfo struct {
	IP        string
	UserAgent string
	RequestID string
}

// auditEntry menyusun entri audit dengan metadata request yang konsisten.
func (s *Service) auditEntry(action string, c ContextInfo) audit.Entry {
	return audit.Event(action).WithRequest(c.IP, c.UserAgent, c.RequestID)
}
