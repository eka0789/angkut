package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/angkut/angkut/apps/internal/domain/shipment"
	"github.com/angkut/angkut/apps/internal/platform"
)

// ShipmentRepo menyimpan pengiriman tenant-scoped.
type ShipmentRepo struct{ q Querier }

const shipmentCols = `id, organization_id, COALESCE(order_id::text,''), COALESCE(store_id::text,''),
	COALESCE(warehouse_id::text,''), COALESCE(courier_id::text,''), COALESCE(courier_service_id::text,''),
	courier_code, service_code, awb, status,
	COALESCE(origin_address_id::text,''), COALESCE(destination_address_id::text,''),
	actual_weight, volumetric_weight, chargeable_weight, length, width, height,
	COALESCE(package_type,''), declared_value, insurance_amount, shipping_price, cost_price,
	is_cod, cod_amount, cod_fee, eta_min_days, eta_max_days,
	COALESCE(notes,''), COALESCE(failure_reason,''), booked_at, delivered_at,
	version, COALESCE(idempotency_key,''), COALESCE(label_url,''), created_at, updated_at`

// Create menyimpan shipment beserta item & paketnya. Harus dijalankan di dalam
// transaksi oleh pemanggil agar ketiganya atomik.
func (r *ShipmentRepo) Create(ctx context.Context, s *shipment.Shipment) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO shipments (
			id, organization_id, order_id, store_id, warehouse_id,
			courier_id, courier_service_id, courier_code, service_code, awb, status,
			origin_address_id, destination_address_id,
			actual_weight, volumetric_weight, chargeable_weight, length, width, height, package_type,
			declared_value, insurance_amount, shipping_price, cost_price,
			is_cod, cod_amount, cod_fee, eta_min_days, eta_max_days,
			notes, failure_reason, booked_at, delivered_at, version, idempotency_key, label_url,
			created_at, updated_at
		) VALUES ($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,
			NULLIF($6,'')::uuid,NULLIF($7,'')::uuid,$8,$9,$10,$11,
			NULLIF($12,'')::uuid,NULLIF($13,'')::uuid,
			$14,$15,$16,$17,$18,$19,NULLIF($20,''),
			$21,$22,$23,$24,
			$25,$26,$27,$28,$29,
			NULLIF($30,''),NULLIF($31,''),$32,$33,$34,NULLIF($35,''),NULLIF($36,''),
			$37,$38)`,
		s.ID, s.OrganizationID, s.OrderID, s.StoreID, s.WarehouseID,
		s.CourierID, s.CourierServiceID, s.CourierCode, s.ServiceCode, s.AWB, string(s.Status),
		s.OriginAddressID, s.DestinationAddressID,
		s.ActualWeight, s.VolumetricWeight, s.ChargeableWeight, s.Length, s.Width, s.Height, s.PackageType,
		s.DeclaredValue, s.InsuranceAmount, s.ShippingPrice, s.CostPrice,
		s.IsCOD, s.CODAmount, s.CODFee, s.EtaMinDays, s.EtaMaxDays,
		s.Notes, s.FailureReason, s.BookedAt, s.DeliveredAt, versionOr(s.Version), s.IdempotencyKey, s.LabelURL,
		s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		switch {
		case isUniqueViolationOn(err, "uq_shipment_awb"):
			return shipment.ErrAWBTaken
		case isUniqueViolationOn(err, "uq_shipment_idem"):
			// Pemanggil menangani ini sebagai "sudah ada" lewat GetByIdempotencyKey.
			return shipment.ErrAWBTaken
		case isForeignKeyOn(err, "shipments_order_id_fkey"):
			return shipment.ErrOrderNotFound
		case isForeignKeyOn(err, "shipments_warehouse_id_fkey"):
			return shipment.ErrWarehouseNotFound
		case isForeignKeyOn(err, "shipments_origin_address_id_fkey"):
			return shipment.ErrAddressNotFound
		case isForeignKeyOn(err, "shipments_destination_address_id_fkey"):
			return shipment.ErrAddressNotFound
		case isForeignKeyOn(err, "shipments_courier_id_fkey"), isForeignKeyOn(err, "shipments_courier_service_id_fkey"):
			return shipment.ErrCourierNotFound
		}
		return err
	}

	for idx := range s.Items {
		it := &s.Items[idx]
		if _, err := r.q.Exec(ctx, `
			INSERT INTO shipment_items (id, organization_id, shipment_id, order_item_id, variant_id, qty, name_snapshot, created_at)
			VALUES ($1,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6,$7,$8)`,
			platform.NewUUID(), s.OrganizationID, s.ID, it.OrderItemID, it.VariantID, it.Qty, it.NameSnapshot, s.CreatedAt,
		); err != nil {
			return err
		}
	}

	for idx := range s.Packages {
		p := &s.Packages[idx]
		if _, err := r.q.Exec(ctx, `
			INSERT INTO shipment_packages (
				id, organization_id, shipment_id, sequence, weight, length, width, height,
				awb, qr_payload, barcode_value, created_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10,$11,$12)`,
			platform.NewUUID(), s.OrganizationID, s.ID, p.Sequence, p.Weight, p.Length, p.Width, p.Height,
			p.AWB, p.QRPayload, p.BarcodeValue, s.CreatedAt,
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *ShipmentRepo) GetByID(ctx context.Context, orgID, id string) (*shipment.Shipment, error) {
	row := r.q.QueryRow(ctx,
		`SELECT `+shipmentCols+` FROM shipments WHERE organization_id=$1 AND id=$2`, orgID, id)
	s, err := scanShipment(row)
	if err != nil {
		return nil, err
	}
	if err := r.hydrate(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// GetByAWB mencari shipment lewat resi persis (dipakai pemanggil internal).
func (r *ShipmentRepo) GetByAWB(ctx context.Context, orgID, awb string) (*shipment.Shipment, error) {
	row := r.q.QueryRow(ctx,
		`SELECT `+shipmentCols+` FROM shipments WHERE organization_id=$1 AND awb=$2`, orgID, awb)
	s, err := scanShipment(row)
	if err != nil {
		return nil, err
	}
	if err := r.hydrate(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// GetByIdempotencyKey mengembalikan shipment dengan key tertentu. ok=false
// berarti belum ada — pemanggil lanjut membuat baru.
func (r *ShipmentRepo) GetByIdempotencyKey(ctx context.Context, orgID, key string) (*shipment.Shipment, bool, error) {
	row := r.q.QueryRow(ctx,
		`SELECT `+shipmentCols+` FROM shipments WHERE organization_id=$1 AND idempotency_key=$2`, orgID, key)
	s, err := scanShipment(row)
	if errors.Is(err, shipment.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := r.hydrate(ctx, s); err != nil {
		return nil, false, err
	}
	return s, true, nil
}

// List mengembalikan halaman shipment + total baris cocok filter.
func (r *ShipmentRepo) List(ctx context.Context, orgID string, f shipment.ListFilter) ([]shipment.Shipment, int, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	where := `organization_id=$1`
	args := []any{orgID}
	if f.Status != "" {
		args = append(args, string(f.Status))
		where += ` AND status=$` + itoa(len(args))
	}
	if f.Courier != "" {
		args = append(args, f.Courier)
		where += ` AND courier_code=$` + itoa(len(args))
	}
	if f.StoreID != "" {
		args = append(args, f.StoreID)
		where += ` AND store_id=$` + itoa(len(args)) + `::uuid`
	}
	if f.WarehouseID != "" {
		args = append(args, f.WarehouseID)
		where += ` AND warehouse_id=$` + itoa(len(args)) + `::uuid`
	}
	if f.OrderID != "" {
		args = append(args, f.OrderID)
		where += ` AND order_id=$` + itoa(len(args)) + `::uuid`
	}
	if f.Search != "" {
		args = append(args, f.Search)
		where += ` AND awb ILIKE '%'||$` + itoa(len(args)) + `||'%'`
	}
	if f.From != "" {
		args = append(args, f.From)
		where += ` AND created_at >= $` + itoa(len(args)) + `::timestamptz`
	}
	if f.To != "" {
		args = append(args, f.To)
		where += ` AND created_at < $` + itoa(len(args)) + `::timestamptz`
	}

	var total int
	if err := r.q.QueryRow(ctx, `SELECT count(*) FROM shipments WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.Limit, f.Offset)
	rows, err := r.q.Query(ctx,
		`SELECT `+shipmentCols+` FROM shipments WHERE `+where+
			` ORDER BY created_at DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]shipment.Shipment, 0)
	for rows.Next() {
		s, err := scanShipment(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *s)
	}
	return out, total, rows.Err()
}

// UpdateStatus mengubah status dengan syarat status saat ini masih `from`.
func (r *ShipmentRepo) UpdateStatus(ctx context.Context, orgID, id string, from, to shipment.Status, failureReason string) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE shipments SET status=$4, failure_reason=NULLIF($5,''),
			delivered_at = CASE WHEN $4='delivered' THEN COALESCE(delivered_at, now()) ELSE delivered_at END,
			version = version + 1
		WHERE organization_id=$1 AND id=$2 AND status=$3`,
		orgID, id, string(from), string(to), failureReason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.GetByID(ctx, orgID, id); err != nil {
			return err
		}
		// Status sudah berubah di tempat lain — laporkan sebagai konflik.
		return shipment.ErrNotFound
	}
	return nil
}

