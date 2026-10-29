package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/cod"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// CODRepo menyimpan transaksi COD & settlement, tenant-scoped.
type CODRepo struct{ q Querier }

const codTxnCols = `id, organization_id, shipment_id::text, courier_code, amount, fee, status,
	COALESCE(settlement_id::text,''), collected_at, created_at, updated_at`

// CreateTransaction menyimpan satu tagihan COD (idempoten per shipment via
// unique index; bentrok dipetakan ke ErrDuplicate pemanggil).
func (r *CODRepo) CreateTransaction(ctx context.Context, t *cod.Transaction) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO cod_transactions (
			id, organization_id, shipment_id, courier_code, amount, fee, status, created_at, updated_at
		) VALUES ($1,$2,$3::uuid,$4,$5,$6,$7,$8,$9)`,
		t.ID, t.OrganizationID, t.ShipmentID, t.CourierCode, t.Amount, t.Fee,
		string(t.Status), t.CreatedAt, t.UpdatedAt)
	if isUniqueViolationOn(err, "uq_cod_shipment") {
		return shared.Conflict("COD_DUPLICATE", "Transaksi COD untuk pengiriman ini sudah ada.")
	}
	return err
}

func scanCODTxn(row pgx.Row) (*cod.Transaction, error) {
	var t cod.Transaction
	var status string
	err := row.Scan(&t.ID, &t.OrganizationID, &t.ShipmentID, &t.CourierCode,
		&t.Amount, &t.Fee, &status, &t.SettlementID, &t.CollectedAt, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("COD_TRANSACTION_NOT_FOUND", "Transaksi COD tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	t.Status = cod.Status(status)
	return &t, nil
}

// GetTransaction mengambil satu transaksi milik organisasi.
func (r *CODRepo) GetTransaction(ctx context.Context, orgID, id string) (*cod.Transaction, error) {
	row := r.q.QueryRow(ctx, `SELECT `+codTxnCols+` FROM cod_transactions WHERE organization_id=$1 AND id=$2`, orgID, id)
	return scanCODTxn(row)
}

// GetByShipment mengambil transaksi COD untuk satu pengiriman (bila ada).
func (r *CODRepo) GetByShipment(ctx context.Context, orgID, shipmentID string) (*cod.Transaction, error) {
	row := r.q.QueryRow(ctx, `SELECT `+codTxnCols+` FROM cod_transactions
		WHERE organization_id=$1 AND shipment_id=$2::uuid`, orgID, shipmentID)
	return scanCODTxn(row)
}

// MarkCollected memindahkan pending → collected (dipanggil saat delivered).
func (r *CODRepo) MarkCollected(ctx context.Context, orgID, shipmentID string, at time.Time) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE cod_transactions SET status='collected', collected_at=$3
		WHERE organization_id=$1 AND shipment_id=$2::uuid AND status='pending'`,
		orgID, shipmentID, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return shared.NotFound("COD_TRANSACTION_NOT_FOUND", "Transaksi COD pending tidak ditemukan.")
	}
	return nil
}

// MarkFailed memindahkan pending → failed (pengiriman gagal/batal).
func (r *CODRepo) MarkFailed(ctx context.Context, orgID, shipmentID string) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE cod_transactions SET status='failed'
		WHERE organization_id=$1 AND shipment_id=$2::uuid AND status='pending'`,
		orgID, shipmentID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return shared.NotFound("COD_TRANSACTION_NOT_FOUND", "Transaksi COD pending tidak ditemukan.")
	}
	return nil
}

// ListTransactions mengembalikan halaman transaksi dengan filter.
func (r *CODRepo) ListTransactions(ctx context.Context, orgID string, f CODTxnFilter) ([]cod.Transaction, int, error) {
	where := `organization_id=$1`
	args := []any{orgID}
	n := 2
	if f.Status != "" {
		if !cod.Status(f.Status).Valid() {
			return nil, 0, shared.Validation(map[string]string{"status": "Status COD tidak dikenal."})
		}
		where += ` AND status=$` + itoa(n)
		args = append(args, f.Status)
		n++
	}
	if f.CourierCode != "" {
		where += ` AND courier_code=$` + itoa(n)
		args = append(args, f.CourierCode)
		n++
	}
	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM cod_transactions WHERE `+where, args...).Scan(&total); err != nil {
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
	rows, err := r.q.Query(ctx, `SELECT `+codTxnCols+` FROM cod_transactions WHERE `+where+
		` ORDER BY created_at DESC LIMIT $`+itoa(n)+` OFFSET $`+itoa(n+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]cod.Transaction, 0, limit)
	for rows.Next() {
		t, err := scanCODTxn(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *t)
	}
	return out, total, rows.Err()
}

