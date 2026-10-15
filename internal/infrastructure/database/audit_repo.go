package database

import (
	"context"

	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// AuditRepo adalah implementasi audit.Recorder berbasis pgx.
//
// Tabel `audit_logs` append-only: file ini sengaja tidak menyediakan UPDATE
// atau DELETE. Koreksi dilakukan dengan menulis baris baru.
type AuditRepo struct {
	q Querier
}

// NewAuditRepo membuat repository audit.
func NewAuditRepo(q Querier) *AuditRepo { return &AuditRepo{q: q} }

// WithTx menjalankan repository di transaksi tertentu — WAJIB dipakai bila
// audit harus ikut ter-rollback bersama perubahan bisnisnya.
func (r *AuditRepo) WithTx(q Querier) *AuditRepo { return &AuditRepo{q: q} }

// Record menyimpan satu entri audit.
//
// organization_id dan actor_user_id dikirim sebagai NULL bila kosong: kolomnya
// ber-FK, dan aksi pra-organisasi (login, register) memang tidak punya tenant.
func (r *AuditRepo) Record(ctx context.Context, e audit.Entry) error {
	actorType := e.ActorType
	if actorType == "" {
		actorType = shared.ActorUser
	}

	var changes any
	if e.Changes != nil {
		changes = e.Changes
	}

	metadata := map[string]any{}
	if e.RequestID != "" {
		metadata["request_id"] = e.RequestID
	}

	_, err := r.q.Exec(ctx, `
		INSERT INTO audit_logs (
			organization_id, actor_user_id, actor_type, action,
			entity_type, entity_id, changes, metadata, ip_address, user_agent
		) VALUES (
			NULLIF($1,'')::uuid, NULLIF($2,'')::uuid, $3, $4,
			NULLIF($5,''), NULLIF($6,'')::uuid, $7, $8,
			NULLIF($9,'')::inet, NULLIF($10,'')
		)`,
		e.OrganizationID, e.ActorUserID, string(actorType), e.Action,
		e.EntityType, e.EntityID, changes, metadata, e.IP, e.UserAgent,
	)
	return err
}

// ListByOrganization mengambil jejak audit sebuah organisasi (untuk panel admin).
func (r *AuditRepo) ListByOrganization(ctx context.Context, orgID string, limit int) ([]audit.Entry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.q.Query(ctx, `
		SELECT COALESCE(organization_id::text,''), COALESCE(actor_user_id::text,''),
		       actor_type, action, COALESCE(entity_type,''), COALESCE(entity_id::text,''),
		       COALESCE(ip_address::text,''), COALESCE(user_agent,'')
		  FROM audit_logs
		 WHERE organization_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []audit.Entry
	for rows.Next() {
		var e audit.Entry
		var actorType string
		if err := rows.Scan(&e.OrganizationID, &e.ActorUserID, &actorType, &e.Action,
			&e.EntityType, &e.EntityID, &e.IP, &e.UserAgent); err != nil {
			return nil, err
		}
		e.ActorType = shared.ActorType(actorType)
		out = append(out, e)
	}
	return out, rows.Err()
}