// UpdateLabel menyimpan URL label.
func (r *ShipmentRepo) UpdateLabel(ctx context.Context, orgID, id, labelURL string) error {
	tag, err := r.q.Exec(ctx,
		`UPDATE shipments SET label_url=NULLIF($3,'') WHERE organization_id=$1 AND id=$2`,
		orgID, id, labelURL)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return shipment.ErrNotFound
	}
	return nil
}

// NextAWBSequence mengembalikan nomor urut resi berikutnya per organisasi,
// kode kurir, dan hari (YYYYMMDD) — bagian AWB yang unik per hari.
func (r *ShipmentRepo) NextAWBSequence(ctx context.Context, orgID, courierCode, day string) (int64, error) {
	prefix := shipment.AWBLookupPrefix(courierCode, day)
	var maxAWB *string
	err := r.q.QueryRow(ctx, `
		SELECT max(awb) FROM shipments
		 WHERE organization_id=$1 AND awb LIKE $2||'%'`, orgID, prefix).Scan(&maxAWB)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	if maxAWB == nil {
		return 1, nil
	}
	seq, ok := shipment.ParseAWBSeq(prefix, *maxAWB)
	if !ok {
		return 1, nil
	}
	return seq + 1, nil
}

// hydrate memuat item & paket sebuah shipment.
func (r *ShipmentRepo) hydrate(ctx context.Context, s *shipment.Shipment) error {
	itemRows, err := r.q.Query(ctx, `
		SELECT id, COALESCE(order_item_id::text,''), COALESCE(variant_id::text,''), qty, name_snapshot
		  FROM shipment_items WHERE shipment_id=$1 ORDER BY name_snapshot, id`, s.ID)
	if err != nil {
		return err
	}
	defer itemRows.Close()
	s.Items = make([]shipment.Item, 0)
	for itemRows.Next() {
		var it shipment.Item
		if err := itemRows.Scan(&it.ID, &it.OrderItemID, &it.VariantID, &it.Qty, &it.NameSnapshot); err != nil {
			return err
		}
		s.Items = append(s.Items, it)
	}
	if err := itemRows.Err(); err != nil {
		return err
	}

	pkgRows, err := r.q.Query(ctx, `
		SELECT id, organization_id, shipment_id, sequence, weight, length, width, height,
		       COALESCE(awb,''), qr_payload, barcode_value
		  FROM shipment_packages WHERE shipment_id=$1 ORDER BY sequence`, s.ID)
	if err != nil {
		return err
	}
	defer pkgRows.Close()
	s.Packages = make([]shipment.Package, 0)
	for pkgRows.Next() {
		var p shipment.Package
		if err := pkgRows.Scan(&p.ID, &p.OrganizationID, &p.ShipmentID, &p.Sequence, &p.Weight,
			&p.Length, &p.Width, &p.Height, &p.AWB, &p.QRPayload, &p.BarcodeValue); err != nil {
			return err
		}
		s.Packages = append(s.Packages, p)
	}
	return pkgRows.Err()
}

