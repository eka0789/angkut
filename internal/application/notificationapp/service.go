// Package notificationapp berisi logika aplikasi notifikasi (TASK-040):
// pusat notifikasi in-app per pengguna, template per organisasi, preferensi
// per pengguna, dan fan-out saat kejadian bisnis terjadi.
//
// Aturan pengiriman yang dipegang paket ini:
//   - penerima in-app = anggota aktif organisasi, KECUALI yang menonaktifkan
//     event itu di preferensinya (default aktif — organisasi satu-orang tetap
//     menerima pembaruan);
//   - email hanya untuk yang OPT-IN (default nonaktif);
//   - pengiriman email DILAKUKAN WORKER lewat event outbox `notification.send`
//     (ADR-005) — tidak ada panggilan jaringan di dalam transaksi.
package notificationapp

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/event"
	"github.com/angkut/angkut/apps/internal/domain/notification"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Deps adalah dependensi service notifikasi.
type Deps struct {
	Tx     *database.TxManager
	Stores *database.Stores
	Log    *slog.Logger
}

// Service adalah service aplikasi notifikasi.
type Service struct {
	tx     *database.TxManager
	stores *database.Stores
	log    *slog.Logger
}

// New membuat service notifikasi.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{tx: d.Tx, stores: d.Stores, log: log}
}

// ListFilter adalah filter daftar notifikasi dari endpoint.
type ListFilter struct {
	Unread   bool
	Priority string
	Limit    int
	Offset   int
}

// toDomainFilter menerjemahkan filter endpoint ke filter domain, memvalidasi
// nilai yang diterima dari query string.
func toDomainFilter(f ListFilter) (notification.Filter, error) {
	out := notification.Filter{Unread: f.Unread, Limit: f.Limit, Offset: f.Offset}
	if f.Priority != "" {
		p := notification.Priority(f.Priority)
		if !p.Valid() {
			return out, shared.Validation(map[string]string{
				"priority": "Prioritas harus low, normal, atau high.",
			})
		}
		out.Priority = p
	}
	return out, nil
}

// List mengembalikan halaman notifikasi in-app milik pengguna.
func (s *Service) List(ctx context.Context, orgID, userID string, f ListFilter) ([]notification.Notification, int, error) {
	df, err := toDomainFilter(f)
	if err != nil {
		return nil, 0, err
	}
	return s.stores.Notifications().List(ctx, orgID, userID, df)
}

// UnreadCount menghitung notifikasi belum dibaca (badge lonceng).
func (s *Service) UnreadCount(ctx context.Context, orgID, userID string) (int, error) {
	return s.stores.Notifications().UnreadCount(ctx, orgID, userID)
}

// MarkRead menandai satu notifikasi (milik pengguna) sebagai dibaca.
func (s *Service) MarkRead(ctx context.Context, orgID, userID, id string) error {
	return s.stores.Notifications().MarkRead(ctx, orgID, userID, id)
}

// MarkAllRead menandai semua notifikasi pengguna sebagai dibaca.
func (s *Service) MarkAllRead(ctx context.Context, orgID, userID string) (int, error) {
	return s.stores.Notifications().MarkAllRead(ctx, orgID, userID)
}

// ── Template ────────────────────────────────────────────────────────────

// TemplateView adalah satu baris tampilan template EFEKTIF: kustom organisasi
// bila ada, kalau tidak template bawaan platform.
type TemplateView struct {
	Key    notification.EventKey `json:"key"`
	Title  string                `json:"title_template"`
	Body   string                `json:"body_template"`
	Custom bool                  `json:"custom"`
}

// Templates mengembalikan template efektif per event (untuk pusat pesan
// in-app; email memakai template yang sama).
func (s *Service) Templates(ctx context.Context, orgID string) ([]TemplateView, error) {
	custom, err := s.stores.NotificationTemplates().List(ctx, orgID)
	if err != nil {
		return nil, err
	}
	byKey := make(map[notification.EventKey]notification.Template, len(custom))
	for _, t := range custom {
		byKey[t.Key] = t
	}
	out := make([]TemplateView, 0, len(notification.EventKeys()))
	for _, k := range notification.EventKeys() {
		if t, ok := byKey[k]; ok && t.TitleTemplate != "" {
			out = append(out, TemplateView{Key: k, Title: t.TitleTemplate, Body: t.BodyTemplate, Custom: true})
			continue
		}
		def, ok := notification.TemplateFor(k, notification.ChannelInApp)
		if !ok {
			continue
		}
		out = append(out, TemplateView{Key: k, Title: def.Title, Body: def.Body})
	}
	return out, nil
}

