package database

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/returns"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// ReturnsRepo menyimpan retur, item, dan kebijakan per organisasi.
type ReturnsRepo struct{ q Querier }

const returnCols = `id, organization_id, COALESCE(order_id::text,''), COALESCE(shipment_id::text,''),
	type, COALESCE(reason,''), status, refund_amount, COALESCE(refund_method,''),
	COALESCE(decided_by::text,''), decided_at, refunded_at, COALESCE(created_by::text,''), created_at, updated_at`

func scanReturn(row pgx.Row) (*returns.Return, error) {
	var r returns.Return
	var typ, status string
	err := row.Scan(&r.ID, &r.OrganizationID, &r.OrderID, &r.ShipmentID, &typ, &r.Reason,
		&status, &r.RefundAmount, &r.RefundMethod, &r.DecidedBy, &r.DecidedAt,
		&r.RefundedAt, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("RETURN_NOT_FOUND", "Retur tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	r.Type = returns.Type(typ)
	r.Status = returns.Status(status)
	return &r, nil
}

// Create menyimpan retur + itemnya (transaksi pemanggil).
func (r *ReturnsRepo) Create(ctx context.Context, ret *returns.Return) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO returns (
			id, organization_id, order_id, shipment_id, type, reason, status,
			refund_amount, created_by, created_at, updated_at
		) VALUES ($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5,$6,$7,$8,NULLIF($9,'')::uuid,$10,$11)`,
		ret.ID, ret.OrganizationID, ret.OrderID, ret.ShipmentID, string(ret.Type),
		ret.Reason, string(ret.Status), ret.RefundAmount, ret.CreatedBy, ret.CreatedAt, ret.UpdatedAt)
	if err != nil {
		return err
	}
	return r.insertItems(ctx, ret.ID, ret.Items)
}

// insertItems menulis item retur. returnID eksplisit — jangan diambil dari
// struct yang mungkin belum terisi ID (sumber bug uuid kosong saat inspect).
func (r *ReturnsRepo) insertItems(ctx context.Context, returnID string, items []returns.Item) error {
	for _, it := range items {
		if _, err := r.q.Exec(ctx, `
			INSERT INTO return_items (id, return_id, order_item_id, name_snapshot, qty, condition, restocked)
			VALUES ($1,$2::uuid,NULLIF($3,'')::uuid,NULLIF($4,''),$5,$6,$7)`,
			it.ID, returnID, it.OrderItemID, it.NameSnapshot, it.Qty, string(it.Condition), it.Restocked); err != nil {
			return err
		}
	}
	return nil
}

// GetByID mengambil retur milik organisasi beserta itemnya.
func (r *ReturnsRepo) GetByID(ctx context.Context, orgID, id string) (*returns.Return, error) {
	row := r.q.QueryRow(ctx, `SELECT `+returnCols+` FROM returns WHERE organization_id=$1 AND id=$2`, orgID, id)
	ret, err := scanReturn(row)
	if err != nil {
		return nil, err
	}
	items, err := r.items(ctx, id)
	if err != nil {
		return nil, err
	}
	ret.Items = items
	return ret, nil
}

func (r *ReturnsRepo) items(ctx context.Context, returnID string) ([]returns.Item, error) {
	rows, err := r.q.Query(ctx, `SELECT id, COALESCE(order_item_id::text,''), COALESCE(name_snapshot,''),
		qty, condition, restocked FROM return_items WHERE return_id=$1 ORDER BY id`, returnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]returns.Item, 0, 4)
	for rows.Next() {
		var it returns.Item
		var cond string
		if err := rows.Scan(&it.ID, &it.OrderItemID, &it.NameSnapshot, &it.Qty, &cond, &it.Restocked); err != nil {
			return nil, err
		}
		it.Condition = returns.Condition(cond)
		out = append(out, it)
	}
	return out, rows.Err()
}

// ReturnsFilter filter daftar retur.
type ReturnsFilter struct {
	Status string
	Type   string
	Limit  int
	Offset int
}

// List mengembalikan halaman retur.
func (r *ReturnsRepo) List(ctx context.Context, orgID string, f ReturnsFilter) ([]returns.Return, int, error) {
	where := `organization_id=$1`
	args := []any{orgID}
	n := 2
	if f.Status != "" {
		if !returns.Status(f.Status).Valid() {
			return nil, 0, shared.Validation(map[string]string{"status": "Status retur tidak dikenal."})
		}
		where += ` AND status=$` + itoa(n)
		args = append(args, f.Status)
		n++
	}
	if f.Type != "" {
		if returns.Type(f.Type) != returns.TypeRefund && returns.Type(f.Type) != returns.TypeExchange {
			return nil, 0, shared.Validation(map[string]string{"type": "Tipe retur harus refund atau exchange."})
		}
		where += ` AND type=$` + itoa(n)
		args = append(args, f.Type)
		n++
	}
	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM returns WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	args = append(args, limit, f.Offset)
	rows, err := r.q.Query(ctx, `SELECT `+returnCols+` FROM returns WHERE `+where+
		` ORDER BY created_at DESC LIMIT $`+itoa(n)+` OFFSET $`+itoa(n+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]returns.Return, 0, limit)
	for rows.Next() {
		ret, err := scanReturn(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *ret)
	}
	return out, total, rows.Err()
}

// UpdateStatus menulis status baru + kolom keputusan (decided_by/at) atau
// refund. 0 baris berarti id/status salah.
func (r *ReturnsRepo) UpdateStatus(ctx context.Context, orgID, id string, status returns.Status, decidedBy string, refundAmount int64, refundMethod string) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE returns SET status=$3,
			decided_by = CASE WHEN $3 IN ('approved','rejected') THEN NULLIF($4,'')::uuid ELSE decided_by END,
			decided_at = CASE WHEN $3 IN ('approved','rejected') THEN now() ELSE decided_at END,
			refund_amount = CASE WHEN $3 = 'refunded' THEN $5 ELSE refund_amount END,
			refund_method = CASE WHEN $3 = 'refunded' THEN NULLIF($6,'') ELSE refund_method END,
			refunded_at   = CASE WHEN $3 = 'refunded' THEN now() ELSE refunded_at END
		WHERE organization_id=$1 AND id=$2`,
		orgID, id, string(status), decidedBy, refundAmount, refundMethod)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return shared.NotFound("RETURN_NOT_FOUND", "Retur tidak ditemukan.")
	}
	return nil
}