func scanShipment(row pgx.Row) (*shipment.Shipment, error) {
	var (
		s           shipment.Shipment
		status      string
		bookedAt    *time.Time
		deliveredAt *time.Time
		createdAt   time.Time
	)
	err := row.Scan(
		&s.ID, &s.OrganizationID, &s.OrderID, &s.StoreID, &s.WarehouseID,
		&s.CourierID, &s.CourierServiceID, &s.CourierCode, &s.ServiceCode, &s.AWB, &status,
		&s.OriginAddressID, &s.DestinationAddressID,
		&s.ActualWeight, &s.VolumetricWeight, &s.ChargeableWeight, &s.Length, &s.Width, &s.Height,
		&s.PackageType, &s.DeclaredValue, &s.InsuranceAmount, &s.ShippingPrice, &s.CostPrice,
		&s.IsCOD, &s.CODAmount, &s.CODFee, &s.EtaMinDays, &s.EtaMaxDays,
		&s.Notes, &s.FailureReason, &bookedAt, &deliveredAt,
		&s.Version, &s.IdempotencyKey, &s.LabelURL, &createdAt, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, shipment.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.Status = shipment.Status(status)
	s.BookedAt = bookedAt
	s.DeliveredAt = deliveredAt
	s.CreatedAt = createdAt
	s.Items = []shipment.Item{}
	s.Packages = []shipment.Package{}
	return &s, nil
}

// versionOr mengembalikan version minimal 1 (kolom punya DEFAULT 1).
func versionOr(v int) int {
	if v <= 0 {
		return 1
	}
	return v
}

// isUniqueViolationOn mendeteksi unique violation pada constraint tertentu.
func isUniqueViolationOn(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
	}
	return false
}