// CODTxnFilter adalah filter transaksi dari endpoint.
type CODTxnFilter struct {
	Status      string
	CourierCode string
	Limit       int
	Offset      int
}

// Summary menghitung rekap per status (TASK-042: expected settlement).
func (r *CODRepo) Summary(ctx context.Context, orgID string) (*cod.Summary, error) {
	var s cod.Summary
	err := r.q.QueryRow(ctx, `
		SELECT
			COALESCE(sum(amount) FILTER (WHERE true), 0),
			COALESCE(sum(amount) FILTER (WHERE status='pending'), 0),
			COALESCE(sum(amount) FILTER (WHERE status='collected'), 0),
			COALESCE(sum(amount) FILTER (WHERE status='settling'), 0),
			COALESCE(sum(amount) FILTER (WHERE status='settled'), 0),
			COALESCE(sum(amount) FILTER (WHERE status IN ('collected','settling')), 0),
			count(*)
		FROM cod_transactions WHERE organization_id=$1`, orgID).
		Scan(&s.Total, &s.Pending, &s.Collected, &s.Settling, &s.Settled, &s.ExpectedSettlement, &s.Count)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ── Settlement ──────────────────────────────────────────────────────────

const codSettleCols = `id, organization_id, courier_code, period_start::text, period_end::text, status,
	total_amount, total_fee, COALESCE(reference,''), paid_at, COALESCE(proof_url,''), COALESCE(created_by::text,''), created_at, updated_at`

func scanSettlement(row pgx.Row) (*cod.Settlement, error) {
	var s cod.Settlement
	var status string
	err := row.Scan(&s.ID, &s.OrganizationID, &s.CourierCode, &s.PeriodStart, &s.PeriodEnd,
		&status, &s.TotalAmount, &s.TotalFee, &s.Reference, &s.PaidAt, &s.ProofURL,
		&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shared.NotFound("COD_SETTLEMENT_NOT_FOUND", "Settlement tidak ditemukan.")
	}
	if err != nil {
		return nil, err
	}
	s.Status = cod.SettlementStatus(status)
	return &s, nil
}

// CreateSettlement menyimpan settlement + mengikat item + memindahkan
// transaksi ke settling — semuanya di transaksi pemanggil.
func (r *CODRepo) CreateSettlement(ctx context.Context, s *cod.Settlement) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO cod_settlements (
			id, organization_id, courier_code, period_start, period_end, status,
			total_amount, total_fee, idempotency_key, created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,'')::uuid,$11,$12)`,
		s.ID, s.OrganizationID, s.CourierCode, s.PeriodStart, s.PeriodEnd, string(s.Status),
		s.TotalAmount, s.TotalFee, s.IdempotencyKey, s.CreatedBy, s.CreatedAt, s.UpdatedAt)
	if err != nil {
		return err
	}
	for _, it := range s.Items {
		if _, err := r.q.Exec(ctx, `
			INSERT INTO cod_settlement_items (settlement_id, cod_transaction_id)
			VALUES ($1,$2::uuid)`, s.ID, it.ID); err != nil {
			return err
		}
	}
	// Status transaksi lama TIDAK diubah isinya — hanya statusnya yang
	// bergerak ke settling (aturan "tidak mengubah transaksi lama").
	ids := make([]string, 0, len(s.Items))
	for _, it := range s.Items {
		ids = append(ids, it.ID)
	}
	tag, err := r.q.Exec(ctx, `
		UPDATE cod_transactions SET status='settling', settlement_id=$2::uuid
		WHERE organization_id=$1 AND id = ANY($3::uuid[]) AND status IN ('pending','collected')`,
		s.OrganizationID, s.ID, ids)
	if err != nil {
		return err
	}
	if int(tag.RowsAffected()) != len(s.Items) {
		return shared.Validation(map[string]string{
			"transaction_ids": "Sebagian transaksi tidak bisa dimasukkan settlement (status berubah atau bukan milik org/kurir ini).",
		})
	}
	return nil
}

// GetSettlement mengambil settlement beserta itemnya.
func (r *CODRepo) GetSettlement(ctx context.Context, orgID, id string) (*cod.Settlement, error) {
	row := r.q.QueryRow(ctx, `SELECT `+codSettleCols+` FROM cod_settlements WHERE organization_id=$1 AND id=$2`, orgID, id)
	s, err := scanSettlement(row)
	if err != nil {
		return nil, err
	}
	items, err := r.settlementItems(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	s.Items = items
	return s, nil
}

func (r *CODRepo) settlementItems(ctx context.Context, orgID, settlementID string) ([]cod.Transaction, error) {
	rows, err := r.q.Query(ctx, `SELECT t.id, t.organization_id, t.shipment_id::text, t.courier_code,
		t.amount, t.fee, t.status, COALESCE(t.settlement_id::text,''), t.collected_at, t.created_at, t.updated_at
		FROM cod_transactions t
		JOIN cod_settlement_items i ON i.cod_transaction_id = t.id AND i.settlement_id = $2::uuid
		WHERE t.organization_id=$1 ORDER BY t.created_at`, orgID, settlementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]cod.Transaction, 0, 8)
	for rows.Next() {
		t, err := scanCODTxn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// ListSettlements mengembalikan halaman settlement.
func (r *CODRepo) ListSettlements(ctx context.Context, orgID string, f CODSettleFilter) ([]cod.Settlement, int, error) {
	where := `organization_id=$1`
	args := []any{orgID}
	n := 2
	if f.Status != "" {
		if f.Status != string(cod.SettlementPending) && f.Status != string(cod.SettlementPaid) {
			return nil, 0, shared.Validation(map[string]string{"status": "Status settlement harus pending atau paid."})
		}
		where += ` AND status=$` + itoa(n)
		args = append(args, f.Status)
		n++
	}
	if f.CourierCode != "" {
		where += ` AND courier_code=$` + itoa(n)
		args = append(args, f.CourierCode)
		n++
	}
	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM cod_settlements WHERE `+where, args...).Scan(&total); err != nil {
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
	rows, err := r.q.Query(ctx, `SELECT `+codSettleCols+` FROM cod_settlements WHERE `+where+
		` ORDER BY created_at DESC LIMIT $`+itoa(n)+` OFFSET $`+itoa(n+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]cod.Settlement, 0, limit)
	for rows.Next() {
		s, err := scanSettlement(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *s)
	}
	return out, total, rows.Err()
}

