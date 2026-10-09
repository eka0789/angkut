// Package devmail menyediakan implementasi Mailer untuk pengembangan: tautan
// ditulis ke log alih-alih dikirim. Ini membuat alur verifikasi & reset dapat
// diuji end-to-end tanpa kredensial email.
//
// Di produksi, modul Notification (Phase 4) yang akan menggantikannya — use case
// di application/auth tidak perlu berubah karena keduanya memenuhi port yang sama.
package devmail

import (
	"context"
	"log/slog"

	"github.com/angkut/angkut/apps/api/internal/application/auth"
)

// Mailer menulis tautan verifikasi/reset ke log terstruktur.
type Mailer struct {
	log    *slog.Logger
	webURL string
}

// New membuat mailer dev.
func New(log *slog.Logger, webURL string) *Mailer {
	return &Mailer{log: log, webURL: webURL}
}

var _ auth.Mailer = (*Mailer)(nil)

// SendEmailVerification mencatat tautan verifikasi.
func (m *Mailer) SendEmailVerification(_ context.Context, to, name, token string) error {
	m.log.Info("email_verification_link",
		"to", to,
		"name", name,
		"url", m.webURL+"/verify-email?token="+token,
		"note", "dev mailer — tidak ada email yang benar-benar dikirim",
	)
	return nil
}

// SendPasswordReset mencatat tautan reset password.
func (m *Mailer) SendPasswordReset(_ context.Context, to, name, token string) error {
	m.log.Info("password_reset_link",
		"to", to,
		"name", name,
		"url", m.webURL+"/reset-password?token="+token,
		"note", "dev mailer — tidak ada email yang benar-benar dikirim",
	)
	return nil
}