// ReplaceItems menulis ulang item (dipakai inspect untuk mengisi kondisi &
// status restock).
func (r *ReturnsRepo) ReplaceItems(ctx context.Context, orgID, returnID string, items []returns.Item) error {
	if _, err := r.q.Exec(ctx, `DELETE FROM return_items WHERE return_id=$1`, returnID); err != nil {
		return err
	}
	return r.insertItems(ctx, returnID, items)
}

// ── Policy ──────────────────────────────────────────────────────────────

// GetPolicy mengembalikan kebijakan organisasi (kosong → policy default).
func (r *ReturnsRepo) GetPolicy(ctx context.Context, orgID string) (*returns.Policy, error) {
	var p returns.Policy
	var conditions []byte
	err := r.q.QueryRow(ctx, `SELECT id, organization_id, window_days, auto_approve_limit,
		conditions, COALESCE(updated_by::text,''), created_at, updated_at
		FROM return_policies WHERE organization_id=$1`, orgID).
		Scan(&p.ID, &p.OrganizationID, &p.WindowDays, &p.AutoApproveLimit, &conditions,
			&p.UpdatedBy, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		def := returns.DefaultPolicy(orgID)
		return &def, nil
	}
	if err != nil {
		return nil, err
	}
	if len(conditions) > 0 {
		_ = json.Unmarshal(conditions, &p.Conditions)
	}
	return &p, nil
}

// UpsertPolicy menyimpan kebijakan organisasi.
func (r *ReturnsRepo) UpsertPolicy(ctx context.Context, p *returns.Policy) error {
	conditions, err := json.Marshal(p.Conditions)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO return_policies (
			id, organization_id, window_days, auto_approve_limit, conditions, updated_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5::jsonb,NULLIF($6,'')::uuid,$7,$8)
		ON CONFLICT (organization_id) DO UPDATE SET
			window_days = EXCLUDED.window_days,
			auto_approve_limit = EXCLUDED.auto_approve_limit,
			conditions = EXCLUDED.conditions,
			updated_by = EXCLUDED.updated_by,
			updated_at = now()`,
		p.ID, p.OrganizationID, p.WindowDays, p.AutoApproveLimit, conditions,
		p.UpdatedBy, p.CreatedAt, p.UpdatedAt)
	return err
}