// UpdateTemplateInput adalah permintaan menyimpan template kustom.
type UpdateTemplateInput struct {
	Key   string
	Title string
	Body  string
	Actor string
}

// UpdateTemplate menyimpan template kustom organisasi untuk satu event.
func (s *Service) UpdateTemplate(ctx context.Context, orgID string, in UpdateTemplateInput) error {
	key := notification.EventKey(strings.TrimSpace(in.Key))
	if !key.Valid() {
		return shared.Validation(map[string]string{"key": "Event notifikasi tidak dikenal."})
	}
	title := strings.TrimSpace(in.Title)
	body := strings.TrimSpace(in.Body)
	if title == "" || body == "" {
		return shared.Validation(map[string]string{
			"title": "Judul dan isi template wajib diisi.",
			"body":  "Judul dan isi template wajib diisi.",
		})
	}
	now := time.Now().UTC()
	t := &notification.Template{
		ID: platform.NewUUID(), OrganizationID: orgID,
		Key: key, Channel: notification.ChannelInApp,
		TitleTemplate: title, BodyTemplate: body,
		Enabled: true, UpdatedBy: in.Actor,
		CreatedAt: now, UpdatedAt: now,
	}
	return s.stores.NotificationTemplates().Upsert(ctx, t)
}

// ── Preferensi ──────────────────────────────────────────────────────────

// PreferenceView adalah satu baris preferensi efektif pengguna.
type PreferenceView struct {
	Key     notification.EventKey `json:"event_key"`
	Channel notification.Channel  `json:"channel"`
	Enabled bool                  `json:"enabled"`
	// Explicit=true berarti pengguna pernah mengaturnya sendiri.
	Explicit bool `json:"explicit"`
}

// Preferences mengembalikan matriks preferensi pengguna: seluruh event ×
// kanal, dengan nilai efektif (default jika tidak pernah diatur).
func (s *Service) Preferences(ctx context.Context, userID string) ([]PreferenceView, error) {
	saved, err := s.stores.NotificationPreferences().List(ctx, userID)
	if err != nil {
		return nil, err
	}
	type k struct {
		key notification.EventKey
		ch  notification.Channel
	}
	by := make(map[k]notification.Preference, len(saved))
	for _, p := range saved {
		by[k{p.EventKey, p.Channel}] = p
	}
	out := make([]PreferenceView, 0, len(notification.EventKeys())*2)
	for _, key := range notification.EventKeys() {
		for _, ch := range []notification.Channel{notification.ChannelInApp, notification.ChannelEmail} {
			p, ok := by[k{key, ch}]
			if ok {
				out = append(out, PreferenceView{Key: key, Channel: ch, Enabled: p.Enabled, Explicit: true})
				continue
			}
			def := ch == notification.ChannelInApp // email default nonaktif
			out = append(out, PreferenceView{Key: key, Channel: ch, Enabled: def})
		}
	}
	return out, nil
}

// UpdatePreferenceInput adalah permintaan mengubah satu preferensi.
type UpdatePreferenceInput struct {
	Key     string
	Channel string
	Enabled bool
}

// UpdatePreference menyimpan satu preferensi pengguna.
func (s *Service) UpdatePreference(ctx context.Context, userID string, in UpdatePreferenceInput) error {
	key := notification.EventKey(strings.TrimSpace(in.Key))
	if !key.Valid() {
		return shared.Validation(map[string]string{"event_key": "Event notifikasi tidak dikenal."})
	}
	ch := notification.Channel(strings.TrimSpace(in.Channel))
	if !ch.Valid() {
		return shared.Validation(map[string]string{"channel": "Kanal harus inapp atau email."})
	}
	now := time.Now().UTC()
	return s.stores.NotificationPreferences().Upsert(ctx, &notification.Preference{
		ID: platform.NewUUID(), UserID: userID,
		EventKey: key, Channel: ch, Enabled: in.Enabled,
		CreatedAt: now, UpdatedAt: now,
	})
}

