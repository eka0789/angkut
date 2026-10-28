package database

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/notification"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// NotificationRepo menyimpan notifikasi per pengguna. Semua query memfilter
// organization_id DAN user_id — notifikasi adalah data pribadi di dalam tenant.
type NotificationRepo struct{ q Querier }

const notificationCols = `id, organization_id, user_id, event_key, channel, priority,
	title, body, data, read_at, created_at`

// Create menyimpan satu notifikasi.
func (r *NotificationRepo) Create(ctx context.Context, n *notification.Notification) error {
	return r.CreateMany(ctx, []notification.Notification{*n})
}

// CreateMany menyimpan beberapa notifikasi sekaligus dalam satu statement —
// satu kejadian yang ditujukan ke banyak penerima tidak perlu N round-trip.
func (r *NotificationRepo) CreateMany(ctx context.Context, items []notification.Notification) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	orgIDs := make([]string, 0, len(items))
	userIDs := make([]string, 0, len(items))
	eventKeys := make([]string, 0, len(items))
	channels := make([]string, 0, len(items))
	priorities := make([]string, 0, len(items))
	titles := make([]string, 0, len(items))
	bodies := make([]string, 0, len(items))
	datas := make([][]byte, 0, len(items))
	createdAts := make([]time.Time, 0, len(items))

	for i := range items {
		it := items[i]
		raw := it.Data
		if raw == nil {
			raw = map[string]any{}
		}
		payload, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		ids = append(ids, it.ID)
		orgIDs = append(orgIDs, it.OrganizationID)
		userIDs = append(userIDs, it.UserID)
		eventKeys = append(eventKeys, string(it.EventKey))
		channels = append(channels, string(it.Channel))
		priorities = append(priorities, string(it.Priority))
		titles = append(titles, it.Title)
		bodies = append(bodies, it.Body)
		datas = append(datas, payload)
		createdAts = append(createdAts, it.CreatedAt)
	}

	_, err := r.q.Exec(ctx, `
		INSERT INTO notifications (
			id, organization_id, user_id, event_key, channel, priority,
			title, body, data, created_at
		)
		SELECT * FROM unnest(
			$1::uuid[], $2::uuid[], $3::uuid[], $4::text[], $5::text[], $6::text[],
			$7::text[], $8::text[], $9::jsonb[], $10::timestamptz[]
		)`,
		ids, orgIDs, userIDs, eventKeys, channels, priorities,
		titles, bodies, datas, createdAts)
	return err
}

// List mengembalikan halaman notifikasi milik pengguna + totalnya.
func (r *NotificationRepo) List(ctx context.Context, orgID, userID string, f notification.Filter) ([]notification.Notification, int, error) {
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	where := `organization_id=$1 AND user_id=$2`
	args := []any{orgID, userID}
	if f.Unread {
		where += ` AND read_at IS NULL`
	}
	if f.Priority != "" {
		args = append(args, string(f.Priority))
		where += ` AND priority=$` + itoa(len(args))
	}
	if f.EventKey != "" {
		args = append(args, string(f.EventKey))
		where += ` AND event_key=$` + itoa(len(args))
	}

	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, f.Offset)
	rows, err := r.q.Query(ctx, `SELECT `+notificationCols+` FROM notifications WHERE `+where+
		` ORDER BY created_at DESC, id DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]notification.Notification, 0, limit)
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *n)
	}
	return out, total, rows.Err()
}

// UnreadCount menghitung notifikasi yang belum dibaca.
func (r *NotificationRepo) UnreadCount(ctx context.Context, orgID, userID string) (int, error) {
	var n int
	err := r.q.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE organization_id=$1 AND user_id=$2 AND read_at IS NULL`,
		orgID, userID).Scan(&n)
	return n, err
}

