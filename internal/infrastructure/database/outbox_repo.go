package database

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/event"
)

// OutboxRepo menulis dan membaca tabel outbox_events (ADR-005).
//
// Tulis (Publish) dipanggil DARI DALAM transaksi bisnis; baca (Claim dsb)
// dipanggil relay di worker. Satu tipe untuk keduanya agar tidak ada dua
// definisi kolom yang bisa lepas sinkron.
type OutboxRepo struct{ q Querier }

// Publish menyimpan event ke outbox. Header menyertakan event_id dan
// occurred_at agar consumer dapat melakukan dedup tanpa membuka payload.
func (r *OutboxRepo) Publish(ctx context.Context, e event.Event) error {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return err
	}
	headers := map[string]any{}
	for k, v := range e.Headers {
		headers[k] = v
	}
	headers["event_id"] = e.ID
	headers["event_type"] = string(e.Type)
	headers["occurred_at"] = e.OccurredAt.UTC().Format(time.RFC3339Nano)
	if e.OrganizationID != "" {
		headers["organization_id"] = e.OrganizationID
	}
	headersB, err := json.Marshal(headers)
	if err != nil {
		return err
	}

	aggregateType := string(e.AggregateType)
	if aggregateType == "" {
		aggregateType = string(event.AggregateOf(e.Type))
	}

	_, err = r.q.Exec(ctx, `
		INSERT INTO outbox_events (
			aggregate_type, aggregate_id, organization_id, event_type, payload, headers
		) VALUES ($1,$2::uuid,NULLIF($3,'')::uuid,$4,CAST($5 AS jsonb),CAST($6 AS jsonb))`,
		aggregateType, e.AggregateID, e.OrganizationID, string(e.Type),
		string(payload), string(headersB))
	return err
}

// Claim mengambil baris pending yang sudah waktunya dan menandainya
// 'processing'. FOR UPDATE SKIP LOCKED membuat beberapa replika relay aman
// berjalan bersamaan: setiap baris hanya diklaim satu replika.
func (r *OutboxRepo) Claim(ctx context.Context, limit int) ([]event.Record, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := r.q.Query(ctx, `
		WITH claimed AS (
			SELECT id FROM outbox_events
			 WHERE status = 'pending' AND available_at <= now()
			 ORDER BY available_at, id
			 LIMIT $1
			 FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events o
		   SET status = 'processing', claimed_at = now(), claimed_by = $2
		  FROM claimed
		 WHERE o.id = claimed.id
		RETURNING o.id, o.aggregate_type, o.aggregate_id::text,
		          COALESCE(o.organization_id::text,''), o.event_type,
		          o.payload, o.headers, o.attempts, o.created_at`,
		limit, relayIdentity())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]event.Record, 0)
	for rows.Next() {
		var (
			rec       event.Record
			payloadB  []byte
			headersB  []byte
			eventType string
		)
		if err := rows.Scan(&rec.ID, &rec.AggregateType, &rec.AggregateID,
			&rec.OrganizationID, &eventType, &payloadB, &headersB,
			&rec.Attempts, &rec.CreatedAt); err != nil {
			return nil, err
		}
		rec.Type = event.Type(eventType)
		if len(payloadB) > 0 {
			_ = json.Unmarshal(payloadB, &rec.Payload)
		}
		if len(headersB) > 0 {
			_ = json.Unmarshal(headersB, &rec.Headers)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// MarkPublished menandai baris berhasil diterbitkan ke broker.
func (r *OutboxRepo) MarkPublished(ctx context.Context, id int64) error {
	_, err := r.q.Exec(ctx, `
		UPDATE outbox_events
		   SET status = 'published', published_at = now(), attempts = attempts + 1,
		       last_error = NULL, claimed_at = NULL, claimed_by = NULL
		 WHERE id = $1`, id)
	return err
}

// MarkFailed mencatat kegagalan penerbitan. Baris kembali ke 'pending' dengan
// available_at dimundurkan (backoff) selama masih ada sisa percobaan; setelah
// batas tercapai baris menjadi 'dead' agar tidak berputar selamanya.
func (r *OutboxRepo) MarkFailed(ctx context.Context, id int64, reason string, retryAfterSeconds int) error {
	if retryAfterSeconds < 0 {
		retryAfterSeconds = 0
	}
	_, err := r.q.Exec(ctx, `
		UPDATE outbox_events
		   SET attempts = attempts + 1,
		       last_error = $2,
		       claimed_at = NULL,
		       claimed_by = NULL,
		       status = CASE WHEN attempts + 1 >= $3 THEN 'dead' ELSE 'failed' END,
		       available_at = CASE WHEN attempts + 1 >= $3 THEN available_at
		                           ELSE now() + make_interval(secs => $4) END
		 WHERE id = $1`,
		id, reason, maxOutboxAttempts, retryAfterSeconds)
	return err
}

// RecoverStaleClaims mengembalikan baris 'processing' yang ditinggalkan relay
// yang mati (crash) ke 'pending' agar tidak macet selamanya.
func (r *OutboxRepo) RecoverStaleClaims(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := r.q.Exec(ctx, `
		UPDATE outbox_events
		   SET status = 'pending', claimed_at = NULL, claimed_by = NULL,
		       last_error = COALESCE(last_error, 'klaim relay basi dipulihkan')
		 WHERE status = 'processing'
		   AND claimed_at IS NOT NULL
		   AND claimed_at < now() - make_interval(secs => $1)`,
		olderThan.Seconds())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// maxOutboxAttempts adalah batas percobaan relay sebelum baris dianggap mati.
// Angka ini adalah kebijakan operasional, bukan parameter bisnis.
const maxOutboxAttempts = 5

// relayIdentity mengembalikan penanda relay untuk kolom claimed_by.
func relayIdentity() string {
	host, err := hostname()
	if err != nil || host == "" {
		return "relay"
	}
	return host
}

// PendingCount menghitung baris yang belum terbit — dipakai /metrics & alert.
func (r *OutboxRepo) PendingCount(ctx context.Context) (int, error) {
	var n int
	err := r.q.QueryRow(ctx,
		`SELECT count(*) FROM outbox_events WHERE status IN ('pending','failed')`).Scan(&n)
	return n, err
}

// DeadCount menghitung baris mati — sinyal butuh perhatian operator.
func (r *OutboxRepo) DeadCount(ctx context.Context) (int, error) {
	var n int
	err := r.q.QueryRow(ctx,
		`SELECT count(*) FROM outbox_events WHERE status = 'dead'`).Scan(&n)
	return n, err
}

// hostname mengembalikan nama host proses. Dipisah sebagai variabel agar
// mudah di-stub di test (nama host tidak memengaruhi kebenaran, hanya jejak).
var hostname = defaultHostname

func defaultHostname() (string, error) { return os.Hostname() }