// ── Fan-out dari kejadian bisnis ────────────────────────────────────────

// DispatchShipmentStatus membuat notifikasi untuk satu perubahan status
// pengiriman. WAJIB dipanggil DI DALAM transaksi tracking (querier = tx) agar
// notifikasi lahir bersama kejadian yang menimbulkannya — tidak ada notifikasi
// untuk kejadian yang di-rollback, dan tidak ada kejadian tanpa notifikasi.
//
// Email TIDAK dikirim di sini: satu event outbox `notification.send` ditulis
// untuk seluruh penerima opt-in email, dan worker yang mengirimkannya.
func (s *Service) DispatchShipmentStatus(ctx context.Context, q database.Querier, n notification.ShipmentNotice) error {
	key, ok := notification.EventKeyForShipmentStatus(n.Status)
	if !ok {
		return nil // status tanpa notifikasi (mis. pending_pickup awal)
	}
	st := s.stores.WithTx(q)
	repo := st.Notifications()

	vars := map[string]any{
		"awb":      n.AWB,
		"status":   n.Status,
		"shipment": n.ShipmentID,
	}

	// ── In-app: default aktif, kecuali preferensi menonaktifkan. ──
	inapp, err := repo.InAppRecipients(ctx, n.OrganizationID, key)
	if err != nil {
		return err
	}
	title, body := s.render(ctx, n.OrganizationID, key, vars)
	now := time.Now().UTC()
	rows := make([]notification.Notification, 0, len(inapp))
	for _, rc := range inapp {
		rows = append(rows, notification.Notification{
			ID: platform.NewUUID(), OrganizationID: n.OrganizationID, UserID: rc.UserID,
			EventKey: key, Channel: notification.ChannelInApp,
			Priority: notification.PriorityFor(key),
			Title:    title, Body: body,
			Data:      map[string]any{"shipment_id": n.ShipmentID, "awb": n.AWB},
			CreatedAt: now,
		})
	}
	if len(rows) > 0 {
		if err := repo.CreateMany(ctx, rows); err != nil {
			return err
		}
	}

	// ── Email: opt-in; dikirim worker lewat outbox (ADR-005). ──
	email, err := repo.EmailRecipients(ctx, n.OrganizationID, key)
	if err != nil {
		return err
	}
	if len(email) == 0 {
		return nil
	}
	recipients := make([]map[string]any, 0, len(email))
	for _, rc := range email {
		recipients = append(recipients, map[string]any{
			"user_id": rc.UserID, "email": rc.Email,
			"title": title, "body": body,
		})
	}
	return st.Outbox().Publish(ctx, event.New(
		platform.NewUUID(),
		event.NotificationSend,
		n.ShipmentID,
		n.OrganizationID,
		n.OccurredAt,
		map[string]any{
			"event_key":  string(key),
			"awb":        n.AWB,
			"recipients": recipients,
		},
	))
}

// render menyusun judul & isi dari template kustom organisasi, atau default
// platform bila tidak ada. Template kustom adalah kenyamanan: bila pembacaan
// gagal, render tetap lanjut dengan default dan tidak menggagalkan kejadian
// bisnis yang menimbulkan notifikasi.
func (s *Service) render(ctx context.Context, orgID string, key notification.EventKey, vars map[string]any) (string, string) {
	title, body := "", ""
	t, found, err := s.stores.Notifications().CustomTemplate(ctx, orgID, key, notification.ChannelInApp)
	switch {
	case err != nil:
		s.log.Warn("template notifikasi kustom gagal dibaca", "org", orgID, "key", string(key), "error", err)
	case found && t != nil:
		title, body = t.TitleTemplate, t.BodyTemplate
	}
	if title == "" {
		if def, ok := notification.TemplateFor(key, notification.ChannelInApp); ok {
			title, body = def.Title, def.Body
		}
	}
	return notification.Render(title, vars), notification.Render(body, vars)
}