// MarkRead menandai satu notifikasi milik pengguna sebagai dibaca. Baris yang
// bukan miliknya menghasilkan NotFound (bukan sukses palsu).
func (r *NotificationRepo) MarkRead(ctx context.Context, orgID, userID, id string) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE notifications SET read_at=COALESCE(read_at, now())
		WHERE organization_id=$1 AND user_id=$2 AND id=$3`,
		orgID, userID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Sudah dibaca sebelumnya juga RowsAffected=0 hanya bila baris tidak
		// ada — UPDATE dengan COALESCE tetap menyentuh baris yang ada, jadi
		// nol baris berarti benar-benar tidak ditemukan.
		return shared.NotFound("NOTIFICATION_NOT_FOUND", "Notifikasi tidak ditemukan.")
	}
	return nil
}

// MarkAllRead menandai seluruh notifikasi pengguna yang belum dibaca.
func (r *NotificationRepo) MarkAllRead(ctx context.Context, orgID, userID string) (int, error) {
	tag, err := r.q.Exec(ctx, `
		UPDATE notifications SET read_at=now()
		WHERE organization_id=$1 AND user_id=$2 AND read_at IS NULL`,
		orgID, userID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func scanNotification(row pgx.Row) (*notification.Notification, error) {
	var n notification.Notification
	var eventKey, channel, priority string
	var data []byte
	err := row.Scan(&n.ID, &n.OrganizationID, &n.UserID, &eventKey, &channel, &priority,
		&n.Title, &n.Body, &data, &n.ReadAt, &n.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("NOTIFICATION_NOT_FOUND", "Notifikasi tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	n.EventKey = notification.EventKey(eventKey)
	n.Channel = notification.Channel(channel)
	n.Priority = notification.Priority(priority)
	if len(data) > 0 {
		_ = json.Unmarshal(data, &n.Data)
	}
	return &n, nil
}

// ── Dispatch (fan-out saat kejadian bisnis terjadi) ─────────────────────

// InAppRecipients mengembalikan anggota aktif organisasi yang notifikasi
// in-app-nya aktif untuk event ini. Preferensi eksplisit enabled=false
// mengecualikan pengguna; tanpa preferensi = aktif (default).
func (r *NotificationRepo) InAppRecipients(ctx context.Context, orgID string, key notification.EventKey) ([]notification.Recipient, error) {
	rows, err := r.q.Query(ctx, `
		SELECT m.user_id::text, u.email
		  FROM organization_members m
		  JOIN users u ON u.id = m.user_id AND u.status = 'active'
		 WHERE m.organization_id = $1::uuid
		   AND m.status = 'active'
		   AND NOT EXISTS (
		       SELECT 1 FROM notification_preferences p
		        WHERE p.user_id = m.user_id
		          AND p.event_key = $2
		          AND p.channel = 'inapp'
		          AND p.enabled = false)`,
		orgID, string(key))
	if err != nil {
		return nil, err
	}
	return scanRecipients(rows)
}

// EmailRecipients mengembalikan anggota aktif yang OPT-IN email untuk event
// ini — hanya preferensi eksplisit enabled=true yang dihitung.
func (r *NotificationRepo) EmailRecipients(ctx context.Context, orgID string, key notification.EventKey) ([]notification.Recipient, error) {
	rows, err := r.q.Query(ctx, `
		SELECT m.user_id::text, u.email
		  FROM organization_members m
		  JOIN users u ON u.id = m.user_id AND u.status = 'active'
		  JOIN notification_preferences p
		    ON p.user_id = m.user_id
		   AND p.event_key = $2
		   AND p.channel = 'email'
		   AND p.enabled = true
		 WHERE m.organization_id = $1::uuid
		   AND m.status = 'active'`,
		orgID, string(key))
	if err != nil {
		return nil, err
	}
	return scanRecipients(rows)
}

func scanRecipients(rows pgx.Rows) ([]notification.Recipient, error) {
	defer rows.Close()
	out := make([]notification.Recipient, 0, 8)
	for rows.Next() {
		var rc notification.Recipient
		if err := rows.Scan(&rc.UserID, &rc.Email); err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}

// CustomTemplate mengembalikan template kustom organisasi untuk (key, channel),
// bila ada dan diaktifkan. Template dinonaktifkan dianggap tidak ada (pemanggil
// memakai default).
func (r *NotificationRepo) CustomTemplate(ctx context.Context, orgID string, key notification.EventKey, ch notification.Channel) (*notification.Template, bool, error) {
	row := r.q.QueryRow(ctx, `SELECT `+templateCols+` FROM notification_templates
		WHERE organization_id=$1 AND key=$2 AND channel=$3`, orgID, string(key), string(ch))
	var t notification.Template
	var k, c string
	err := row.Scan(&t.ID, &t.OrganizationID, &k, &c, &t.TitleTemplate, &t.BodyTemplate,
		&t.Enabled, &t.UpdatedBy, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	t.Key = notification.EventKey(k)
	t.Channel = notification.Channel(c)
	if !t.Enabled {
		return nil, false, nil
	}
	return &t, true, nil
}

// ── Template ────────────────────────────────────────────────────────────

// NotificationTemplateRepo menyimpan template pesan per organisasi.
type NotificationTemplateRepo struct{ q Querier }

const templateCols = `id, organization_id, key, channel, title_template, body_template,
	enabled, COALESCE(updated_by::text,''), created_at, updated_at`

// List mengembalikan template yang sudah dikustomisasi organisasi.
func (r *NotificationTemplateRepo) List(ctx context.Context, orgID string) ([]notification.Template, error) {
	rows, err := r.q.Query(ctx, `SELECT `+templateCols+` FROM notification_templates
		WHERE organization_id=$1 ORDER BY key, channel`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []notification.Template
	for rows.Next() {
		var t notification.Template
		var key, channel string
		if err := rows.Scan(&t.ID, &t.OrganizationID, &key, &channel, &t.TitleTemplate,
			&t.BodyTemplate, &t.Enabled, &t.UpdatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Key = notification.EventKey(key)
		t.Channel = notification.Channel(channel)
		out = append(out, t)
	}
	return out, rows.Err()
}

// Upsert membuat atau memperbarui template (key, channel) organisasi.
func (r *NotificationTemplateRepo) Upsert(ctx context.Context, t *notification.Template) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO notification_templates (
			id, organization_id, key, channel, title_template, body_template,
			enabled, updated_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9,$10)
		ON CONFLICT (organization_id, key, channel) DO UPDATE SET
			title_template = EXCLUDED.title_template,
			body_template  = EXCLUDED.body_template,
			enabled        = EXCLUDED.enabled,
			updated_by     = EXCLUDED.updated_by,
			updated_at     = EXCLUDED.updated_at`,
		t.ID, t.OrganizationID, string(t.Key), string(t.Channel), t.TitleTemplate,
		t.BodyTemplate, t.Enabled, t.UpdatedBy, t.CreatedAt, t.UpdatedAt)
	return err
}

