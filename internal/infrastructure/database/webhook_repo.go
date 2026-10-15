package database

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/webhook"
)

// WebhookRepo menyimpan event webhook masuk. Raw body SELALU tersimpan.
type WebhookRepo struct{ q Querier }

const webhookCols = `id, COALESCE(organization_id::text,''), provider, COALESCE(event_id,''),
	COALESCE(event_type,''), payload_hash, signature_valid, COALESCE(signature_error,''),
	payload, COALESCE(content_type,''), headers, COALESCE(source_ip::text,''), COALESCE(awb,''),
	status, attempts, COALESCE(last_error,''), created_at`

// Store menyimpan event masuk; raw body ikut tersimpan.
func (r *WebhookRepo) Store(ctx context.Context, e *webhook.Event) error {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return err
	}
	headers := e.Headers
	if headers == nil {
		headers = map[string]any{}
	}
	headersB, err := json.Marshal(headers)
	if err != nil {
		return err
	}

	_, err = r.q.Exec(ctx, `
		INSERT INTO webhook_events (
			id, organization_id, provider, event_id, event_type, payload_hash,
			signature_valid, signature_error, raw_body, payload, content_type,
			headers, source_ip, awb, status, created_at
		) VALUES ($1,NULLIF($2,'')::uuid,$3,NULLIF($4,''),NULLIF($5,''),$6,
			$7,NULLIF($8,''),$9,CAST($10 AS jsonb),NULLIF($11,''),
			CAST($12 AS jsonb),NULLIF($13,'')::inet,NULLIF($14,''),$15,$16)`,
		e.ID, e.OrganizationID, e.Provider, e.EventID, e.EventType, e.PayloadHash,
		e.SignatureValid, e.SignatureError, rawBodyOf(e), string(payload), e.ContentType,
		string(headersB), e.SourceIP, e.AWB, string(e.Status), e.CreatedAt)
	if isUniqueViolationOn(err, "uq_webhook_provider_event") ||
		isUniqueViolationOn(err, "uq_webhook_provider_hash") {
		return webhook.ErrDuplicate
	}
	return err
}

// GetByID mengambil satu event webhook.
func (r *WebhookRepo) GetByID(ctx context.Context, id string) (*webhook.Event, error) {
	row := r.q.QueryRow(ctx, `SELECT `+webhookCols+` FROM webhook_events WHERE id=$1::uuid`, id)
	return scanWebhook(row)
}

// MarkProcessed menandai event selesai diproses.
func (r *WebhookRepo) MarkProcessed(ctx context.Context, id string) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE webhook_events
		   SET status = 'processed', processed_at = now(), last_error = NULL
		 WHERE id = $1::uuid`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return webhook.ErrNotFound
	}
	return nil
}

// MarkFailed mencatat kegagalan pemrosesan.
func (r *WebhookRepo) MarkFailed(ctx context.Context, id, reason string) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE webhook_events
		   SET status = 'failed', attempts = attempts + 1, last_error = $2
		 WHERE id = $1::uuid`, id, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return webhook.ErrNotFound
	}
	return nil
}

// ClaimPending mengambil event yang belum diproses untuk dikerjakan consumer.
// Signature tidak sah TIDAK diambil — event itu disimpan sebagai bukti, bukan
// untuk diproses. FOR UPDATE SKIP LOCKED membuat aman untuk beberapa worker.
func (r *WebhookRepo) ClaimPending(ctx context.Context, limit int) ([]webhook.Event, error) {
	if limit <= 0 || limit > 200 {
		limit = 25
	}
	rows, err := r.q.Query(ctx, `
		SELECT `+webhookCols+` FROM webhook_events
		 WHERE status = 'received' AND signature_valid
		 ORDER BY created_at
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]webhook.Event, 0)
	for rows.Next() {
		e, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// List mengembalikan halaman event webhook.
func (r *WebhookRepo) List(ctx context.Context, provider string, status webhook.Status, limit, offset int) ([]webhook.Event, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	where := `TRUE`
	args := []any{}
	if provider != "" {
		args = append(args, provider)
		where += ` AND provider=$` + itoa(len(args))
	}
	if status != "" {
		args = append(args, string(status))
		where += ` AND status=$` + itoa(len(args))
	}

	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM webhook_events WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := r.q.Query(ctx,
		`SELECT `+webhookCols+` FROM webhook_events WHERE `+where+
			` ORDER BY created_at DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]webhook.Event, 0)
	for rows.Next() {
		e, err := scanWebhook(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *e)
	}
	return out, total, rows.Err()
}

func scanWebhook(row pgx.Row) (*webhook.Event, error) {
	var (
		e         webhook.Event
		payloadB  []byte
		headersB  []byte
		status    string
		createdAt time.Time
	)
	err := row.Scan(&e.ID, &e.OrganizationID, &e.Provider, &e.EventID, &e.EventType,
		&e.PayloadHash, &e.SignatureValid, &e.SignatureError,
		&payloadB, &e.ContentType, &headersB, &e.SourceIP, &e.AWB,
		&status, &e.Attempts, &e.LastError, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, webhook.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	e.Status = webhook.Status(status)
	e.CreatedAt = createdAt
	if len(payloadB) > 0 {
		_ = json.Unmarshal(payloadB, &e.Payload)
	}
	if len(headersB) > 0 {
		_ = json.Unmarshal(headersB, &e.Headers)
	}
	return &e, nil
}

// rawBody mengembalikan byte asli untuk kolom raw_body.
//
// RawBody adalah sumbernya; kolom NOT NULL dipertahankan aman dengan memakai
// payload JSON bila RawBody kosong (mis. event yang dibentuk di test).
func rawBodyOf(e *webhook.Event) []byte {
	if len(e.RawBody) > 0 {
		return e.RawBody
	}
	b, _ := json.Marshal(e.Payload)
	return b
}

// MarkIgnored menandai event yang sengaja tidak diproses. Dipisah dari
// MarkFailed supaya dasbor operator dapat membedakan "salah" dari "tidak berlaku".
func (r *WebhookRepo) MarkIgnored(ctx context.Context, id, reason string) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE webhook_events
		   SET status = 'ignored', processed_at = now(), last_error = $2
		 WHERE id = $1::uuid`, id, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return webhook.ErrNotFound
	}
	return nil
}

// FindByProviderEventID mencari event dari event_id provider.
func (r *WebhookRepo) FindByProviderEventID(ctx context.Context, provider, eventID string) (*webhook.Event, error) {
	if eventID == "" {
		return nil, webhook.ErrNotFound
	}
	row := r.q.QueryRow(ctx,
		`SELECT `+webhookCols+` FROM webhook_events WHERE provider=$1 AND event_id=$2 AND signature_valid`,
		provider, eventID)
	return scanWebhook(row)
}

// FindByProviderHash mencari event dari hash payload (tanpa event_id).
func (r *WebhookRepo) FindByProviderHash(ctx context.Context, provider, payloadHash string) (*webhook.Event, error) {
	if payloadHash == "" {
		return nil, webhook.ErrNotFound
	}
	row := r.q.QueryRow(ctx,
		`SELECT `+webhookCols+` FROM webhook_events
		  WHERE provider=$1 AND payload_hash=$2 AND event_id IS NULL AND signature_valid`,
		provider, payloadHash)
	return scanWebhook(row)
}
