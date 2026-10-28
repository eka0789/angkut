package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/angkut/angkut/apps/internal/domain/pickup"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// PickupRepo menyimpan pickup request tenant-scoped.
type PickupRepo struct{ q Querier }

const pickupCols = `id, organization_id, warehouse_id, courier_code, scheduled_at, status, notes, COALESCE(created_by::text,''), created_at, updated_at`

func (r *PickupRepo) Create(ctx context.Context, pr *pickup.Request) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO pickup_requests (
			id, organization_id, warehouse_id, courier_code, scheduled_at, status, notes, created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9,$10)`,
		pr.ID, pr.OrganizationID, pr.WarehouseID, pr.CourierCode, pr.ScheduledAt, string(pr.Status),
		pr.Notes, pr.CreatedBy, pr.CreatedAt, pr.UpdatedAt)
	if err != nil {
		if isForeignKeyViolation(err) {
			return pickup.ErrNotFound
		}
		return err
	}
	return nil
}

func (r *PickupRepo) GetByID(ctx context.Context, orgID, id string) (*pickup.Request, error) {
	row := r.q.QueryRow(ctx, `SELECT `+pickupCols+` FROM pickup_requests WHERE organization_id=$1 AND id=$2`, orgID, id)
	pr, err := scanPickup(row)
	if err != nil {
		return nil, err
	}
	ids, err := r.ListShipments(ctx, pr.ID)
	if err == nil {
		pr.ShipmentIDs = ids
	}
	return pr, nil
}

func (r *PickupRepo) List(ctx context.Context, orgID string, f pickup.ListFilter) ([]pickup.Request, int, error) {
	where := `organization_id=$1`
	args := []any{orgID}
	n := 2
	if f.Status != "" {
		where += ` AND status=$` + itoa(n)
		args = append(args, string(f.Status))
		n++
	}
	if f.WarehouseID != "" {
		where += ` AND warehouse_id=$` + itoa(n)
		args = append(args, f.WarehouseID)
		n++
	}
	// total
	var total int
	if err := r.q.QueryRow(ctx, `SELECT COUNT(*) FROM pickup_requests WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	q := `SELECT ` + pickupCols + ` FROM pickup_requests WHERE ` + where + ` ORDER BY created_at DESC LIMIT $` + itoa(n) + ` OFFSET $` + itoa(n+1)
	args = append(args, limit, offset)
	rows, err := r.q.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]pickup.Request, 0)
	for rows.Next() {
		pr, err := scanPickup(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *pr)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *PickupRepo) UpdateStatus(ctx context.Context, orgID, id string, from, to pickup.Status) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE pickup_requests SET status=$3
		WHERE organization_id=$1 AND id=$2 AND status=$4`,
		orgID, id, string(to), string(from))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Barisnya ada (dicek pemanggil) tetapi statusnya tidak seperti yang
		// diharapkan, atau id/tenant salah. Conflict lebih jujur daripada
		// NotFound untuk perubahan status yang ditolak.
		return shared.Conflict("PICKUP_STATUS_MISMATCH",
			"Status penjemputan sudah berubah; muat ulang lalu coba lagi.")
	}
	return nil
}

// Schedule menyimpan jadwal sekaligus memindahkan status requested → scheduled.
// scheduled_at WAJIB ikut tersimpan — sebelumnya hanya statusnya yang berubah
// sehingga jadwal hilang setelah reload.
func (r *PickupRepo) Schedule(ctx context.Context, orgID, id string, at time.Time) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE pickup_requests SET status='scheduled', scheduled_at=$3
		WHERE organization_id=$1 AND id=$2 AND status='requested'`,
		orgID, id, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return shared.Conflict("PICKUP_STATUS_MISMATCH",
			"Penjemputan tidak dalam status yang bisa dijadwalkan.")
	}
	return nil
}

// CancelFrom membatalkan pickup dari status non-terminal mana pun.
func (r *PickupRepo) CancelFrom(ctx context.Context, orgID, id string) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE pickup_requests SET status='cancelled'
		WHERE organization_id=$1 AND id=$2 AND status IN ('requested','scheduled')`,
		orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return shared.Conflict("PICKUP_NOT_CANCELLABLE",
			"Penjemputan yang sudah selesai/dibatalkan tidak bisa dibatalkan lagi.")
	}
	return nil
}

// AddShipments melampirkan pengiriman milik organisasi yang sama.
//
// INSERT ... SELECT mengikat KEDUA sisi ke tenant: pickup harus milik org ini
// (kalau tidak, org lain bisa menulis ke pickup kita) DAN shipment harus milik
// org yang sama dengan pickup (kalau tidak, shipment tenant lain bocor ke
// daftar penjemputan). Jumlah baris yang masuk dibandingkan dengan jumlah ID
// agar ID tak dikenal tidak diabaikan diam-diam.
func (r *PickupRepo) AddShipments(ctx context.Context, orgID, pickupID string, shipmentIDs []string) error {
	if len(shipmentIDs) == 0 {
		return nil
	}
	tag, err := r.q.Exec(ctx, `
		INSERT INTO pickup_shipments (pickup_id, shipment_id)
		SELECT p.id, s.id
		  FROM pickup_requests p
		  JOIN shipments s
		    ON s.id = ANY($3::uuid[])
		   AND s.organization_id = p.organization_id
		 WHERE p.id = $1::uuid
		   AND p.organization_id = $2::uuid
		ON CONFLICT (pickup_id, shipment_id) DO NOTHING`,
		pickupID, orgID, shipmentIDs)
	if err != nil {
		return err
	}
	if int(tag.RowsAffected()) < len(shipmentIDs) {
		// Berarti: ID tidak ada, milik tenant lain, pickup bukan milik org ini,
		// atau sudah terlampir sebelumnya. Pemanggil harus tahu.
		return shared.Validation(map[string]string{
			"shipment_ids": "Sebagian pengiriman tidak ditemukan pada organisasi ini (atau sudah terlampir).",
		})
	}
	return nil
}

func (r *PickupRepo) ListShipments(ctx context.Context, pickupID string) ([]string, error) {
	rows, err := r.q.Query(ctx, `SELECT shipment_id FROM pickup_shipments WHERE pickup_id=$1`, pickupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanPickup(row pgx.Row) (*pickup.Request, error) {
	var (
		pr          pickup.Request
		scheduledAt *time.Time
		createdAt   time.Time
		status      string
	)
	err := row.Scan(&pr.ID, &pr.OrganizationID, &pr.WarehouseID, &pr.CourierCode,
		&scheduledAt, &status, &pr.Notes, &pr.CreatedBy, &createdAt, &pr.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pickup.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	pr.Status = pickup.Status(status)
	pr.ScheduledAt = scheduledAt
	pr.CreatedAt = createdAt
	return &pr, nil
}