// ── Preferensi ──────────────────────────────────────────────────────────

// NotificationPreferenceRepo menyimpan preferensi notifikasi per pengguna.
type NotificationPreferenceRepo struct{ q Querier }

const preferenceCols = `id, user_id, event_key, channel, enabled, created_at, updated_at`

// List mengembalikan preferensi pengguna (hanya yang diset eksplisit).
func (r *NotificationPreferenceRepo) List(ctx context.Context, userID string) ([]notification.Preference, error) {
	rows, err := r.q.Query(ctx, `SELECT `+preferenceCols+` FROM notification_preferences
		WHERE user_id=$1 ORDER BY event_key, channel`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []notification.Preference
	for rows.Next() {
		var p notification.Preference
		var key, channel string
		if err := rows.Scan(&p.ID, &p.UserID, &key, &channel, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.EventKey = notification.EventKey(key)
		p.Channel = notification.Channel(channel)
		out = append(out, p)
	}
	return out, rows.Err()
}

// Upsert menyimpan preferensi (event_key, channel) pengguna.
func (r *NotificationPreferenceRepo) Upsert(ctx context.Context, p *notification.Preference) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO notification_preferences (
			id, user_id, event_key, channel, enabled, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (user_id, event_key, channel) DO UPDATE SET
			enabled    = EXCLUDED.enabled,
			updated_at = EXCLUDED.updated_at`,
		p.ID, p.UserID, string(p.EventKey), string(p.Channel), p.Enabled, p.CreatedAt, p.UpdatedAt)
	return err
}