// CODSettleFilter adalah filter settlement dari endpoint.
type CODSettleFilter struct {
	Status      string
	CourierCode string
	Limit       int
	Offset      int
}

// SettlementsForExport mengembalikan seluruh settlement untuk CSV (tanpa paging).
func (r *CODRepo) SettlementsForExport(ctx context.Context, orgID, courierCode, periodStart, periodEnd string) ([]cod.Settlement, error) {
	where := `organization_id=$1`
	args := []any{orgID}
	n := 2
	if courierCode != "" {
		where += ` AND courier_code=$` + itoa(n)
		args = append(args, courierCode)
		n++
	}
	if periodStart != "" {
		where += ` AND period_start >= $` + itoa(n)
		args = append(args, periodStart)
		n++
	}
	if periodEnd != "" {
		where += ` AND period_end <= $` + itoa(n)
		args = append(args, periodEnd)
		n++
	}
	rows, err := r.q.Query(ctx, `SELECT `+codSettleCols+` FROM cod_settlements WHERE `+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]cod.Settlement, 0, 16)
	for rows.Next() {
		s, err := scanSettlement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// MarkPaid menandai settlement lunas + menyimpan referensi transfer. Status
// transaksi COD ikut settling → settled (baris transaksi TIDAK diubah isi
// lainnya).
func (r *CODRepo) MarkPaid(ctx context.Context, orgID, id string, in cod.MarkPaidInput) error {
	paidAt := time.Now().UTC()
	if in.PaidAt != nil {
		paidAt = *in.PaidAt
	}
	tag, err := r.q.Exec(ctx, `
		UPDATE cod_settlements SET status='paid', reference=NULLIF($3,''), paid_at=$4,
			proof_url=NULLIF($5,'')
		WHERE organization_id=$1 AND id=$2 AND status='pending'`,
		orgID, id, in.Reference, paidAt, in.ProofURL)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return cod.ErrAlreadySettled()
	}
	_, err = r.q.Exec(ctx, `
		UPDATE cod_transactions SET status='settled'
		WHERE organization_id=$1 AND settlement_id=$2::uuid AND status='settling'`,
		orgID, id)
	return err
}

// GetSettlementIDempotency mengembalikan settlement hasil replay key sama.
func (r *CODRepo) GetSettlementIDempotency(ctx context.Context, orgID, key string) (*cod.Settlement, error) {
	row := r.q.QueryRow(ctx, `SELECT `+codSettleCols+` FROM cod_settlements
		WHERE organization_id=$1 AND idempotency_key=$2`, orgID, key)
	return scanSettlement(row)
}
