// Package notificationemail menyediakan adapter kanal EMAIL untuk notifikasi
// (TASK-040). Adapter dev menulis email ke log terstruktur — pola yang sama
// dengan devmail untuk alur auth — sehingga alur notifikasi dapat diuji
// end-to-end tanpa kredensial SMTP.
//
// Adapter SMTP/provider sungguhan menyusul di fase deployment; port di worker
// (handler.EmailSender) tidak berubah.
package notificationemail

import (
	"context"
	"log/slog"
)

// LogMailer adalah EmailSender dev: mencatat email yang AKAN dikirim.
type LogMailer struct {
	log  *slog.Logger
	from string
}

// NewLogMailer membuat mailer log.
func NewLogMailer(log *slog.Logger, from string) *LogMailer {
	if log == nil {
		log = slog.Default()
	}
	return &LogMailer{log: log, from: from}
}

// Send mencatat satu email keluar. Tidak pernah gagal — kegagalan pengiriman
// sungguhan adalah urusan adapter produksi.
func (m *LogMailer) Send(_ context.Context, to, subject, body string) error {
	m.log.Info("email_notifikasi_keluar",
		"to", to,
		"from", m.from,
		"subject", subject,
		"body", body,
		"note", "dev mailer — tidak ada email yang benar-benar dikirim",
	)
	return nil
}
